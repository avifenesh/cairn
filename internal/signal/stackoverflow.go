package signal

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// SOConfig configures the Stack Overflow poller.
type SOConfig struct {
	Tags   []string // tags to monitor (e.g. "go", "svelte", "sqlite")
	APIKey string   // optional, for higher rate limit (10K/day vs 300/day)
	Logger *slog.Logger
}

// SOPoller fetches recent questions from Stack Overflow by tag.
type SOPoller struct {
	tags   []string
	apiKey string
	client *http.Client
	logger *slog.Logger
}

// NewSOPoller creates a Stack Overflow poller.
func NewSOPoller(cfg SOConfig) *SOPoller {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &SOPoller{
		tags:   cfg.Tags,
		apiKey: cfg.APIKey,
		client: &http.Client{Timeout: 30 * time.Second},
		logger: logger,
	}
}

func (s *SOPoller) Source() string { return SourceStackOverflow }

func (s *SOPoller) Poll(ctx context.Context, since time.Time) ([]*RawEvent, error) {
	if len(s.tags) == 0 {
		return nil, nil
	}

	tagged := strings.Join(s.tags, ";")
	url := fmt.Sprintf(
		"https://api.stackexchange.com/2.3/questions?tagged=%s&sort=creation&order=desc&site=stackoverflow&fromdate=%d&pagesize=20",
		tagged, since.Unix(),
	)
	if s.apiKey != "" {
		url += "&key=" + s.apiKey
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stackoverflow: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stackoverflow: status %d", resp.StatusCode)
	}

	// SO API returns gzip by default.
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("stackoverflow: gzip decode: %w", err)
		}
		defer gr.Close()
		reader = gr
	}

	body, err := io.ReadAll(io.LimitReader(reader, 2<<20))
	if err != nil {
		return nil, err
	}

	var result struct {
		Items []struct {
			QuestionID  int      `json:"question_id"`
			Title       string   `json:"title"`
			Link        string   `json:"link"`
			Tags        []string `json:"tags"`
			Score       int      `json:"score"`
			AnswerCount int      `json:"answer_count"`
			ViewCount   int      `json:"view_count"`
			IsAnswered  bool     `json:"is_answered"`
			Owner       struct {
				DisplayName string `json:"display_name"`
				Link        string `json:"link"`
			} `json:"owner"`
			CreationDate int64 `json:"creation_date"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("stackoverflow: parse: %w", err)
	}

	var events []*RawEvent
	for _, q := range result.Items {
		events = append(events, &RawEvent{
			Source:   SourceStackOverflow,
			SourceID: fmt.Sprintf("so:%d", q.QuestionID),
			Kind:     KindPost,
			Title:    q.Title,
			URL:      q.Link,
			Actor:    q.Owner.DisplayName,
			GroupKey: "stackoverflow",
			Metadata: map[string]any{
				"tags":        q.Tags,
				"score":       q.Score,
				"answerCount": q.AnswerCount,
				"viewCount":   q.ViewCount,
				"isAnswered":  q.IsAnswered,
			},
			OccurredAt: time.Unix(q.CreationDate, 0).UTC(),
		})
	}

	return events, nil
}
