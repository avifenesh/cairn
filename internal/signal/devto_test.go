package signal

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDevToPoller_Source(t *testing.T) {
	p := NewDevToPoller(DevToConfig{Tags: []string{"go"}})
	if p.Source() != SourceDevTo {
		t.Errorf("source = %q, want %q", p.Source(), SourceDevTo)
	}
}

func TestDevToPoller_PollByTag(t *testing.T) {
	now := time.Now().UTC()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tag") != "go" {
			http.Error(w, "unexpected tag", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `[{
			"id": 101,
			"title": "Go Concurrency Patterns",
			"description": "Deep dive into channels",
			"url": "https://dev.to/alice/go-concurrency",
			"published_at": %q,
			"reading_time_minutes": 8,
			"positive_reactions_count": 42,
			"comments_count": 5,
			"tag_list": ["go", "concurrency"],
			"user": {"name": "Alice", "username": "alice"}
		}]`, now.Format(time.RFC3339))
	}))
	defer srv.Close()

	poller := NewDevToPoller(DevToConfig{Tags: []string{"go"}, Logger: noopLogger()})
	poller.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	events, err := poller.Poll(context.Background(), now.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	e := events[0]
	if e.Source != SourceDevTo {
		t.Errorf("source = %q, want %q", e.Source, SourceDevTo)
	}
	if e.Kind != KindPost {
		t.Errorf("kind = %q, want %q", e.Kind, KindPost)
	}
	if e.Title != "Go Concurrency Patterns" {
		t.Errorf("title = %q", e.Title)
	}
	if e.Actor != "Alice" {
		t.Errorf("actor = %q, want Alice", e.Actor)
	}
	if e.URL != "https://dev.to/alice/go-concurrency" {
		t.Errorf("url = %q", e.URL)
	}
	if e.SourceID != "devto:101" {
		t.Errorf("sourceID = %q, want devto:101", e.SourceID)
	}
	if e.Metadata["reactions"] != 42 {
		t.Errorf("reactions = %v, want 42", e.Metadata["reactions"])
	}
	if e.Metadata["author"] != "alice" {
		t.Errorf("author = %v, want alice", e.Metadata["author"])
	}
}

func TestDevToPoller_PollByUsername(t *testing.T) {
	now := time.Now().UTC()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("username") == "" {
			w.Write([]byte("[]"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `[{
			"id": 201,
			"title": "My Article",
			"description": "Personal post",
			"url": "https://dev.to/bob/my-article",
			"published_at": %q,
			"reading_time_minutes": 3,
			"positive_reactions_count": 10,
			"comments_count": 2,
			"tag_list": ["webdev"],
			"user": {"name": "Bob", "username": "bob"}
		}]`, now.Format(time.RFC3339))
	}))
	defer srv.Close()

	poller := NewDevToPoller(DevToConfig{Username: "bob", Logger: noopLogger()})
	poller.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	events, err := poller.Poll(context.Background(), now.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Title != "My Article" {
		t.Errorf("title = %q", events[0].Title)
	}
}

func TestDevToPoller_DeduplicateWithinPoll(t *testing.T) {
	now := time.Now().UTC()
	article := fmt.Sprintf(`[{
		"id": 301,
		"title": "Shared Article",
		"description": "Appears in both",
		"url": "https://dev.to/shared",
		"published_at": %q,
		"reading_time_minutes": 5,
		"positive_reactions_count": 20,
		"comments_count": 3,
		"tag_list": ["go"],
		"user": {"name": "Carol", "username": "carol"}
	}]`, now.Format(time.RFC3339))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(article))
	}))
	defer srv.Close()

	poller := NewDevToPoller(DevToConfig{Tags: []string{"go"}, Username: "carol", Logger: noopLogger()})
	poller.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	events, err := poller.Poll(context.Background(), now.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1 (same article ID should be deduped)", len(events))
	}
}

func TestDevToPoller_SinceFilter(t *testing.T) {
	old := time.Now().UTC().Add(-48 * time.Hour)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `[{
			"id": 401,
			"title": "Old Post",
			"description": "Stale",
			"url": "https://dev.to/old",
			"published_at": %q,
			"reading_time_minutes": 1,
			"positive_reactions_count": 0,
			"comments_count": 0,
			"tag_list": [],
			"user": {"name": "Dan", "username": "dan"}
		}]`, old.Format(time.RFC3339))
	}))
	defer srv.Close()

	poller := NewDevToPoller(DevToConfig{Tags: []string{"go"}, Logger: noopLogger()})
	poller.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	events, err := poller.Poll(context.Background(), time.Now().UTC().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("events = %d, want 0 (old post should be filtered)", len(events))
	}
}

func TestDevToPoller_FetchError(t *testing.T) {
	now := time.Now().UTC()
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		tag := r.URL.Query().Get("tag")
		if tag == "fail" {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `[{
			"id": 501,
			"title": "Success Article",
			"description": "Works",
			"url": "https://dev.to/success",
			"published_at": %q,
			"reading_time_minutes": 2,
			"positive_reactions_count": 5,
			"comments_count": 1,
			"tag_list": ["ok"],
			"user": {"name": "Eve", "username": "eve"}
		}]`, now.Format(time.RFC3339))
	}))
	defer srv.Close()

	poller := NewDevToPoller(DevToConfig{Tags: []string{"fail", "ok"}, Logger: noopLogger()})
	poller.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	events, err := poller.Poll(context.Background(), now.Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("poll should not return error on partial failure: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("events = %d, want 1 (second tag should succeed)", len(events))
	}
}

func TestDevToPoller_EmptyConfig(t *testing.T) {
	poller := NewDevToPoller(DevToConfig{Logger: noopLogger()})
	events, err := poller.Poll(context.Background(), time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("events = %d, want 0", len(events))
	}
}

func TestParseDevToTime(t *testing.T) {
	tests := []struct {
		input string
		zero  bool
	}{
		{"2026-03-20T10:00:00Z", false},
		{"2026-03-20T10:00:00+05:00", false},
		{"invalid", true},
		{"", true},
	}
	for _, tt := range tests {
		got := parseDevToTime(tt.input)
		if tt.zero && !got.IsZero() {
			t.Errorf("parseDevToTime(%q) = %v, want zero", tt.input, got)
		}
		if !tt.zero && got.IsZero() {
			t.Errorf("parseDevToTime(%q) = zero, want non-zero", tt.input)
		}
	}
}
