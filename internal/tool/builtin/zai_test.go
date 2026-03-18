package builtin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func saveZaiState() func() {
	key := zaiConfig.APIKey
	base := zaiConfig.BaseURL
	enabled := zaiConfig.enabled.Load()
	client := zaiConfig.HTTPClient
	return func() {
		zaiConfig.APIKey = key
		zaiConfig.BaseURL = base
		zaiConfig.enabled.Store(enabled)
		zaiConfig.HTTPClient = client
	}
}

func TestZaiEnabled(t *testing.T) {
	defer saveZaiState()()

	SetZaiConfig("", "")
	if ZaiEnabled() {
		t.Fatal("expected Z.ai disabled with no API key")
	}

	SetZaiConfig("test-key", "")
	if !ZaiEnabled() {
		t.Fatal("expected Z.ai enabled with API key")
	}
}

func TestZaiToolCount(t *testing.T) {
	defer saveZaiState()()

	SetZaiConfig("test-key", "http://localhost")
	tools := All()
	// 22 base + 5 Z.ai tools = 27
	if len(tools) != 27 {
		t.Fatalf("expected 27 tools with Z.ai enabled, got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, tl := range tools {
		names[tl.Name()] = true
	}

	zaiTools := []string{
		"cairn.webSearch", "cairn.webFetch",
		"cairn.searchDoc", "cairn.repoStructure", "cairn.readRepoFile",
	}
	for _, name := range zaiTools {
		if !names[name] {
			t.Fatalf("expected Z.ai tool %q to be registered", name)
		}
	}
}

func TestZaiDefaultToolCount(t *testing.T) {
	defer saveZaiState()()

	SetZaiConfig("", "")
	tools := All()
	// 22 base + 2 SearXNG tools = 24
	if len(tools) != 24 {
		t.Fatalf("expected 24 tools with Z.ai disabled, got %d", len(tools))
	}
}

func TestCallZaiMCP_MockServer(t *testing.T) {
	defer saveZaiState()()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer auth, got %q", r.Header.Get("Authorization"))
		}
		// Verify service path is included.
		if !strings.Contains(r.URL.Path, "/web_search_prime/mcp") {
			t.Errorf("expected /web_search_prime/mcp in path, got %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"search results here"}]}}`))
	}))
	defer srv.Close()

	SetZaiConfig("test-key", srv.URL)

	text, err := callZaiMCP(context.Background(), "web_search_prime", "webSearchPrime", map[string]any{"query": "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "search results here" {
		t.Fatalf("expected 'search results here', got %q", text)
	}
}

func TestExtractMCPText(t *testing.T) {
	raw := []byte(`{"content":[{"type":"text","text":"hello"},{"type":"text","text":"world"}]}`)
	got := extractMCPText(raw)
	if got != "hello\nworld" {
		t.Fatalf("expected 'hello\\nworld', got %q", got)
	}

	// Empty content falls back to raw.
	raw2 := []byte(`{"content":[]}`)
	got2 := extractMCPText(raw2)
	if got2 != `{"content":[]}` {
		t.Fatalf("expected raw fallback, got %q", got2)
	}
}
