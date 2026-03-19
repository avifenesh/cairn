package signal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// DevToConfig configures the Dev.to poller.
type DevToConfig struct {
	Tags     []string // tags to monitor (e.g. "go", "webdev")
	Username string   // your dev.to username (track own articles)
	Logger   *slog.Logger
}

// DevToPoller fetches articles from Dev.to by tag and/or username.
type DevToPoller struct {
	tags     []string
	username string
	client   *http.Client
	logger   *slog.Logger
}

// NewDevToPoller creates a Dev.to poller.
func NewDevToPoller(cfg DevToConfig) *DevToPoller {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &DevToPoller{
		tags:     cfg.Tags,
		username: cfg.Username,
		client:   &http.Client{Timeout: 30 * time.Second},
		logger:   logger,
	}
}

func (d *DevToPoller) Source() string { return SourceDevTo }

type devtoArticle struct {
	ID                int      `json:"id"`
	Title             string   `json:"title"`
	Description       string   `json:"description"`
	URL               string   `json:"url"`
	CoverImage        string   `json:"cover_image"`
	PublishedAt       string   `json:"published_at"`
	ReadingTimeMin    int      `json:"reading_time_minutes"`
	PositiveReactions int      `json:"positive_reactions_count"`
	CommentsCount     int      `json:"comments_count"`
	TagList           []string `json:"tag_list"`
	User              struct {
		Name     string `json:"name"`
		Username string `json:"username"`
	} `json:"user"`
}

func (d *DevToPoller) Poll(ctx context.Context, since time.Time) ([]*RawEvent, error) {
	seen := map[int]bool{}
	var events []*RawEvent

	// Fetch articles by tag.
	for _, tag := range d.tags {
		if ctx.Err() != nil {
			break
		}
		articles, err := d.fetchArticles(ctx, fmt.Sprintf("https://dev.to/api/articles?tag=%s&per_page=20&state=fresh", tag))
		if err != nil {
			d.logger.Warn("devto: fetch by tag failed", "tag", tag, "error", err)
			continue
		}
		for _, a := range articles {
			if seen[a.ID] {
				continue
			}
			seen[a.ID] = true
			published := parseDevToTime(a.PublishedAt)
			if published.Before(since) {
				continue
			}
			events = append(events, d.articleToEvent(a))
		}
	}

	// Fetch own articles.
	if d.username != "" && ctx.Err() == nil {
		articles, err := d.fetchArticles(ctx, fmt.Sprintf("https://dev.to/api/articles?username=%s&per_page=10", d.username))
		if err != nil {
			d.logger.Warn("devto: fetch by user failed", "user", d.username, "error", err)
		} else {
			for _, a := range articles {
				if seen[a.ID] {
					continue
				}
				seen[a.ID] = true
				published := parseDevToTime(a.PublishedAt)
				if published.Before(since) {
					continue
				}
				events = append(events, d.articleToEvent(a))
			}
		}
	}

	return events, nil
}

func (d *DevToPoller) fetchArticles(ctx context.Context, url string) ([]devtoArticle, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("devto: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}

	var articles []devtoArticle
	if err := json.Unmarshal(body, &articles); err != nil {
		return nil, fmt.Errorf("devto: parse: %w", err)
	}
	return articles, nil
}

func (d *DevToPoller) articleToEvent(a devtoArticle) *RawEvent {
	desc := a.Description
	if len(desc) > 200 {
		desc = desc[:200] + "..."
	}
	return &RawEvent{
		Source:   SourceDevTo,
		SourceID: fmt.Sprintf("devto:%d", a.ID),
		Kind:     KindPost,
		Title:    a.Title,
		Body:     desc,
		URL:      a.URL,
		Actor:    a.User.Name,
		GroupKey: "devto",
		Metadata: map[string]any{
			"tags":        a.TagList,
			"reactions":   a.PositiveReactions,
			"comments":    a.CommentsCount,
			"readingTime": a.ReadingTimeMin,
			"coverImage":  a.CoverImage,
			"author":      a.User.Username,
		},
		OccurredAt: parseDevToTime(a.PublishedAt),
	}
}

func parseDevToTime(s string) time.Time {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse("2006-01-02T15:04:05Z", s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}
