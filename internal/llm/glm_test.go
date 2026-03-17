package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// sseChunk builds an SSE data line from a GLM-format chunk JSON.
func sseChunk(data string) string {
	return "data: " + data + "\n\n"
}

// sseStream concatenates multiple SSE data lines, ending with [DONE].
func sseStream(chunks ...string) string {
	var b strings.Builder
	for _, c := range chunks {
		b.WriteString(sseChunk(c))
	}
	b.WriteString("data: [DONE]\n\n")
	return b.String()
}

// collectLLMEvents drains a channel into a slice.
func collectLLMEvents(ch <-chan Event) []Event {
	var events []Event
	for ev := range ch {
		events = append(events, ev)
	}
	return events
}

func TestGLM_BasicTextStreaming(t *testing.T) {
	stream := sseStream(
		`{"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{"content":"Hello"}}]}`,
		`{"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{"content":" world"}}]}`,
		`{"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request format.
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("expected /chat/completions path, got %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", auth)
		}

		// Verify request body.
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		if body["stream"] != true {
			t.Error("expected stream=true")
		}
		if body["model"] != "glm-5-turbo" {
			t.Errorf("expected model glm-5-turbo, got %v", body["model"])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, stream)
	}))
	defer srv.Close()

	provider := NewGLMProvider("test-key", srv.URL, "glm-5-turbo")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := provider.Stream(ctx, &Request{
		Messages: []Message{
			{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	events := collectLLMEvents(ch)

	// Expect: TextDelta("Hello"), TextDelta(" world"), MessageEnd
	var texts []string
	var msgEnd *MessageEnd
	for _, ev := range events {
		switch e := ev.(type) {
		case TextDelta:
			texts = append(texts, e.Text)
		case MessageEnd:
			msgEnd = &e
		}
	}

	if got := strings.Join(texts, ""); got != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", got)
	}
	if msgEnd == nil {
		t.Fatal("expected MessageEnd event")
	}
	if msgEnd.FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got %q", msgEnd.FinishReason)
	}
	if msgEnd.InputTokens != 10 {
		t.Errorf("expected 10 input tokens, got %d", msgEnd.InputTokens)
	}
	if msgEnd.OutputTokens != 5 {
		t.Errorf("expected 5 output tokens, got %d", msgEnd.OutputTokens)
	}
	if msgEnd.Model != "glm-5-turbo" {
		t.Errorf("expected model 'glm-5-turbo', got %q", msgEnd.Model)
	}
}

func TestGLM_ReasoningContent(t *testing.T) {
	stream := sseStream(
		`{"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{"reasoning_content":"Let me think"}}]}`,
		`{"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{"reasoning_content":"... about this"}}]}`,
		`{"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{"content":"The answer is 42"}}]}`,
		`{"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":10,"total_tokens":30}}`,
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, stream)
	}))
	defer srv.Close()

	provider := NewGLMProvider("test-key", srv.URL, "glm-5-turbo")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := provider.Stream(ctx, &Request{
		Messages: []Message{
			{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "Think about this"}}},
		},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	events := collectLLMEvents(ch)

	var reasoning []string
	var texts []string
	for _, ev := range events {
		switch e := ev.(type) {
		case ReasoningDelta:
			reasoning = append(reasoning, e.Text)
		case TextDelta:
			texts = append(texts, e.Text)
		}
	}

	if got := strings.Join(reasoning, ""); got != "Let me think... about this" {
		t.Errorf("expected reasoning 'Let me think... about this', got %q", got)
	}
	if got := strings.Join(texts, ""); got != "The answer is 42" {
		t.Errorf("expected text 'The answer is 42', got %q", got)
	}
}

func TestGLM_ToolCallAssembly(t *testing.T) {
	// Tool call arguments come as fragments that must be concatenated.
	stream := sseStream(
		`{"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"readFile","arguments":""}}]}}]}`,
		`{"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":"}}]}}]}`,
		`{"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"test.go\"}"}}]}}]}`,
		`{"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":50,"completion_tokens":20,"total_tokens":70}}`,
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, stream)
	}))
	defer srv.Close()

	provider := NewGLMProvider("test-key", srv.URL, "glm-5-turbo")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := provider.Stream(ctx, &Request{
		Messages: []Message{
			{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "Read the file"}}},
		},
		Tools: []ToolDef{
			{
				Name:        "readFile",
				Description: "Read a file",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
			},
		},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	events := collectLLMEvents(ch)

	var toolCall *ToolCallDelta
	var msgEnd *MessageEnd
	for _, ev := range events {
		switch e := ev.(type) {
		case ToolCallDelta:
			toolCall = &e
		case MessageEnd:
			msgEnd = &e
		}
	}

	if toolCall == nil {
		t.Fatal("expected ToolCallDelta event")
	}
	if toolCall.ID != "call_abc" {
		t.Errorf("expected tool call ID 'call_abc', got %q", toolCall.ID)
	}
	if toolCall.Name != "readFile" {
		t.Errorf("expected tool call name 'readFile', got %q", toolCall.Name)
	}

	expectedArgs := `{"path":"test.go"}`
	if string(toolCall.Input) != expectedArgs {
		t.Errorf("expected tool call args %q, got %q", expectedArgs, string(toolCall.Input))
	}

	if msgEnd == nil {
		t.Fatal("expected MessageEnd event")
	}
	if msgEnd.FinishReason != "tool_calls" {
		t.Errorf("expected finish_reason 'tool_calls', got %q", msgEnd.FinishReason)
	}
}

func TestGLM_NetworkErrorRetryable(t *testing.T) {
	stream := sseStream(
		`{"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{"content":"partial"}}]}`,
		`{"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{},"finish_reason":"network_error"}]}`,
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, stream)
	}))
	defer srv.Close()

	provider := NewGLMProvider("test-key", srv.URL, "glm-5-turbo")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := provider.Stream(ctx, &Request{
		Messages: []Message{
			{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	events := collectLLMEvents(ch)

	var streamErr *StreamError
	var hasText bool
	for _, ev := range events {
		switch e := ev.(type) {
		case TextDelta:
			hasText = true
			if e.Text != "partial" {
				t.Errorf("expected 'partial', got %q", e.Text)
			}
		case StreamError:
			streamErr = &e
		}
	}

	if !hasText {
		t.Error("expected TextDelta event before error")
	}
	if streamErr == nil {
		t.Fatal("expected StreamError event")
	}
	if !streamErr.Retryable {
		t.Error("expected StreamError to be retryable for network_error")
	}
}

func TestGLM_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"message":"rate limited"}}`)
	}))
	defer srv.Close()

	provider := NewGLMProvider("test-key", srv.URL, "glm-5-turbo")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := provider.Stream(ctx, &Request{
		Messages: []Message{
			{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "Hi"}}},
		},
	})
	if err == nil {
		t.Fatal("expected error for HTTP 429")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("expected error to contain '429', got: %s", err.Error())
	}
}

func TestGLM_SystemMessage(t *testing.T) {
	var receivedBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseStream(
			`{"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{"content":"ok"}}]}`,
			`{"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`,
		))
	}))
	defer srv.Close()

	provider := NewGLMProvider("test-key", srv.URL, "glm-5-turbo")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := provider.Stream(ctx, &Request{
		System: "You are a helpful assistant.",
		Messages: []Message{
			{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "Hi"}}},
		},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	// Drain events.
	collectLLMEvents(ch)

	// Verify system message is first in the messages array.
	msgs, ok := receivedBody["messages"].([]interface{})
	if !ok || len(msgs) < 2 {
		t.Fatalf("expected at least 2 messages, got %v", receivedBody["messages"])
	}
	firstMsg, ok := msgs[0].(map[string]interface{})
	if !ok {
		t.Fatal("first message is not an object")
	}
	if firstMsg["role"] != "system" {
		t.Errorf("expected first message role 'system', got %v", firstMsg["role"])
	}
}

func TestGLM_MultipleToolCalls(t *testing.T) {
	// Two tool calls in the same response.
	stream := sseStream(
		`{"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"readFile","arguments":"{\"path\":\"a.go\"}"}}]}}]}`,
		`{"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_2","type":"function","function":{"name":"readFile","arguments":"{\"path\":\"b.go\"}"}}]}}]}`,
		`{"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":30,"completion_tokens":15,"total_tokens":45}}`,
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, stream)
	}))
	defer srv.Close()

	provider := NewGLMProvider("test-key", srv.URL, "glm-5-turbo")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := provider.Stream(ctx, &Request{
		Messages: []Message{
			{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "Read files"}}},
		},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	events := collectLLMEvents(ch)

	var toolCalls []ToolCallDelta
	for _, ev := range events {
		if tc, ok := ev.(ToolCallDelta); ok {
			toolCalls = append(toolCalls, tc)
		}
	}

	if len(toolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(toolCalls))
	}
	if toolCalls[0].ID != "call_1" || toolCalls[1].ID != "call_2" {
		t.Errorf("unexpected tool call IDs: %q, %q", toolCalls[0].ID, toolCalls[1].ID)
	}
}

func TestGLM_ProviderInterface(t *testing.T) {
	p := NewGLMProvider("key", "http://localhost", "glm-5-turbo")

	if p.ID() != "glm" {
		t.Errorf("expected ID 'glm', got %q", p.ID())
	}

	models := p.Models()
	if len(models) < 1 {
		t.Fatal("expected at least 1 model")
	}
	if models[0].ID != "glm-5-turbo" {
		t.Errorf("expected first model 'glm-5-turbo', got %q", models[0].ID)
	}
}
