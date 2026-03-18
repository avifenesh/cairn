package builtin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	ctx := &tool.ToolContext{Cancel: context.Background()}
	args, _ := json.Marshal(map[string]string{"query": "test"})

	result, err := webSearch.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if result.Metadata["count"].(int) != 1 {
		t.Fatalf("expected 1 result, got %v", result.Metadata["count"])
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

func TestWebFetchMock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body>Hello World</body></html>"))
	}))
	defer srv.Close()

	SetWebConfig("", 0, 0)
	ctx := &tool.ToolContext{Cancel: context.Background()}
	args, _ := json.Marshal(map[string]string{"url": srv.URL})

	result, err := webFetch.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if result.Metadata["contentType"].(string) != "text/html" {
		t.Fatalf("expected text/html, got %v", result.Metadata["contentType"])
	}
}
