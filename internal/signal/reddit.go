package signal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RedditPoller fetches new posts from configured subreddits via the public JSON API.
type RedditPoller struct {
	subreddits []string
	client     *http.Client
}

// RedditConfig holds configuration for the Reddit poller.
type RedditConfig struct {
	Subreddits []string // Subreddit names to monitor (without r/ prefix)
}

// NewRedditPoller creates a Reddit poller. Uses the public JSON API (no auth needed,
// rate limited to ~10 req/min for anonymous access).
func NewRedditPoller(cfg RedditConfig) *RedditPoller {
	return &RedditPoller{
		subreddits: cfg.Subreddits,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (r *RedditPoller) Source() string { return SourceReddit }

func (r *RedditPoller) Poll(ctx context.Context, since time.Time) ([]*RawEvent, error) {
	var all []*RawEvent
	for _, sub := range r.subreddits {
		posts, err := r.fetchSubreddit(ctx, sub, since)
		if err != nil {
			continue
		}
		all = append(all, posts...)
	}
	return all, nil
}

type redditListing struct {
	Data struct {
		Children []struct {
			Data struct {
				ID        string  `json:"id"`
				Title     string  `json:"title"`
				Selftext  string  `json:"selftext"`
				URL       string  `json:"url"`
				Permalink string  `json:"permalink"`
				Author    string  `json:"author"`
				Subreddit string  `json:"subreddit"`
				Score     int     `json:"score"`
				Created   float64 `json:"created_utc"`
			} `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

func (r *RedditPoller) fetchSubreddit(ctx context.Context, subreddit string, since time.Time) ([]*RawEvent, error) {
	url := fmt.Sprintf("https://www.reddit.com/r/%s/new.json?limit=25", subreddit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "cairn/1.0 (signal poller)")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reddit: request r/%s: %w", subreddit, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reddit: r/%s status %d", subreddit, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, fmt.Errorf("reddit: read r/%s: %w", subreddit, err)
	}

	var listing redditListing
	if err := json.Unmarshal(body, &listing); err != nil {
		return nil, fmt.Errorf("reddit: parse r/%s: %w", subreddit, err)
	}

	var events []*RawEvent
	for _, child := range listing.Data.Children {
		post := child.Data
		postTime := time.Unix(int64(post.Created), 0)
		if postTime.Before(since) {
			continue
		}

		events = append(events, &RawEvent{
			Source:   SourceReddit,
			SourceID: fmt.Sprintf("post:%s", post.ID),
			Kind:     KindPost,
			Title:    post.Title,
			Body:     truncate(post.Selftext, 500),
			URL:      "https://www.reddit.com" + post.Permalink,
			Actor:    post.Author,
			GroupKey: fmt.Sprintf("r/%s", post.Subreddit),
			Metadata: map[string]any{
				"score":     post.Score,
				"subreddit": post.Subreddit,
			},
			OccurredAt: postTime,
		})
	}
	return events, nil
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
