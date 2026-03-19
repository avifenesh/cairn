package signal

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBotFilter(t *testing.T) {
	// Default bots.
	for _, login := range []string{"dependabot[bot]", "renovate[bot]", "github-actions[bot]", "copilot"} {
		if !IsBot(login, nil) {
			t.Errorf("expected %q to be bot", login)
		}
	}
	// Pattern matching.
	if !IsBot("some-random[bot]", nil) {
		t.Error("expected [bot] suffix to match")
	}
	if !IsBot("my-custom-bot", nil) {
		t.Error("expected -bot suffix to match")
	}
	// Not bots.
	if IsBot("avifenesh", nil) {
		t.Error("expected avifenesh to NOT be bot")
	}
	if IsBot("contributor", nil) {
		t.Error("expected contributor to NOT be bot")
	}
	// Custom extra.
	extra := map[string]bool{"mybot": true}
	if !IsBot("mybot", extra) {
		t.Error("expected custom bot to match")
	}
	// Case insensitive.
	if !IsBot("Dependabot[bot]", nil) {
		t.Error("expected case-insensitive match")
	}
}

func TestGitHubSignalEngagement(t *testing.T) {
	mux := http.NewServeMux()

	// Issues endpoint: 1 external, 1 self, 1 bot.
	mux.HandleFunc("/repos/test/repo/issues", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"number": 1, "title": "Bug report", "html_url": "https://github.com/test/repo/issues/1", "user": map[string]string{"login": "external-user"}, "created_at": "2026-03-19T10:00:00Z"},
			{"number": 2, "title": "My issue", "html_url": "https://github.com/test/repo/issues/2", "user": map[string]string{"login": "testowner"}, "created_at": "2026-03-19T10:00:00Z"},
			{"number": 3, "title": "Bot issue", "html_url": "https://github.com/test/repo/issues/3", "user": map[string]string{"login": "dependabot[bot]"}, "created_at": "2026-03-19T10:00:00Z"},
		})
	})

	// PRs endpoint: 1 external.
	mux.HandleFunc("/repos/test/repo/pulls", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"number": 10, "title": "External PR", "html_url": "https://github.com/test/repo/pulls/10", "user": map[string]string{"login": "contributor"}, "created_at": "2026-03-19T10:00:00Z"},
		})
	})

	// Comments endpoint: 1 external, 1 bot.
	mux.HandleFunc("/repos/test/repo/issues/comments", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"id": 100, "body": "Nice work!", "html_url": "https://github.com/test/repo/issues/1#comment-100", "user": map[string]string{"login": "reviewer"}, "created_at": "2026-03-19T10:00:00Z"},
			{"id": 101, "body": "Auto-review", "html_url": "https://github.com/test/repo/issues/1#comment-101", "user": map[string]string{"login": "claude-review[bot]"}, "created_at": "2026-03-19T10:00:00Z"},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	db := setupTestDB(t)
	state := NewSourceState(db)

	poller := &GitHubSignalPoller{
		token:           "test-token",
		owner:           "testowner",
		repos:           []string{"test/repo"},
		bots:            map[string]bool{},
		state:           state,
		client:          srv.Client(),
		logger:          noopLogger(),
		metricsInterval: 24 * time.Hour,
		lastMetrics:     time.Now(), // skip metrics
	}
	// Override API base URL by replacing ghGet.
	origGet := poller.client
	_ = origGet
	// We need to patch the base URL. Instead, override the HTTP transport.
	poller.client = &http.Client{
		Transport: &rewriteTransport{base: srv.URL, wrapped: http.DefaultTransport},
		Timeout:   5 * time.Second,
	}

	events, err := poller.Poll(context.Background(), time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have: 1 issue (external), 1 PR (external), 1 comment (external) = 3 events.
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	kinds := map[string]int{}
	for _, ev := range events {
		kinds[ev.Kind]++
	}
	if kinds[KindIssue] != 1 {
		t.Errorf("expected 1 issue, got %d", kinds[KindIssue])
	}
	if kinds[KindPR] != 1 {
		t.Errorf("expected 1 PR, got %d", kinds[KindPR])
	}
	if kinds[KindComment] != 1 {
		t.Errorf("expected 1 comment, got %d", kinds[KindComment])
	}
}

func TestGitHubSignalMetrics(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/test/repo", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"stargazers_count":  15,
			"forks_count":       3,
			"subscribers_count": 5,
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	db := setupTestDB(t)
	state := NewSourceState(db)

	// Seed previous snapshot.
	state.SetExtra(context.Background(), SourceGitHubSignal, map[string]any{
		"metrics:test/repo": map[string]any{"stars": float64(12), "forks": float64(2), "watchers": float64(5)},
	})

	poller := &GitHubSignalPoller{
		token:           "test-token",
		owner:           "testowner",
		repos:           []string{"test/repo"},
		bots:            map[string]bool{},
		state:           state,
		logger:          noopLogger(),
		metricsInterval: 0,
		client: &http.Client{
			Transport: &rewriteTransport{base: srv.URL, wrapped: http.DefaultTransport},
			Timeout:   5 * time.Second,
		},
	}

	events, err := poller.pollMetrics(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Delta: +3 stars, +1 fork = 1 metrics event.
	if len(events) != 1 {
		t.Fatalf("expected 1 metrics event, got %d", len(events))
	}
	if events[0].Kind != KindMetrics {
		t.Errorf("expected kind=%s, got %s", KindMetrics, events[0].Kind)
	}
	if events[0].Metadata["dStars"] != 3 {
		t.Errorf("expected dStars=3, got %v", events[0].Metadata["dStars"])
	}
}

func TestGitHubSignalFollowers(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/users/testowner/followers", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]string{
			{"login": "alice"},
			{"login": "bob"},
			{"login": "charlie"},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	db := setupTestDB(t)
	state := NewSourceState(db)

	// Seed previous followers: alice, bob.
	state.SetExtra(context.Background(), SourceGitHubSignal, map[string]any{
		"followers": []any{"alice", "bob"},
	})

	poller := &GitHubSignalPoller{
		token:  "test-token",
		owner:  "testowner",
		state:  state,
		logger: noopLogger(),
		client: &http.Client{
			Transport: &rewriteTransport{base: srv.URL, wrapped: http.DefaultTransport},
			Timeout:   5 * time.Second,
		},
	}

	events, err := poller.pollFollowers(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// New: charlie.
	if len(events) != 1 {
		t.Fatalf("expected 1 new follower event, got %d", len(events))
	}
	if events[0].Actor != "charlie" {
		t.Errorf("expected charlie, got %s", events[0].Actor)
	}
}

func TestGitHubSignalNewRepos(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/user/repos", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]any{
			{"full_name": "test/repo1", "fork": false},
			{"full_name": "test/repo2", "fork": false},
			{"full_name": "test/new-repo", "fork": false},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	db := setupTestDB(t)
	state := NewSourceState(db)

	// Seed previous repos.
	state.SetExtra(context.Background(), SourceGitHubSignal, map[string]any{
		"repos": []any{"test/repo1", "test/repo2"},
	})

	poller := &GitHubSignalPoller{
		token:  "test-token",
		owner:  "testowner",
		state:  state,
		logger: noopLogger(),
		client: &http.Client{
			Transport: &rewriteTransport{base: srv.URL, wrapped: http.DefaultTransport},
			Timeout:   5 * time.Second,
		},
	}

	events, err := poller.pollNewRepos(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 new repo event, got %d", len(events))
	}
	if events[0].Kind != KindNewRepo {
		t.Errorf("expected kind=%s, got %s", KindNewRepo, events[0].Kind)
	}
}

// rewriteTransport redirects GitHub API calls to the test server.
type rewriteTransport struct {
	base    string
	wrapped http.RoundTripper
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = t.base[len("http://"):]
	req.Header.Del("Authorization")
	return t.wrapped.RoundTrip(req)
}

func noopLogger() *slog.Logger {
	return slog.Default()
}
