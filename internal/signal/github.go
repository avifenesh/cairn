package signal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GitHubPoller fetches notifications, events, and releases from GitHub REST API.
type GitHubPoller struct {
	token  string
	orgs   []string // org names to track (discover repos from these)
	client *http.Client
}

// GitHubConfig holds configuration for the GitHub poller.
type GitHubConfig struct {
	Token string   // GitHub personal access token or OAuth token
	Orgs  []string // Organizations to track
}

// NewGitHubPoller creates a GitHub poller. Token is required.
func NewGitHubPoller(cfg GitHubConfig) *GitHubPoller {
	return &GitHubPoller{
		token: cfg.Token,
		orgs:  cfg.Orgs,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (g *GitHubPoller) Source() string { return "github" }

func (g *GitHubPoller) Poll(ctx context.Context, since time.Time) ([]*RawEvent, error) {
	if g.token == "" {
		return nil, fmt.Errorf("github: token not configured")
	}

	var all []*RawEvent

	// 1. Fetch notifications (covers PRs, issues, releases, discussions).
	notifs, err := g.fetchNotifications(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("github notifications: %w", err)
	}
	all = append(all, notifs...)

	// 2. Fetch org events if orgs are configured.
	for _, org := range g.orgs {
		orgEvents, err := g.fetchOrgEvents(ctx, org, since)
		if err != nil {
			// Log but don't fail the whole poll for one org.
			continue
		}
		all = append(all, orgEvents...)
	}

	return all, nil
}

// ghNotification is the GitHub notification API response shape.
type ghNotification struct {
	ID        string    `json:"id"`
	Reason    string    `json:"reason"`
	Unread    bool      `json:"unread"`
	UpdatedAt time.Time `json:"updated_at"`
	Subject   struct {
		Title string `json:"title"`
		URL   string `json:"url"`
		Type  string `json:"type"` // PullRequest, Issue, Release, Discussion, etc.
	} `json:"subject"`
	Repository struct {
		FullName string `json:"full_name"`
		HTMLURL  string `json:"html_url"`
	} `json:"repository"`
}

func (g *GitHubPoller) fetchNotifications(ctx context.Context, since time.Time) ([]*RawEvent, error) {
	url := fmt.Sprintf("https://api.github.com/notifications?since=%s&all=false&per_page=50",
		since.UTC().Format(time.RFC3339))

	body, err := g.doGet(ctx, url)
	if err != nil {
		return nil, err
	}

	var notifs []ghNotification
	if err := json.Unmarshal(body, &notifs); err != nil {
		return nil, fmt.Errorf("github: parse notifications: %w", err)
	}

	var events []*RawEvent
	for _, n := range notifs {
		kind := ghSubjectTypeToKind(n.Subject.Type)
		htmlURL := ghAPIToHTML(n.Subject.URL, n.Repository.FullName)

		events = append(events, &RawEvent{
			Source:     "github",
			SourceID:   fmt.Sprintf("notif:%s", n.ID),
			Kind:       kind,
			Title:      n.Subject.Title,
			URL:        htmlURL,
			Actor:      "", // notifications don't include actor
			Repo:       n.Repository.FullName,
			GroupKey:   n.Repository.FullName,
			Metadata:   map[string]any{"reason": n.Reason, "type": n.Subject.Type},
			OccurredAt: n.UpdatedAt,
		})
	}
	return events, nil
}

// ghOrgEvent is the shape of a GitHub event from the org events API.
type ghOrgEvent struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"` // PushEvent, PullRequestEvent, IssuesEvent, etc.
	CreatedAt time.Time `json:"created_at"`
	Actor     struct {
		Login string `json:"login"`
	} `json:"actor"`
	Repo struct {
		Name string `json:"name"` // "org/repo"
	} `json:"repo"`
	Payload json.RawMessage `json:"payload"`
}

func (g *GitHubPoller) fetchOrgEvents(ctx context.Context, org string, since time.Time) ([]*RawEvent, error) {
	url := fmt.Sprintf("https://api.github.com/orgs/%s/events?per_page=30", org)

	body, err := g.doGet(ctx, url)
	if err != nil {
		return nil, err
	}

	var ghEvents []ghOrgEvent
	if err := json.Unmarshal(body, &ghEvents); err != nil {
		return nil, fmt.Errorf("github: parse org events: %w", err)
	}

	var events []*RawEvent
	for _, e := range ghEvents {
		if e.CreatedAt.Before(since) {
			continue
		}

		kind, title, url := ghEventToFields(e)
		if kind == "" {
			continue // skip event types we don't care about
		}

		events = append(events, &RawEvent{
			Source:     "github",
			SourceID:   fmt.Sprintf("orgevent:%s", e.ID),
			Kind:       kind,
			Title:      title,
			URL:        url,
			Actor:      e.Actor.Login,
			Repo:       e.Repo.Name,
			GroupKey:   e.Repo.Name,
			Metadata:   map[string]any{"eventType": e.Type, "org": org},
			OccurredAt: e.CreatedAt,
		})
	}
	return events, nil
}

func (g *GitHubPoller) doGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return []byte("[]"), nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("github: status %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

func ghSubjectTypeToKind(subjectType string) string {
	switch subjectType {
	case "PullRequest":
		return "pr"
	case "Issue":
		return "issue"
	case "Release":
		return "release"
	case "Discussion":
		return "discussion"
	case "Commit":
		return "commit"
	default:
		return strings.ToLower(subjectType)
	}
}

// ghAPIToHTML converts a GitHub API URL to an HTML URL.
// e.g. https://api.github.com/repos/org/repo/pulls/123 -> https://github.com/org/repo/pull/123
func ghAPIToHTML(apiURL, repoFullName string) string {
	if apiURL == "" {
		if repoFullName != "" {
			return "https://github.com/" + repoFullName
		}
		return ""
	}
	// Convert API URL to HTML URL.
	html := strings.Replace(apiURL, "https://api.github.com/repos/", "https://github.com/", 1)
	html = strings.Replace(html, "/pulls/", "/pull/", 1)
	html = strings.Replace(html, "/issues/", "/issues/", 1) // same path
	html = strings.Replace(html, "/releases/", "/releases/tag/", 1)
	return html
}

func ghEventToFields(e ghOrgEvent) (kind, title, url string) {
	repoURL := "https://github.com/" + e.Repo.Name

	// Parse common payload fields.
	var payload struct {
		Action      string `json:"action"`
		PullRequest *struct {
			Title  string `json:"title"`
			Number int    `json:"number"`
		} `json:"pull_request"`
		Issue *struct {
			Title  string `json:"title"`
			Number int    `json:"number"`
		} `json:"issue"`
		Release *struct {
			TagName string `json:"tag_name"`
			Name    string `json:"name"`
		} `json:"release"`
		Ref string `json:"ref"`
	}
	json.Unmarshal(e.Payload, &payload)

	switch e.Type {
	case "PullRequestEvent":
		if payload.PullRequest != nil {
			return "pr",
				fmt.Sprintf("[%s] %s PR #%d: %s", payload.Action, e.Repo.Name, payload.PullRequest.Number, payload.PullRequest.Title),
				fmt.Sprintf("%s/pull/%d", repoURL, payload.PullRequest.Number)
		}
		return "pr", fmt.Sprintf("[%s] %s PR", payload.Action, e.Repo.Name), repoURL

	case "IssuesEvent":
		if payload.Issue != nil {
			return "issue",
				fmt.Sprintf("[%s] %s #%d: %s", payload.Action, e.Repo.Name, payload.Issue.Number, payload.Issue.Title),
				fmt.Sprintf("%s/issues/%d", repoURL, payload.Issue.Number)
		}
		return "issue", fmt.Sprintf("[%s] %s issue", payload.Action, e.Repo.Name), repoURL

	case "ReleaseEvent":
		if payload.Release != nil {
			name := payload.Release.Name
			if name == "" {
				name = payload.Release.TagName
			}
			return "release",
				fmt.Sprintf("%s released %s", e.Repo.Name, name),
				fmt.Sprintf("%s/releases/tag/%s", repoURL, payload.Release.TagName)
		}
		return "release", fmt.Sprintf("%s new release", e.Repo.Name), repoURL

	case "PushEvent":
		ref := payload.Ref
		branch := ref
		if idx := strings.LastIndex(ref, "/"); idx >= 0 {
			branch = ref[idx+1:]
		}
		return "push",
			fmt.Sprintf("%s pushed to %s", e.Repo.Name, branch),
			fmt.Sprintf("%s/tree/%s", repoURL, branch)

	case "CreateEvent":
		return "branch",
			fmt.Sprintf("%s created %s", e.Repo.Name, payload.Ref),
			repoURL

	case "ForkEvent":
		return "fork",
			fmt.Sprintf("%s forked by %s", e.Repo.Name, e.Actor.Login),
			repoURL

	case "WatchEvent":
		return "star",
			fmt.Sprintf("%s starred by %s", e.Repo.Name, e.Actor.Login),
			repoURL

	default:
		return "", "", ""
	}
}
