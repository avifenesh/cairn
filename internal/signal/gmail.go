package signal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// GmailConfig configures the Gmail poller.
type GmailConfig struct {
	GWSPath     string       // path to gws binary
	FilterQuery string       // Gmail search query (default: exclude promos/social/forums)
	State       *SourceState // for tracking poll state
	Logger      *slog.Logger
}

// GmailPoller fetches emails via the gws CLI.
// GitHub notification emails are auto-archived (ingested but hidden from feed).
type GmailPoller struct {
	gwsPath     string
	filterQuery string
	state       *SourceState
	logger      *slog.Logger
}

// NewGmailPoller creates a Gmail poller.
func NewGmailPoller(cfg GmailConfig) *GmailPoller {
	filter := cfg.FilterQuery
	if filter == "" {
		filter = "-category:promotions -category:social -category:forums"
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &GmailPoller{
		gwsPath:     cfg.GWSPath,
		filterQuery: filter,
		state:       cfg.State,
		logger:      logger,
	}
}

func (g *GmailPoller) Source() string { return SourceGmail }

func (g *GmailPoller) Poll(ctx context.Context, since time.Time) ([]*RawEvent, error) {
	// List recent messages.
	params := map[string]any{
		"userId":     "me",
		"maxResults": 20,
		"q":          g.filterQuery,
	}
	listOut, err := g.callGWS(ctx, "gmail", "users", "messages", "list", params)
	if err != nil {
		return nil, fmt.Errorf("gmail: list messages: %w", err)
	}

	var listResp struct {
		Messages []struct {
			ID       string `json:"id"`
			ThreadID string `json:"threadId"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(listOut, &listResp); err != nil {
		return nil, fmt.Errorf("gmail: parse list: %w", err)
	}

	var events []*RawEvent
	for _, msg := range listResp.Messages {
		// Fetch metadata for each message.
		getParams := map[string]any{
			"userId": "me",
			"id":     msg.ID,
			"format": "metadata",
		}
		getOut, err := g.callGWS(ctx, "gmail", "users", "messages", "get", getParams)
		if err != nil {
			g.logger.Warn("gmail: get message failed", "id", msg.ID, "error", err)
			continue
		}

		var msgResp struct {
			ID       string `json:"id"`
			ThreadID string `json:"threadId"`
			Snippet  string `json:"snippet"`
			Payload  struct {
				Headers []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"headers"`
			} `json:"payload"`
			LabelIDs     []string `json:"labelIds"`
			InternalDate string   `json:"internalDate"`
		}
		if err := json.Unmarshal(getOut, &msgResp); err != nil {
			continue
		}

		// Extract headers.
		var from, subject, date string
		for _, h := range msgResp.Payload.Headers {
			switch strings.ToLower(h.Name) {
			case "from":
				from = h.Value
			case "subject":
				subject = h.Value
			case "date":
				date = h.Value
			}
		}

		if subject == "" {
			subject = "(no subject)"
		}

		snippet := msgResp.Snippet
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}

		// Parse timestamp.
		occurredAt := parseEmailDate(date)
		if occurredAt.Before(since) {
			continue
		}

		// Check if this is a GitHub notification email.
		autoArchive := isGitHubEmail(from)

		actor := parseEmailName(from)

		events = append(events, &RawEvent{
			Source:   SourceGmail,
			SourceID: fmt.Sprintf("gmail:%s", msg.ID),
			Kind:     KindEmail,
			Title:    subject,
			Body:     snippet,
			URL:      fmt.Sprintf("https://mail.google.com/mail/u/0/#inbox/%s", msg.ID),
			Actor:    actor,
			GroupKey: "gmail",
			Metadata: map[string]any{
				"from":        from,
				"threadId":    msgResp.ThreadID,
				"labelIds":    msgResp.LabelIDs,
				"autoArchive": autoArchive,
			},
			OccurredAt: occurredAt,
		})
	}

	return events, nil
}

// isGitHubEmail returns true if the sender is a GitHub notification address.
func isGitHubEmail(from string) bool {
	lower := strings.ToLower(from)
	return strings.Contains(lower, "notifications@github.com") ||
		strings.Contains(lower, "noreply@github.com")
}

// parseEmailName extracts the display name from a From header.
// "John Doe <john@example.com>" -> "John Doe"
func parseEmailName(from string) string {
	if idx := strings.Index(from, "<"); idx > 0 {
		name := strings.TrimSpace(from[:idx])
		name = strings.Trim(name, "\"")
		if name != "" {
			return name
		}
	}
	// Return email address if no name.
	if idx := strings.Index(from, "<"); idx >= 0 {
		end := strings.Index(from, ">")
		if end > idx {
			return from[idx+1 : end]
		}
	}
	return from
}

// parseEmailDate tries common email date formats.
func parseEmailDate(s string) time.Time {
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"Mon, 2 Jan 2006 15:04:05 MST",
		time.RFC3339,
		"2 Jan 2006 15:04:05 -0700",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, strings.TrimSpace(s)); err == nil {
			return t.UTC()
		}
	}
	return time.Now().UTC()
}

// callGWS executes a gws CLI command and returns the JSON output.
func (g *GmailPoller) callGWS(ctx context.Context, service, resource, subResource, method string, params map[string]any) ([]byte, error) {
	args := []string{service, resource, subResource, method}
	if len(params) > 0 {
		p, _ := json.Marshal(params)
		args = append(args, "--params", string(p))
	}
	args = append(args, "--format", "json")

	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, g.gwsPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	out, err := cmd.CombinedOutput()
	if execCtx.Err() != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return nil, fmt.Errorf("gmail: gws timeout after 30s")
	}
	if err != nil {
		return nil, fmt.Errorf("gmail: gws error: %v: %s", err, string(out))
	}
	return out, nil
}
