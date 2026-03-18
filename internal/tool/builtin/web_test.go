package builtin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/avifenesh/cairn/internal/tool"
)

func TestWebSearchNoConfig(t *testing.T) {
	// Reset config.
	old := webConfig.SearXNGURL
	webConfig.SearXNGURL = ""
	defer func() { webConfig.SearXNGURL = old }()

	ctx := &tool.ToolContext{Cancel: context.Background()}
	args, _ := json.Marshal(map[string]string{"query": "test"})

	result, err := webSearch.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error when SearXNG not configured")
	}
}

func TestWebSearchEmptyQuery(t *testing.T) {
	SetWebConfig("http://localhost:8888", 0, 0)
	ctx := &tool.ToolContext{Cancel: context.Background()}
	args, _ := json.Marshal(map[string]string{"query": ""})

	result, err := webSearch.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error for empty query")
	}
}

func TestWebSearchMock(t *testing.T) {
	// Create a mock SearXNG server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"results":[{"title":"Result 1","url":"https://example.com","content":"A snippet"}]}`))
	}))
	defer srv.Close()

	SetWebConfig(srv.URL, 0, 0)

	// Test doSearXNGSearch directly (bypasses SSRF check since SearXNG is trusted).
	results, err := doSearXNGSearch(context.Background(), "test", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Title != "Result 1" {
		t.Fatalf("expected 'Result 1', got %q", results[0].Title)
	}
}

func TestWebSearchPathPreservation(t *testing.T) {
	// Verify SearXNG URL path prefix is preserved.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/prefix/search") {
			t.Errorf("expected /prefix/search path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()

	SetWebConfig(srv.URL+"/prefix", 0, 0)
	_, err := doSearXNGSearch(context.Background(), "test", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWebFetchEmptyURL(t *testing.T) {
	ctx := &tool.ToolContext{Cancel: context.Background()}
	args, _ := json.Marshal(map[string]string{"url": ""})

	result, err := webFetch.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error for empty URL")
	}
}

func TestWebFetchInvalidScheme(t *testing.T) {
	ctx := &tool.ToolContext{Cancel: context.Background()}
	args, _ := json.Marshal(map[string]string{"url": "ftp://example.com"})

	result, err := webFetch.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error for non-HTTP scheme")
	}
}

func TestWebFetchSSRFBlocked(t *testing.T) {
	ctx := &tool.ToolContext{Cancel: context.Background()}

	blocked := []string{
		"http://127.0.0.1/secret",
		"http://localhost/secret",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.1/internal",
		"http://192.168.1.1/admin",
	}
	for _, u := range blocked {
		args, _ := json.Marshal(map[string]string{"url": u})
		result, err := webFetch.Execute(ctx, args)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", u, err)
		}
		if result.Error == "" {
			t.Fatalf("expected SSRF block for %s", u)
		}
	}
}

func TestWebFetchDoFetchMock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body>Hello World</body></html>"))
	}))
	defer srv.Close()

	SetWebConfig("", 0, 0)

	// Test doFetch directly (bypasses SSRF check for localhost test server).
	content, contentType, err := doFetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contentType != "text/html" {
		t.Fatalf("expected text/html, got %s", contentType)
	}
	if !strings.Contains(content, "Hello World") {
		t.Fatalf("expected content to contain 'Hello World', got: %s", content)
	}
}

func TestSafeCtxNil(t *testing.T) {
	ctx := safeCtx(nil)
	if ctx == nil {
		t.Fatal("safeCtx(nil) should return non-nil context")
	}
}

func TestValidateHostBlocked(t *testing.T) {
	blocked := []string{"127.0.0.1", "169.254.169.254", "10.0.0.1", "192.168.1.1", "metadata.google.internal"}
	for _, host := range blocked {
		if err := validateHost(host); err == nil {
			t.Errorf("expected %s to be blocked", host)
		}
	}
}
