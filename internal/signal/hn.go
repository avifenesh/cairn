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

// HNPoller fetches top and new stories from Hacker News via the Firebase API.
// Filters by keywords and minimum score.
type HNPoller struct {
	keywords []string
	minScore int
	client   *http.Client
}

// HNConfig holds configuration for the Hacker News poller.
type HNConfig struct {
	Keywords []string // Keywords to filter stories by (case-insensitive, match title)
	MinScore int      // Minimum score threshold (0 = no filter)
}

const hnBaseURL = "https://hacker-news.firebaseio.com/v0"

// NewHNPoller creates a Hacker News poller. Keywords are optional - if empty, all
// top stories above minScore are included.
func NewHNPoller(cfg HNConfig) *HNPoller {
	// Normalize keywords to lowercase.
	kw := make([]string, len(cfg.Keywords))
	for i, k := range cfg.Keywords {
		kw[i] = strings.ToLower(strings.TrimSpace(k))
	}
	return &HNPoller{
		keywords: kw,
		minScore: cfg.MinScore,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (h *HNPoller) Source() string { return "hn" }

func (h *HNPoller) Poll(ctx context.Context, since time.Time) ([]*RawEvent, error) {
	// Fetch top story IDs.
	ids, err := h.fetchStoryIDs(ctx, "topstories")
	if err != nil {
		return nil, fmt.Errorf("hn: fetch top stories: %w", err)
	}

	// Limit to first 50 to avoid excessive API calls.
	if len(ids) > 50 {
		ids = ids[:50]
	}

	var events []*RawEvent
	for _, id := range ids {
		story, err := h.fetchItem(ctx, id)
		if err != nil {
			continue // skip individual failures
		}

		// Filter: must be after since.
		storyTime := time.Unix(story.Time, 0)
		if storyTime.Before(since) {
			continue
		}

		// Filter: minimum score.
		if h.minScore > 0 && story.Score < h.minScore {
			continue
		}

		// Filter: keyword match (if keywords configured).
		if len(h.keywords) > 0 && !h.matchesKeywords(story) {
			continue
		}

		url := story.URL
		if url == "" {
			url = fmt.Sprintf("https://news.ycombinator.com/item?id=%d", story.ID)
		}

		events = append(events, &RawEvent{
			Source:   "hn",
			SourceID: fmt.Sprintf("story:%d", story.ID),
			Kind:     "story",
			Title:    story.Title,
			Body:     "", // HN stories don't have body in API
			URL:      url,
			Actor:    story.By,
			GroupKey: "hn",
			Metadata: map[string]any{
				"score":       story.Score,
				"descendants": story.Descendants,
				"hnURL":       fmt.Sprintf("https://news.ycombinator.com/item?id=%d", story.ID),
			},
			OccurredAt: storyTime,
		})
	}

	return events, nil
}

type hnItem struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	By          string `json:"by"`
	Score       int    `json:"score"`
	Time        int64  `json:"time"`
	Descendants int    `json:"descendants"`
}

func (h *HNPoller) fetchStoryIDs(ctx context.Context, endpoint string) ([]int, error) {
	url := fmt.Sprintf("%s/%s.json", hnBaseURL, endpoint)
	body, err := h.doGet(ctx, url)
	if err != nil {
		return nil, err
	}

	var ids []int
	if err := json.Unmarshal(body, &ids); err != nil {
		return nil, fmt.Errorf("hn: parse story ids: %w", err)
	}
	return ids, nil
}

func (h *HNPoller) fetchItem(ctx context.Context, id int) (*hnItem, error) {
	url := fmt.Sprintf("%s/item/%d.json", hnBaseURL, id)
	body, err := h.doGet(ctx, url)
	if err != nil {
		return nil, err
	}

	var item hnItem
	if err := json.Unmarshal(body, &item); err != nil {
		return nil, fmt.Errorf("hn: parse item %d: %w", id, err)
	}
	return &item, nil
}

func (h *HNPoller) doGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hn: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hn: status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (h *HNPoller) matchesKeywords(story *hnItem) bool {
	title := strings.ToLower(story.Title)
	url := strings.ToLower(story.URL)
	for _, kw := range h.keywords {
		if kw == "" {
			continue
		}
		if strings.Contains(title, kw) || strings.Contains(url, kw) {
			return true
		}
	}
	return false
}
