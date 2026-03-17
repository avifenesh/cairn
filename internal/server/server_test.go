package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/avifenesh/cairn/internal/config"
	"github.com/avifenesh/cairn/internal/eventbus"
)

// newTestServer creates a minimal Server for testing with the given config overrides.
func newTestServer(t *testing.T, cfg *config.Config) (*Server, *httptest.Server) {
	t.Helper()
	if cfg == nil {
		cfg = &config.Config{}
	}

	bus := eventbus.New()
	t.Cleanup(bus.Close)

	srv := New(ServerConfig{
		Config: cfg,
		Bus:    bus,
	})

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	return srv, ts
}

func TestHealth(t *testing.T) {
	_, ts := newTestServer(t, nil)

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if ok, _ := body["ok"].(bool); !ok {
		t.Fatalf("expected ok:true, got %v", body)
	}

	if _, exists := body["now"]; !exists {
		t.Fatalf("expected 'now' field in response")
	}
}

func TestReady(t *testing.T) {
	_, ts := newTestServer(t, nil)

	resp, err := http.Get(ts.URL + "/ready")
	if err != nil {
		t.Fatalf("GET /ready: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if ok, _ := body["ok"].(bool); !ok {
		t.Fatalf("expected ok:true, got %v", body)
	}
}

func TestAuth_WriteRequiresToken(t *testing.T) {
	// Test 1: WRITE_API_TOKEN configured, but request has no token -> 401
	cfg := &config.Config{WriteAPIToken: "secret-write-token"}
	_, ts := newTestServer(t, cfg)

	resp, err := http.Post(ts.URL+"/v1/memories", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /v1/memories: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}

	// Test 2: WRITE_API_TOKEN not configured -> 503
	cfg2 := &config.Config{WriteAPIToken: ""}
	_, ts2 := newTestServer(t, cfg2)

	resp2, err := http.Post(ts2.URL+"/v1/memories", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /v1/memories: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp2.StatusCode)
	}
}

func TestAuth_ReadOpen(t *testing.T) {
	// READ_API_TOKEN not set -> read endpoints are open.
	cfg := &config.Config{}
	_, ts := newTestServer(t, cfg)

	resp, err := http.Get(ts.URL + "/v1/tasks")
	if err != nil {
		t.Fatalf("GET /v1/tasks: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestAuth_ReadTokenRequired(t *testing.T) {
	// READ_API_TOKEN set -> read endpoints require token.
	cfg := &config.Config{ReadAPIToken: "secret-read-token"}
	_, ts := newTestServer(t, cfg)

	// Without token -> 401.
	resp, err := http.Get(ts.URL + "/v1/tasks")
	if err != nil {
		t.Fatalf("GET /v1/tasks: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}

	// With correct token -> 200.
	req, _ := http.NewRequest("GET", ts.URL+"/v1/tasks", nil)
	req.Header.Set("X-Api-Token", "secret-read-token")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/tasks with token: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
}

func TestCORS(t *testing.T) {
	cfg := &config.Config{FrontendOrigin: "https://example.com"}
	_, ts := newTestServer(t, cfg)

	// Preflight OPTIONS.
	req, _ := http.NewRequest("OPTIONS", ts.URL+"/v1/tasks", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS /v1/tasks: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://example.com" {
		t.Fatalf("expected Access-Control-Allow-Origin: https://example.com, got %q", got)
	}

	if got := resp.Header.Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatal("expected Access-Control-Allow-Methods header")
	}
}

func TestCORS_Wildcard(t *testing.T) {
	cfg := &config.Config{} // No FrontendOrigin -> wildcard.
	_, ts := newTestServer(t, cfg)

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected Access-Control-Allow-Origin: *, got %q", got)
	}
}
