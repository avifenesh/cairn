package signal

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/avifenesh/cairn/internal/db"
)

// --- Reddit poller tests ---

func TestRedditPoller_Poll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"data": {
				"children": [
					{"data": {"id": "abc1", "title": "Svelte 5 released", "selftext": "Big news", "url": "https://svelte.dev", "permalink": "/r/programming/comments/abc1/", "author": "rich_harris", "subreddit": "programming", "score": 500, "created_utc": %d}},
					{"data": {"id": "abc2", "title": "Old post", "selftext": "", "url": "https://old.com", "permalink": "/r/programming/comments/abc2/", "author": "someone", "subreddit": "programming", "score": 10, "created_utc": %d}}
				]
			}
		}`, time.Now().Unix(), time.Now().Add(-48*time.Hour).Unix())
	}))
	defer srv.Close()

	poller := NewRedditPoller(RedditConfig{Subreddits: []string{"programming"}})
	poller.client = srv.Client()

	// Override URL by wrapping the client transport.
	origTransport := poller.client.Transport
	poller.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = srv.Listener.Addr().String()
		if origTransport != nil {
			return origTransport.RoundTrip(req)
		}
		return http.DefaultTransport.RoundTrip(req)
	})

	events, err := poller.Poll(context.Background(), time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}

	// Only the recent post should pass the since filter.
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Title != "Svelte 5 released" {
		t.Errorf("title = %q, want %q", events[0].Title, "Svelte 5 released")
	}
	if events[0].Source != SourceReddit {
		t.Errorf("source = %q, want %q", events[0].Source, SourceReddit)
	}
	if events[0].Kind != KindPost {
		t.Errorf("kind = %q, want %q", events[0].Kind, KindPost)
	}
}

func TestRedditPoller_Source(t *testing.T) {
	p := NewRedditPoller(RedditConfig{Subreddits: []string{"golang"}})
	if p.Source() != SourceReddit {
		t.Errorf("source = %q, want %q", p.Source(), SourceReddit)
	}
}

// --- npm poller tests ---

func TestNPMPoller_Poll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"name": "svelte",
			"dist-tags": {"latest": "5.0.0"},
			"time": {"5.0.0": "%s", "4.0.0": "2024-01-01T00:00:00.000Z"}
		}`, time.Now().UTC().Format(time.RFC3339))
	}))
	defer srv.Close()

	poller := NewNPMPoller(NPMConfig{Packages: []string{"svelte"}})
	poller.client = srv.Client()
	poller.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = srv.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})

	events, err := poller.Poll(context.Background(), time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Kind != KindPackage {
		t.Errorf("kind = %q, want %q", events[0].Kind, KindPackage)
	}
	if !strings.Contains(events[0].Title, "svelte 5.0.0") {
		t.Errorf("title = %q, want to contain 'svelte 5.0.0'", events[0].Title)
	}
}

func TestNPMPoller_OldVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"name": "old-pkg",
			"dist-tags": {"latest": "1.0.0"},
			"time": {"1.0.0": "2020-01-01T00:00:00.000Z"}
		}`)
	}))
	defer srv.Close()

	poller := NewNPMPoller(NPMConfig{Packages: []string{"old-pkg"}})
	poller.client = srv.Client()
	poller.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = srv.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})

	events, err := poller.Poll(context.Background(), time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("events = %d, want 0 (old version)", len(events))
	}
}

// --- crates.io poller tests ---

func TestCratesPoller_Poll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"crate": {
				"name": "tokio",
				"max_version": "2.0.0",
				"updated_at": "%s"
			}
		}`, time.Now().UTC().Format(time.RFC3339))
	}))
	defer srv.Close()

	poller := NewCratesPoller(CratesConfig{Crates: []string{"tokio"}})
	poller.client = srv.Client()
	poller.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req.URL.Scheme = "http"
		req.URL.Host = srv.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})

	events, err := poller.Poll(context.Background(), time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if !strings.Contains(events[0].Title, "tokio 2.0.0") {
		t.Errorf("title = %q, want to contain 'tokio 2.0.0'", events[0].Title)
	}
}

// --- Webhook handler tests ---

func TestWebhookHandler_ValidSignature(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	defer database.Close()

	store := NewEventStore(database.DB)
	secret := "test-secret"
	handler := NewWebhookHandler(store, map[string]string{"github": secret})

	body := `{"title":"PR opened","url":"https://github.com/a/b/pull/1"}`
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/github", strings.NewReader(body))
	req.SetPathValue("name", "github")
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. Body: %s", rr.Code, rr.Body.String())
	}

	// Verify event was ingested.
	count, _ := store.Count(context.Background(), EventFilter{Source: SourceWebhook})
	if count != 1 {
		t.Errorf("webhook events = %d, want 1", count)
	}
}

func TestWebhookHandler_InvalidSignature(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	defer database.Close()

	store := NewEventStore(database.DB)
	handler := NewWebhookHandler(store, map[string]string{"github": "real-secret"})

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/github", strings.NewReader(`{"title":"bad"}`))
	req.SetPathValue("name", "github")
	req.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestWebhookHandler_UnknownWebhook(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	defer database.Close()

	store := NewEventStore(database.DB)
	handler := NewWebhookHandler(store, map[string]string{"github": "s"})

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/unknown", strings.NewReader(`{}`))
	req.SetPathValue("name", "unknown")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestWebhookHandler_TokenAuth(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	defer database.Close()

	store := NewEventStore(database.DB)
	handler := NewWebhookHandler(store, map[string]string{"simple": "my-token"})

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/simple?token=my-token", strings.NewReader(`{"title":"Token event"}`))
	req.SetPathValue("name", "simple")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

// --- Digest runner tests ---

func TestDigestRunner_EmptyEvents(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	defer database.Close()

	store := NewEventStore(database.DB)
	runner := NewDigestRunner(store, nil, "test-model")

	digest, err := runner.Generate(context.Background())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if digest.Summary != "No new events." {
		t.Errorf("summary = %q, want 'No new events.'", digest.Summary)
	}
}

func TestDigestRunner_WithEventsNoLLM(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	defer database.Close()

	store := NewEventStore(database.DB)
	store.Ingest(context.Background(), []*RawEvent{
		{Source: "github", SourceID: "d:1", Kind: "pr", Title: "Fix bug", OccurredAt: time.Now().UTC()},
		{Source: "github", SourceID: "d:2", Kind: "pr", Title: "Add feature", OccurredAt: time.Now().UTC()},
		{Source: "hn", SourceID: "d:3", Kind: "story", Title: "Go 2.0", OccurredAt: time.Now().UTC()},
	})

	// No LLM provider - should return basic summary.
	runner := NewDigestRunner(store, nil, "")
	digest, err := runner.Generate(context.Background())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if digest.EventCount != 3 {
		t.Errorf("eventCount = %d, want 3", digest.EventCount)
	}
	if len(digest.Groups) != 2 {
		t.Errorf("groups = %d, want 2", len(digest.Groups))
	}
	if !strings.Contains(digest.Summary, "3 unread events") {
		t.Errorf("summary = %q, want to contain '3 unread events'", digest.Summary)
	}
}

func TestStringFromMap(t *testing.T) {
	m := map[string]any{
		"title": "Hello",
		"sender": map[string]any{
			"login": "alice",
		},
	}

	if got := stringFromMap(m, "title"); got != "Hello" {
		t.Errorf("title = %q, want Hello", got)
	}
	if got := stringFromMap(m, "sender.login"); got != "alice" {
		t.Errorf("sender.login = %q, want alice", got)
	}
	if got := stringFromMap(m, "missing", "title"); got != "Hello" {
		t.Errorf("fallback = %q, want Hello", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 100); got != "short" {
		t.Errorf("truncate short = %q", got)
	}
	long := strings.Repeat("a", 600)
	if got := truncate(long, 500); len(got) != 503 { // 500 + "..."
		t.Errorf("truncate long len = %d, want 503", len(got))
	}
}

// --- Helpers ---

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
