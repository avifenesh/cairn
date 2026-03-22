package signal

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/avifenesh/cairn/internal/db"
	"github.com/avifenesh/cairn/internal/eventbus"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database.DB
}

// --- EventStore tests ---

func TestEventStore_IngestAndList(t *testing.T) {
	sqlDB := setupTestDB(t)
	store := NewEventStore(sqlDB)
	ctx := context.Background()

	events := []*RawEvent{
		{Source: "github", SourceID: "pr:1", Kind: "pr", Title: "Fix bug", URL: "https://github.com/a/b/pull/1", Actor: "alice", OccurredAt: time.Now().UTC()},
		{Source: "github", SourceID: "pr:2", Kind: "pr", Title: "Add feature", URL: "https://github.com/a/b/pull/2", Actor: "bob", OccurredAt: time.Now().UTC()},
		{Source: "hn", SourceID: "story:100", Kind: "story", Title: "Go is great", URL: "https://example.com", OccurredAt: time.Now().UTC()},
	}

	inserted, err := store.Ingest(ctx, events)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if len(inserted) != 3 {
		t.Errorf("inserted = %d, want 3", len(inserted))
	}

	// List all.
	all, err := store.List(ctx, EventFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("list count = %d, want 3", len(all))
	}

	// List by source.
	ghEvents, err := store.List(ctx, EventFilter{Source: "github"})
	if err != nil {
		t.Fatalf("list github: %v", err)
	}
	if len(ghEvents) != 2 {
		t.Errorf("github events = %d, want 2", len(ghEvents))
	}
}

func TestEventStore_Dedup(t *testing.T) {
	sqlDB := setupTestDB(t)
	store := NewEventStore(sqlDB)
	ctx := context.Background()

	ev := &RawEvent{Source: "github", SourceID: "pr:1", Kind: "pr", Title: "Fix bug", OccurredAt: time.Now().UTC()}

	// Insert once.
	n1, err := store.Ingest(ctx, []*RawEvent{ev})
	if err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	if len(n1) != 1 {
		t.Errorf("first insert = %d, want 1", len(n1))
	}

	// Insert same event again - should be deduped.
	n2, err := store.Ingest(ctx, []*RawEvent{ev})
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if len(n2) != 0 {
		t.Errorf("dedup insert = %d, want 0", len(n2))
	}

	// Verify only one row exists.
	count, err := store.Count(ctx, EventFilter{})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

// TestEventStore_Ingest_CrossSourceURLDedup verifies that the same article URL
// arriving from two different sources (e.g. devto + rss poller) is deduplicated.
func TestEventStore_Ingest_CrossSourceURLDedup(t *testing.T) {
	sqlDB := setupTestDB(t)
	store := NewEventStore(sqlDB)
	ctx := t.Context()

	events := []*RawEvent{
		{
			Source:     "devto",
			SourceID:   "devto:12345",
			Kind:       "post",
			Title:      "AI Agents Fail 97.5%",
			URL:        "https://dev.to/maxquimby/ai-agents-fail",
			OccurredAt: time.Now().UTC(),
		},
	}

	inserted, err := store.Ingest(ctx, events)
	if err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	if len(inserted) != 1 {
		t.Fatalf("first ingest: count = %d, want 1", len(inserted))
	}

	// Same URL, different source and source_item_id — should be deduped.
	events2 := []*RawEvent{
		{
			Source:     "rss",
			SourceID:   "rss:f7d668fd:https://dev.to/maxquimby/ai-agents-fail",
			Kind:       "post",
			Title:      "AI Agents Fail 97.5%",
			URL:        "https://dev.to/maxquimby/ai-agents-fail",
			OccurredAt: time.Now().UTC(),
		},
	}

	inserted2, err := store.Ingest(ctx, events2)
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if len(inserted2) != 0 {
		t.Fatalf("second ingest: count = %d, want 0 (cross-source dedup)", len(inserted2))
	}
}

// TestEventStore_Ingest_CrossSourceURLDedup_NPMDoesNotBlockGitHub verifies that
// same-URL events from non-article sources (npm, github) are NOT cross-deduped.
// A github star event and an npm publish event can share the same base URL
// (e.g. npmjs.com/package/express) but represent distinct events.
func TestEventStore_Ingest_CrossSourceURLDedup_NPMDoesNotBlockGitHub(t *testing.T) {
	sqlDB := setupTestDB(t)
	store := NewEventStore(sqlDB)
	ctx := t.Context()

	events := []*RawEvent{
		{
			Source:     "npm",
			SourceID:   "npm:express:5.1.0",
			Kind:       "version",
			Title:      "express 5.1.0 published",
			URL:        "https://www.npmjs.com/package/express",
			OccurredAt: time.Now().UTC(),
		},
	}
	inserted, err := store.Ingest(ctx, events)
	if err != nil {
		t.Fatalf("npm ingest: %v", err)
	}
	if len(inserted) != 1 {
		t.Fatalf("npm ingest: count = %d, want 1", len(inserted))
	}

	// Same URL from github — should NOT be deduped.
	events2 := []*RawEvent{
		{
			Source:     "github",
			SourceID:   "gh:star:expressjs/express",
			Kind:       "star",
			Title:      "express repo starred",
			URL:        "https://www.npmjs.com/package/express",
			OccurredAt: time.Now().UTC(),
		},
	}
	inserted2, err := store.Ingest(ctx, events2)
	if err != nil {
		t.Fatalf("github ingest: %v", err)
	}
	if len(inserted2) != 1 {
		t.Fatalf("github ingest: count = %d, want 1 (non-article sources should not cross-dedup)", len(inserted2))
	}

	count, err := store.Count(ctx, EventFilter{})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("total count = %d, want 2", count)
	}
}

// TestEventStore_Ingest_CrossSourceURLDedup_DevToDoesNotBlockGitHub verifies that
// a devto article URL does not block a github event with the same URL (e.g. a
// devto article linking to a github repo). Only article↔article dedup should apply.
func TestEventStore_Ingest_CrossSourceURLDedup_DevToDoesNotBlockGitHub(t *testing.T) {
	sqlDB := setupTestDB(t)
	store := NewEventStore(sqlDB)
	ctx := t.Context()

	events := []*RawEvent{
		{
			Source:     "devto",
			SourceID:   "devto:99999",
			Kind:       "post",
			Title:      "Some Dev.to Article",
			URL:        "https://github.com/expressjs/express",
			OccurredAt: time.Now().UTC(),
		},
	}
	inserted, err := store.Ingest(ctx, events)
	if err != nil {
		t.Fatalf("devto ingest: %v", err)
	}
	if len(inserted) != 1 {
		t.Fatalf("devto ingest: count = %d, want 1", len(inserted))
	}

	// Same URL from github — should NOT be deduped (github is not an article source).
	events2 := []*RawEvent{
		{
			Source:     "github",
			SourceID:   "gh:star:expressjs/express",
			Kind:       "star",
			Title:      "express starred",
			URL:        "https://github.com/expressjs/express",
			OccurredAt: time.Now().UTC(),
		},
	}
	inserted2, err := store.Ingest(ctx, events2)
	if err != nil {
		t.Fatalf("github ingest: %v", err)
	}
	if len(inserted2) != 1 {
		t.Fatalf("github ingest: count = %d, want 1 (github should not be deduped against devto URL)", len(inserted2))
	}

	count, err := store.Count(ctx, EventFilter{})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("total count = %d, want 2", count)
	}
}

// TestEventStore_Ingest_IntraBatchURLDedup verifies that duplicate URLs
// within a single batch (e.g. same dev.to article from multiple RSS feeds)
// are deduplicated.
func TestEventStore_Ingest_IntraBatchURLDedup(t *testing.T) {
	sqlDB := setupTestDB(t)
	store := NewEventStore(sqlDB)
	ctx := t.Context()

	events := []*RawEvent{
		{
			Source:     "rss",
			SourceID:   "rss:feed1:https://dev.to/article",
			Kind:       "post",
			Title:      "Article",
			URL:        "https://dev.to/article",
			OccurredAt: time.Now().UTC(),
		},
		{
			Source:     "rss",
			SourceID:   "rss:feed2:https://dev.to/article",
			Kind:       "post",
			Title:      "Article",
			URL:        "https://dev.to/article",
			OccurredAt: time.Now().UTC(),
		},
	}

	inserted, err := store.Ingest(ctx, events)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if len(inserted) != 1 {
		t.Fatalf("ingest: count = %d, want 1 (intra-batch dedup)", len(inserted))
	}
}
func TestEventStore_MarkRead(t *testing.T) {
	sqlDB := setupTestDB(t)
	store := NewEventStore(sqlDB)
	ctx := context.Background()

	ev := &RawEvent{Source: "test", SourceID: "1", Kind: "test", Title: "Test", OccurredAt: time.Now().UTC()}
	store.Ingest(ctx, []*RawEvent{ev})

	all, _ := store.List(ctx, EventFilter{})
	id := all[0].ID

	// Should be unread.
	unread, _ := store.Count(ctx, EventFilter{UnreadOnly: true})
	if unread != 1 {
		t.Errorf("unread = %d, want 1", unread)
	}

	// Mark read.
	if err := store.MarkRead(ctx, id); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	// Should now be read.
	unread, _ = store.Count(ctx, EventFilter{UnreadOnly: true})
	if unread != 0 {
		t.Errorf("unread after mark = %d, want 0", unread)
	}
}

func TestEventStore_MarkAllRead(t *testing.T) {
	sqlDB := setupTestDB(t)
	store := NewEventStore(sqlDB)
	ctx := context.Background()

	events := []*RawEvent{
		{Source: "test", SourceID: "1", Kind: "test", Title: "A", OccurredAt: time.Now().UTC()},
		{Source: "test", SourceID: "2", Kind: "test", Title: "B", OccurredAt: time.Now().UTC()},
		{Source: "test", SourceID: "3", Kind: "test", Title: "C", OccurredAt: time.Now().UTC()},
	}
	store.Ingest(ctx, events)

	n, err := store.MarkAllRead(ctx)
	if err != nil {
		t.Fatalf("mark all read: %v", err)
	}
	if n != 3 {
		t.Errorf("marked = %d, want 3", n)
	}

	unread, _ := store.Count(ctx, EventFilter{UnreadOnly: true})
	if unread != 0 {
		t.Errorf("unread = %d, want 0", unread)
	}
}

func TestEventStore_Delete(t *testing.T) {
	sqlDB := setupTestDB(t)
	store := NewEventStore(sqlDB)
	ctx := context.Background()

	old := time.Now().UTC().Add(-48 * time.Hour)
	recent := time.Now().UTC()

	events := []*RawEvent{
		{Source: "test", SourceID: "old", Kind: "test", Title: "Old", OccurredAt: old},
		{Source: "test", SourceID: "new", Kind: "test", Title: "New", OccurredAt: recent},
	}
	store.Ingest(ctx, events)

	deleted, err := store.Delete(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	remaining, _ := store.Count(ctx, EventFilter{})
	if remaining != 1 {
		t.Errorf("remaining = %d, want 1", remaining)
	}
}

func TestEventStore_ListByKind(t *testing.T) {
	sqlDB := setupTestDB(t)
	store := NewEventStore(sqlDB)
	ctx := context.Background()

	events := []*RawEvent{
		{Source: "github", SourceID: "pr:1", Kind: "pr", Title: "PR 1", OccurredAt: time.Now().UTC()},
		{Source: "github", SourceID: "issue:1", Kind: "issue", Title: "Issue 1", OccurredAt: time.Now().UTC()},
		{Source: "github", SourceID: "pr:2", Kind: "pr", Title: "PR 2", OccurredAt: time.Now().UTC()},
	}
	store.Ingest(ctx, events)

	prs, err := store.List(ctx, EventFilter{Kind: "pr"})
	if err != nil {
		t.Fatalf("list prs: %v", err)
	}
	if len(prs) != 2 {
		t.Errorf("prs = %d, want 2", len(prs))
	}
}

func TestEventStore_Metadata(t *testing.T) {
	sqlDB := setupTestDB(t)
	store := NewEventStore(sqlDB)
	ctx := context.Background()

	ev := &RawEvent{
		Source: "hn", SourceID: "story:42", Kind: "story", Title: "Test",
		Metadata:   map[string]any{"score": float64(150), "by": "dang"},
		OccurredAt: time.Now().UTC(),
	}
	store.Ingest(ctx, []*RawEvent{ev})

	all, _ := store.List(ctx, EventFilter{})
	if len(all) != 1 {
		t.Fatalf("count = %d, want 1", len(all))
	}
	if score, ok := all[0].Metadata["score"].(float64); !ok || score != 150 {
		t.Errorf("metadata score = %v, want 150", all[0].Metadata["score"])
	}
}

// --- SourceState tests ---

func TestSourceState_GetSetLastPoll(t *testing.T) {
	sqlDB := setupTestDB(t)
	state := NewSourceState(sqlDB)
	ctx := context.Background()

	// Should be zero before any poll.
	last, err := state.GetLastPoll(ctx, "github")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !last.IsZero() {
		t.Errorf("initial last poll = %v, want zero", last)
	}

	// Set and verify.
	now := time.Now().UTC().Truncate(time.Second)
	if err := state.SetLastPoll(ctx, "github", now); err != nil {
		t.Fatalf("set: %v", err)
	}

	last, err = state.GetLastPoll(ctx, "github")
	if err != nil {
		t.Fatalf("get after set: %v", err)
	}
	if !last.Equal(now) {
		t.Errorf("last poll = %v, want %v", last, now)
	}
}

func TestSourceState_CursorAndPoll(t *testing.T) {
	sqlDB := setupTestDB(t)
	state := NewSourceState(sqlDB)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	if err := state.SetCursorAndPoll(ctx, "github", "page2", now); err != nil {
		t.Fatalf("set cursor: %v", err)
	}

	cursor, err := state.GetCursor(ctx, "github")
	if err != nil {
		t.Fatalf("get cursor: %v", err)
	}
	if cursor != "page2" {
		t.Errorf("cursor = %q, want %q", cursor, "page2")
	}

	last, _ := state.GetLastPoll(ctx, "github")
	if !last.Equal(now) {
		t.Errorf("last poll = %v, want %v", last, now)
	}
}

// --- Scheduler tests ---

func TestScheduler_PollNow(t *testing.T) {
	sqlDB := setupTestDB(t)
	store := NewEventStore(sqlDB)
	state := NewSourceState(sqlDB)
	bus := eventbus.New()
	defer bus.Close()

	scheduler := NewScheduler(store, state, bus, nil)
	scheduler.Register(&fakePoller{source: "test", events: []*RawEvent{
		{Source: "test", SourceID: "1", Kind: "test", Title: "Event 1", OccurredAt: time.Now().UTC()},
		{Source: "test", SourceID: "2", Kind: "test", Title: "Event 2", OccurredAt: time.Now().UTC()},
	}}, 5*time.Minute)

	scheduler.PollNow(context.Background())

	count, _ := store.Count(context.Background(), EventFilter{})
	if count != 2 {
		t.Errorf("events after poll = %d, want 2", count)
	}

	// Poll again - should dedup.
	scheduler.PollNow(context.Background())
	count, _ = store.Count(context.Background(), EventFilter{})
	if count != 2 {
		t.Errorf("events after second poll = %d, want 2 (dedup)", count)
	}
}

func TestScheduler_BackoffOnError(t *testing.T) {
	sqlDB := setupTestDB(t)
	store := NewEventStore(sqlDB)
	state := NewSourceState(sqlDB)

	scheduler := NewScheduler(store, state, nil, nil)
	scheduler.Register(&fakePoller{source: "fail", err: fmt.Errorf("connection refused")}, 5*time.Minute)

	// Poll should not panic.
	scheduler.PollNow(context.Background())

	// Verify backoff is set.
	if scheduler.pollers[0].backoff != initialBackoff {
		t.Errorf("backoff = %v, want %v", scheduler.pollers[0].backoff, initialBackoff)
	}

	// Poll again - backoff should double.
	scheduler.PollNow(context.Background())
	if scheduler.pollers[0].backoff != initialBackoff*2 {
		t.Errorf("backoff = %v, want %v", scheduler.pollers[0].backoff, initialBackoff*2)
	}
}

func TestScheduler_BackoffResetOnSuccess(t *testing.T) {
	sqlDB := setupTestDB(t)
	store := NewEventStore(sqlDB)
	state := NewSourceState(sqlDB)

	fp := &fakePoller{source: "flaky", err: fmt.Errorf("temporary error")}
	scheduler := NewScheduler(store, state, nil, nil)
	scheduler.Register(fp, 5*time.Minute)

	// First poll fails.
	scheduler.PollNow(context.Background())
	if scheduler.pollers[0].backoff == 0 {
		t.Fatal("expected non-zero backoff after failure")
	}

	// Fix the poller and poll again.
	fp.err = nil
	fp.events = []*RawEvent{{Source: "flaky", SourceID: "1", Kind: "test", Title: "OK", OccurredAt: time.Now().UTC()}}
	scheduler.PollNow(context.Background())

	if scheduler.pollers[0].backoff != 0 {
		t.Errorf("backoff = %v, want 0 after success", scheduler.pollers[0].backoff)
	}
}

func TestScheduler_BusEvents(t *testing.T) {
	sqlDB := setupTestDB(t)
	store := NewEventStore(sqlDB)
	state := NewSourceState(sqlDB)
	bus := eventbus.New()
	defer bus.Close()

	got := make(chan eventbus.EventIngested, 10)
	eventbus.Subscribe(bus, func(e eventbus.EventIngested) {
		got <- e
	})

	scheduler := NewScheduler(store, state, bus, nil)
	scheduler.Register(&fakePoller{source: "test", events: []*RawEvent{
		{Source: "test", SourceID: "bus:1", Kind: "test", Title: "Bus Event", URL: "https://example.com", OccurredAt: time.Now().UTC()},
	}}, 5*time.Minute)

	// First poll - should publish 1 event.
	scheduler.PollNow(context.Background())

	select {
	case ev := <-got:
		if ev.Title != "Bus Event" {
			t.Errorf("title = %q, want %q", ev.Title, "Bus Event")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for bus event after first poll")
	}

	// Second poll with same data - should NOT publish (dedup).
	scheduler.PollNow(context.Background())

	select {
	case ev := <-got:
		t.Errorf("unexpected bus event after second poll (dedup should prevent): %+v", ev)
	case <-time.After(100 * time.Millisecond):
		// Good - no event published.
	}
}

func TestScheduler_StartAndClose(t *testing.T) {
	sqlDB := setupTestDB(t)
	store := NewEventStore(sqlDB)
	state := NewSourceState(sqlDB)

	scheduler := NewScheduler(store, state, nil, nil)
	scheduler.Register(&fakePoller{source: "test", events: []*RawEvent{
		{Source: "test", SourceID: "start:1", Kind: "test", Title: "Startup", OccurredAt: time.Now().UTC()},
	}}, 100*time.Millisecond) // short interval for test

	scheduler.Start()

	// Wait for at least one poll cycle.
	time.Sleep(200 * time.Millisecond)

	scheduler.Close()

	count, _ := store.Count(context.Background(), EventFilter{})
	if count < 1 {
		t.Errorf("events after start/close = %d, want >= 1", count)
	}
}

// --- HN poller tests (with fake HTTP server) ---

func newHNTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0/topstories.json":
			w.Write([]byte("[1, 2, 3]"))
		case "/v0/item/1.json":
			fmt.Fprintf(w, `{"id":1,"title":"Go 1.25 released","url":"https://go.dev","by":"rob","score":200,"time":%d,"type":"story"}`, time.Now().Unix())
		case "/v0/item/2.json":
			fmt.Fprintf(w, `{"id":2,"title":"Python is slow","url":"https://python.org","by":"guido","score":100,"time":%d,"type":"story"}`, time.Now().Unix())
		case "/v0/item/3.json":
			fmt.Fprintf(w, `{"id":3,"title":"Rust async explained","url":"https://rust-lang.org","by":"niko","score":150,"time":%d,"type":"story"}`, time.Now().Unix())
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestHNPoller_Poll(t *testing.T) {
	srv := newHNTestServer()
	defer srv.Close()

	poller := NewHNPoller(HNConfig{
		Keywords: []string{"go", "rust"},
		MinScore: 50,
	})
	poller.baseURL = srv.URL + "/v0"

	events, err := poller.Poll(context.Background(), time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}

	// Should match "Go 1.25 released" and "Rust async explained" (keyword match).
	// "Python is slow" should be filtered out (no keyword match).
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}

	titles := map[string]bool{}
	for _, ev := range events {
		titles[ev.Title] = true
		if ev.Source != SourceHN {
			t.Errorf("source = %q, want %q", ev.Source, SourceHN)
		}
		if ev.Kind != KindStory {
			t.Errorf("kind = %q, want %q", ev.Kind, KindStory)
		}
	}
	if !titles["Go 1.25 released"] {
		t.Error("missing 'Go 1.25 released'")
	}
	if !titles["Rust async explained"] {
		t.Error("missing 'Rust async explained'")
	}
}

func TestHNPoller_KeywordMatching(t *testing.T) {
	poller := NewHNPoller(HNConfig{Keywords: []string{"go", "rust"}, MinScore: 50})

	if !poller.matchesKeywords(&hnItem{Title: "Go 1.25 released"}) {
		t.Error("expected 'Go 1.25 released' to match keyword 'go'")
	}
	if poller.matchesKeywords(&hnItem{Title: "Python is slow"}) {
		t.Error("expected 'Python is slow' to NOT match keywords [go, rust]")
	}
	if !poller.matchesKeywords(&hnItem{Title: "Rust async explained"}) {
		t.Error("expected 'Rust async explained' to match keyword 'rust'")
	}
	if !poller.matchesKeywords(&hnItem{Title: "Something", URL: "https://go.dev"}) {
		t.Error("expected URL match for 'go'")
	}
}

func TestHNPoller_MinScoreOnly(t *testing.T) {
	srv := newHNTestServer()
	defer srv.Close()

	poller := NewHNPoller(HNConfig{MinScore: 150})
	poller.baseURL = srv.URL + "/v0"

	events, err := poller.Poll(context.Background(), time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}

	// No keywords - all stories above score 150 pass: Go (200) and Rust (150).
	if len(events) != 2 {
		t.Errorf("events = %d, want 2 (score >= 150)", len(events))
	}
}

// --- GitHub poller tests ---

func TestGitHubPoller_APIToHTML(t *testing.T) {
	tests := []struct {
		apiURL   string
		repo     string
		expected string
	}{
		{"https://api.github.com/repos/org/repo/pulls/42", "org/repo", "https://github.com/org/repo/pull/42"},
		{"https://api.github.com/repos/org/repo/issues/10", "org/repo", "https://github.com/org/repo/issues/10"},
		{"", "org/repo", "https://github.com/org/repo"},
		{"", "", ""},
	}

	for _, tt := range tests {
		got := ghAPIToHTML(tt.apiURL, tt.repo)
		if got != tt.expected {
			t.Errorf("ghAPIToHTML(%q, %q) = %q, want %q", tt.apiURL, tt.repo, got, tt.expected)
		}
	}
}

func TestGitHubPoller_SubjectTypeToKind(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"PullRequest", "pr"},
		{"Issue", "issue"},
		{"Release", "release"},
		{"Discussion", "discussion"},
		{"Commit", "commit"},
		{"Unknown", "unknown"},
	}

	for _, tt := range tests {
		got := ghSubjectTypeToKind(tt.input)
		if got != tt.expected {
			t.Errorf("ghSubjectTypeToKind(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestGitHubPoller_FetchNotifications(t *testing.T) {
	// The GitHub poller hardcodes api.github.com URLs internally,
	// so we test the doGet + JSON parsing via a mock server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `[{
			"id": "1",
			"reason": "review_requested",
			"unread": true,
			"updated_at": "2026-03-17T12:00:00Z",
			"subject": {"title": "Fix memory leak", "url": "https://api.github.com/repos/org/repo/pulls/42", "type": "PullRequest"},
			"repository": {"full_name": "org/repo", "html_url": "https://github.com/org/repo"}
		}]`)
	}))
	defer srv.Close()

	poller := NewGitHubPoller(GitHubConfig{Token: "test-token"})

	// Test doGet directly against mock server.
	body, err := poller.doGet(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("doGet: %v", err)
	}

	var notifs []ghNotification
	if err := json.Unmarshal(body, &notifs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(notifs) != 1 {
		t.Fatalf("notifs = %d, want 1", len(notifs))
	}
	if notifs[0].Subject.Title != "Fix memory leak" {
		t.Errorf("title = %q, want %q", notifs[0].Subject.Title, "Fix memory leak")
	}
	if notifs[0].Subject.Type != "PullRequest" {
		t.Errorf("type = %q, want %q", notifs[0].Subject.Type, "PullRequest")
	}
}

func TestGitHubPoller_NoToken(t *testing.T) {
	poller := NewGitHubPoller(GitHubConfig{})
	_, err := poller.Poll(context.Background(), time.Now())
	if err == nil {
		t.Error("expected error when token is empty")
	}
}

// --- Helpers ---

type fakePoller struct {
	source string
	events []*RawEvent
	err    error
}

func (f *fakePoller) Source() string { return f.source }
func (f *fakePoller) Poll(_ context.Context, _ time.Time) ([]*RawEvent, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.events, nil
}
