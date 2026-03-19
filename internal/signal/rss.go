package signal

import (
	"context"
	"crypto/md5"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/mmcdole/gofeed"
)

// RSSConfig configures the RSS/Atom feed poller.
type RSSConfig struct {
	Feeds  []string // feed URLs
	Logger *slog.Logger
}

// RSSPoller fetches articles from RSS/Atom/JSON feeds.
type RSSPoller struct {
	feeds  []string
	parser *gofeed.Parser
	client *http.Client
	logger *slog.Logger
}

// NewRSSPoller creates an RSS feed poller.
func NewRSSPoller(cfg RSSConfig) *RSSPoller {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &RSSPoller{
		feeds:  cfg.Feeds,
		parser: gofeed.NewParser(),
		client: &http.Client{Timeout: 30 * time.Second},
		logger: logger,
	}
}

func (r *RSSPoller) Source() string { return SourceRSS }

var releasePattern = regexp.MustCompile(`(?i)(release|changelog|v\d+\.\d+)`)

func (r *RSSPoller) Poll(ctx context.Context, since time.Time) ([]*RawEvent, error) {
	var events []*RawEvent

	for _, feedURL := range r.feeds {
		if ctx.Err() != nil {
			break
		}
		feed, err := r.parser.ParseURLWithContext(feedURL, ctx)
		if err != nil {
			r.logger.Warn("rss: failed to parse feed", "url", feedURL, "error", err)
			continue
		}

		feedHash := fmt.Sprintf("%x", md5.Sum([]byte(feedURL)))[:8]

		for _, item := range feed.Items {
			published := itemTime(item)
			if published.Before(since) {
				continue
			}

			guid := item.GUID
			if guid == "" {
				guid = item.Link
			}
			if guid == "" {
				continue
			}

			kind := KindPost
			if releasePattern.MatchString(item.Title) {
				kind = KindRelease
			}

			body := item.Description
			if len(body) > 200 {
				body = body[:200] + "..."
			}

			author := ""
			if item.Author != nil {
				author = item.Author.Name
			}

			var categories []string
			for _, c := range item.Categories {
				categories = append(categories, c)
			}

			events = append(events, &RawEvent{
				Source:   SourceRSS,
				SourceID: fmt.Sprintf("rss:%s:%s", feedHash, guid),
				Kind:     kind,
				Title:    item.Title,
				Body:     body,
				URL:      item.Link,
				Actor:    author,
				GroupKey: feed.Title,
				Metadata: map[string]any{
					"feedTitle":  feed.Title,
					"feedURL":    feedURL,
					"categories": categories,
				},
				OccurredAt: published,
			})
		}
	}

	return events, nil
}

func itemTime(item *gofeed.Item) time.Time {
	if item.PublishedParsed != nil {
		return item.PublishedParsed.UTC()
	}
	if item.UpdatedParsed != nil {
		return item.UpdatedParsed.UTC()
	}
	return time.Time{}
}
