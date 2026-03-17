package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAI_BasicTextStreaming(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %q", r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"id":"1","choices":[{"index":0,"delta":{"content":"Hello"}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"id":"1","choices":[{"index":0,"delta":{"content":" world"}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"id":"1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2}}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	p := NewOpenAIProvider("test-key", server.URL, "gpt-4o")
	ch, err := p.Stream(context.Background(), &Request{
		Messages: []Message{{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "hi"}}}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var text string
	var end *MessageEnd
	for ev := range ch {
		switch e := ev.(type) {
		case TextDelta:
			text += e.Text
		case MessageEnd:
			end = &e
		}
	}

	if text != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", text)
	}
	if end == nil {
		t.Fatal("expected MessageEnd")
	}
	if end.InputTokens != 10 || end.OutputTokens != 2 {
		t.Errorf("expected tokens 10/2, got %d/%d", end.InputTokens, end.OutputTokens)
	}
	if end.FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got %q", end.FinishReason)
	}
}

func TestOpenAI_ToolCallAssembly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// Tool call arrives in fragments.
		fmt.Fprint(w, `data: {"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"readFile","arguments":""}}]}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":"}}]}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"foo.txt\"}"}}]}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"id":"1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	p := NewOpenAIProvider("", server.URL, "test")
	ch, err := p.Stream(context.Background(), &Request{
		Messages: []Message{{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "read foo"}}}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var toolCall *ToolCallDelta
	for ev := range ch {
		if tc, ok := ev.(ToolCallDelta); ok {
			toolCall = &tc
		}
	}

	if toolCall == nil {
		t.Fatal("expected ToolCallDelta")
	}
	if toolCall.ID != "call_abc" {
		t.Errorf("expected id 'call_abc', got %q", toolCall.ID)
	}
	if toolCall.Name != "readFile" {
		t.Errorf("expected name 'readFile', got %q", toolCall.Name)
	}
	if string(toolCall.Input) != `{"path":"foo.txt"}` {
		t.Errorf("expected assembled args, got %q", string(toolCall.Input))
	}
}

func TestOpenAI_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"invalid api key"}`)
	}))
	defer server.Close()

	p := NewOpenAIProvider("bad-key", server.URL, "test")
	_, err := p.Stream(context.Background(), &Request{
		Messages: []Message{{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "hi"}}}},
	})

	if err == nil {
		t.Fatal("expected error for 401")
	}
}

func TestOpenAI_RateLimitRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":"rate limited"}`)
	}))
	defer server.Close()

	p := NewOpenAIProvider("key", server.URL, "test")
	_, err := p.Stream(context.Background(), &Request{
		Messages: []Message{{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "hi"}}}},
	})

	if err == nil {
		t.Fatal("expected error for 429")
	}
	if _, ok := err.(*retryableError); !ok {
		t.Errorf("expected retryableError, got %T", err)
	}
}

func TestOpenAI_NoAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("expected no auth header, got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"id":"1","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	// Empty API key — valid for local models (Ollama, vLLM).
	p := NewOpenAIProvider("", server.URL, "local-model")
	ch, err := p.Stream(context.Background(), &Request{
		Messages: []Message{{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "hi"}}}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var text string
	for ev := range ch {
		if td, ok := ev.(TextDelta); ok {
			text += td.Text
		}
	}
	if text != "ok" {
		t.Errorf("expected 'ok', got %q", text)
	}
}

func TestOpenAI_SystemMessage(t *testing.T) {
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = readAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"id":"1","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	p := NewOpenAIProvider("key", server.URL, "test")
	ch, err := p.Stream(context.Background(), &Request{
		System:   "You are helpful.",
		Messages: []Message{{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "hi"}}}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range ch {
	}

	// Verify system message is first.
	var req struct {
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
	}
	if err := jsonUnmarshal(capturedBody, &req); err != nil {
		t.Fatalf("failed to parse request: %v", err)
	}
	if len(req.Messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "system" {
		t.Errorf("expected first message role 'system', got %q", req.Messages[0].Role)
	}
	if req.Messages[1].Role != "user" {
		t.Errorf("expected second message role 'user', got %q", req.Messages[1].Role)
	}
}

func TestOpenAI_ProviderInterface(t *testing.T) {
	var _ Provider = (*OpenAIProvider)(nil)

	p := NewOpenAIProvider("key", "", "")
	if p.ID() != "openai" {
		t.Errorf("expected ID 'openai', got %q", p.ID())
	}
	models := p.Models()
	if len(models) == 0 {
		t.Error("expected at least one model")
	}
}

// Helpers shared with glm_test.go — avoid redeclare if already in package.
func readAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var buf []byte
	tmp := make([]byte, 1024)
	for {
		n, err := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf, nil
}

func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
