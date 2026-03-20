package builtin

import (
	"context"
	"encoding/json"
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
	// 35 base + 5 Z.ai tools - 2 non-Zai web tools = 38 total
	if len(tools) != 38 {
		t.Fatalf("expected 38 tools with Z.ai enabled, got %d", len(tools))
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
	// 33 base + 2 SearXNG tools = 35
	if len(tools) != 35 {
		t.Fatalf("expected 35 tools with Z.ai disabled, got %d", len(tools))
	}
}

func TestCallZaiMCP_MockServer(t *testing.T) {
	defer saveZaiState()()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer auth, got %q", r.Header.Get("Authorization"))
		}
		var req jsonRPCRequest
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "text/event-stream")
		if req.Method == "initialize" {
			w.Header().Set("mcp-session-id", "test-session-123")
			w.Write([]byte("id:1\nevent:message\ndata:{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":\"2024-11-05\"}}\n"))
		} else {
			w.Write([]byte("id:1\nevent:message\ndata:{\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"search results here\"}]}}\n"))
		}
	}))
	defer srv.Close()

	SetZaiConfig("test-key", srv.URL)

	text, err := callZaiMCP(context.Background(), "web_search_prime", "web_search_prime", map[string]any{"search_query": "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "search results here" {
		t.Fatalf("expected 'search results here', got %q", text)
	}
}

func TestExtractSSEData(t *testing.T) {
	sse := "id:1\nevent:message\ndata:{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n"
	got := extractSSEData(sse)
	if !strings.HasPrefix(got, "{\"jsonrpc\"") {
		t.Fatalf("expected JSON from SSE, got %q", got)
	}

	// Plain JSON passthrough.
	plain := `{"jsonrpc":"2.0","id":1,"result":{}}`
	got2 := extractSSEData(plain)
	if got2 != plain {
		t.Fatalf("expected passthrough for plain JSON, got %q", got2)
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
