// Package signal implements the Signal Plane: source polling, event ingestion,
// deduplication, and digest generation.
package signal

import (
	"context"
	"time"
)

// Source name constants.
const (
	SourceGitHub  = "github"
	SourceHN      = "hn"
	SourceReddit  = "reddit"
	SourceNPM     = "npm"
	SourceCrates  = "crates"
	SourceWebhook = "webhook"
)

// Event kind constants.
const (
	KindPR         = "pr"
	KindIssue      = "issue"
	KindRelease    = "release"
	KindDiscussion = "discussion"
	KindCommit     = "commit"
	KindPush       = "push"
	KindBranch     = "branch"
	KindFork       = "fork"
	KindStar       = "star"
	KindStory      = "story"
	KindPost       = "post"
	KindPackage    = "package"
	KindWebhook    = "webhook"
	KindComment    = "comment"
	KindFollow     = "follow"
	KindMetrics    = "metrics"
	KindNewRepo    = "new_repo"

	SourceGitHubSignal = "github_signal"
)

// Poller fetches new events from an external source.
type Poller interface {
	// Source returns the unique source identifier (e.g. "github", "hn").
	Source() string

	// Poll fetches events that occurred after since. Implementations must
	// handle their own pagination and rate limiting.
	Poll(ctx context.Context, since time.Time) ([]*RawEvent, error)
}

// RawEvent is a normalized event from any source before storage.
type RawEvent struct {
	Source     string         `json:"source"`
	SourceID   string         `json:"sourceId"` // dedup key within source
	Kind       string         `json:"kind"`     // pr, issue, comment, email, post, release, story
	Title      string         `json:"title"`
	Body       string         `json:"body"`
	URL        string         `json:"url"`
	Actor      string         `json:"actor"`
	Repo       string         `json:"repo"`
	GroupKey   string         `json:"groupKey"` // for grouping related events
	Metadata   map[string]any `json:"metadata"`
	OccurredAt time.Time      `json:"occurredAt"`
}

// StoredEvent is a persisted event with DB-assigned fields.
type StoredEvent struct {
	ID           string         `json:"id"`
	Source       string         `json:"source"`
	SourceItemID string         `json:"sourceItemId"`
	Kind         string         `json:"kind"`
	Title        string         `json:"title"`
	Body         string         `json:"body"`
	URL          string         `json:"url"`
	Actor        string         `json:"actor"`
	GroupKey     string         `json:"groupKey"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"createdAt"`
	ReadAt       *time.Time     `json:"readAt,omitempty"`
	ArchivedAt   *time.Time     `json:"archivedAt,omitempty"`
}

// EventFilter controls which events to list.
type EventFilter struct {
	Source          string // filter by source
	Kind            string // filter by kind
	UnreadOnly      bool   // only events with read_at IS NULL
	ExcludeArchived bool   // only events with archived_at IS NULL
	Limit           int    // max results (0 = default 50)
	Before          string // cursor: events before this ID (for pagination)
}
