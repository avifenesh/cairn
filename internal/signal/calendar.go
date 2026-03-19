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

// CalendarConfig configures the Calendar poller.
type CalendarConfig struct {
	GWSPath    string // path to gws binary
	LookaheadH int    // hours to look ahead (default 48)
	Logger     *slog.Logger
}

// CalendarPoller fetches upcoming calendar events via the gws CLI.
type CalendarPoller struct {
	gwsPath    string
	lookaheadH int
	logger     *slog.Logger
}

// NewCalendarPoller creates a Calendar poller.
func NewCalendarPoller(cfg CalendarConfig) *CalendarPoller {
	h := cfg.LookaheadH
	if h <= 0 {
		h = 48
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &CalendarPoller{
		gwsPath:    cfg.GWSPath,
		lookaheadH: h,
		logger:     logger,
	}
}

func (c *CalendarPoller) Source() string { return SourceCalendar }

func (c *CalendarPoller) Poll(ctx context.Context, _ time.Time) ([]*RawEvent, error) {
	now := time.Now().UTC()
	timeMin := now.Format(time.RFC3339)
	timeMax := now.Add(time.Duration(c.lookaheadH) * time.Hour).Format(time.RFC3339)

	params := map[string]any{
		"calendarId":   "primary",
		"maxResults":   50,
		"timeMin":      timeMin,
		"timeMax":      timeMax,
		"singleEvents": true,
		"orderBy":      "startTime",
	}

	out, err := c.callGWS(ctx, "calendar", "events", "", "list", params)
	if err != nil {
		return nil, fmt.Errorf("calendar: list events: %w", err)
	}

	var resp struct {
		Items []struct {
			ID          string `json:"id"`
			Summary     string `json:"summary"`
			Description string `json:"description"`
			HTMLLink    string `json:"htmlLink"`
			Status      string `json:"status"` // confirmed, tentative, cancelled
			Start       struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"` // all-day events
			} `json:"start"`
			End struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"`
			} `json:"end"`
			Location string `json:"location"`
			Creator  struct {
				Email string `json:"email"`
			} `json:"creator"`
			Attendees []struct {
				Email          string `json:"email"`
				ResponseStatus string `json:"responseStatus"` // needsAction, accepted, declined, tentative
				Self           bool   `json:"self"`
			} `json:"attendees"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("calendar: parse events: %w", err)
	}

	var events []*RawEvent
	for _, item := range resp.Items {
		if item.Status == "cancelled" {
			continue
		}

		startStr := item.Start.DateTime
		if startStr == "" {
			startStr = item.Start.Date
		}
		endStr := item.End.DateTime
		if endStr == "" {
			endStr = item.End.Date
		}

		occurredAt := parseCalTime(startStr)
		if occurredAt.IsZero() {
			occurredAt = time.Now().UTC()
		}

		// Dedup key includes start time so recurring events are separate.
		sourceID := fmt.Sprintf("calendar:%s:%s", item.ID, startStr)

		// Determine kind: invitation if any self-attendee has needsAction.
		kind := KindEvent
		for _, a := range item.Attendees {
			if a.Self && a.ResponseStatus == "needsAction" {
				kind = KindInvitation
				break
			}
		}

		desc := item.Description
		if len(desc) > 200 {
			desc = desc[:200] + "..."
		}

		// Build attendee list for metadata.
		var attendeeEmails []string
		for _, a := range item.Attendees {
			attendeeEmails = append(attendeeEmails, a.Email)
		}

		events = append(events, &RawEvent{
			Source:   SourceCalendar,
			SourceID: sourceID,
			Kind:     kind,
			Title:    item.Summary,
			Body:     desc,
			URL:      item.HTMLLink,
			Actor:    item.Creator.Email,
			GroupKey: "calendar",
			Metadata: map[string]any{
				"start":     startStr,
				"end":       endStr,
				"location":  item.Location,
				"attendees": attendeeEmails,
				"status":    item.Status,
			},
			OccurredAt: occurredAt,
		})
	}

	return events, nil
}

func parseCalTime(s string) time.Time {
	// Try RFC3339 first (dateTime), then date-only.
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC()
	}
	return time.Time{} // zero time on parse failure
}

func (c *CalendarPoller) callGWS(ctx context.Context, service, resource, subResource, method string, params map[string]any) ([]byte, error) {
	args := []string{service, resource}
	if subResource != "" {
		args = append(args, subResource)
	}
	args = append(args, method)
	if len(params) > 0 {
		p, _ := json.Marshal(params)
		args = append(args, "--params", string(p))
	}
	args = append(args, "--format", "json")

	execCtx, cancel := context.WithTimeout(ctx, gwsTimeout) // gwsTimeout defined in gmail.go
	defer cancel()

	cmd := exec.CommandContext(execCtx, c.gwsPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	out, err := cmd.CombinedOutput()
	if execCtx.Err() != nil {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil, fmt.Errorf("calendar: gws timeout after 30s")
	}

	output := strings.TrimSpace(string(out))
	if err != nil {
		return nil, fmt.Errorf("calendar: gws error: %v: %s", err, output)
	}
	return []byte(output), nil
}
