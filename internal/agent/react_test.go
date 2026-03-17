package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/llm"
	"github.com/avifenesh/cairn/internal/tool"
)

// mockLLMServer creates a test server that returns a simple text response.
func mockLLMServer(t *testing.T, responses ...string) *httptest.Server {
	t.Helper()
	callCount := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		idx := callCount
		if idx >= len(responses) {
			idx = len(responses) - 1
		}
		callCount++

		fmt.Fprint(w, responses[idx])
	}))
}

func textSSE(text string) string {
	return fmt.Sprintf(
		"data: {\"id\":\"1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":%q}}]}\n\n"+
			"data: {\"id\":\"1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5}}\n\n"+
			"data: [DONE]\n\n",
		text)
}

func toolCallSSE(toolName, toolID, args string) string {
	return fmt.Sprintf(
		"data: {\"id\":\"1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":%q,\"type\":\"function\",\"function\":{\"name\":%q,\"arguments\":%q}}]}}]}\n\n"+
			"data: {\"id\":\"1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n"+
			"data: [DONE]\n\n",
		toolID, toolName, args)
}

func TestReActAgent_SimpleText(t *testing.T) {
	server := mockLLMServer(t, textSSE("Hello from the agent!"))
	defer server.Close()

	provider := llm.NewOpenAIProvider("", server.URL, "test")
	bus := eventbus.New()
	defer bus.Close()

	ag := NewReActAgent("test-agent", nil)

	session := &Session{ID: "s1", State: map[string]any{}}
	invCtx := &InvocationContext{
		Context:     context.Background(),
		SessionID:   "s1",
		UserMessage: "hello",
		Mode:        tool.ModeTalk,
		Session:     session,
		Tools:       tool.NewRegistry(),
		LLM:         provider,
		Bus:         bus,
		Config:      &AgentConfig{Model: "test"},
	}

	var texts []string
	for ev := range ag.Run(invCtx) {
		if ev.Err != nil {
			t.Fatalf("unexpected error: %v", ev.Err)
		}
		for _, p := range ev.Event.Parts {
			if tp, ok := p.(TextPart); ok && ev.Event.Author != "user" {
				texts = append(texts, tp.Text)
			}
		}
	}

	joined := ""
	for _, s := range texts {
		joined += s
	}
	if joined != "Hello from the agent!" {
		t.Errorf("expected 'Hello from the agent!', got %q", joined)
	}
}

func TestReActAgent_ToolExecution(t *testing.T) {
	// First call: LLM requests a tool. Second call: LLM responds with text.
	server := mockLLMServer(t,
		toolCallSSE("echo", "call_1", `{"text":"world"}`),
		textSSE("The echo tool said: world"),
	)
	defer server.Close()

	provider := llm.NewOpenAIProvider("", server.URL, "test")
	bus := eventbus.New()
	defer bus.Close()

	// Register a simple echo tool.
	registry := tool.NewRegistry()
	echoTool := tool.Define("echo", "Echo input text",
		[]tool.Mode{tool.ModeTalk, tool.ModeWork, tool.ModeCoding},
		func(ctx *tool.ToolContext, p struct {
			Text string `json:"text"`
		}) (*tool.ToolResult, error) {
			return &tool.ToolResult{Output: p.Text}, nil
		},
	)
	registry.Register(echoTool)

	ag := NewReActAgent("test-agent", nil)

	session := &Session{ID: "s1", State: map[string]any{"workDir": "."}}
	invCtx := &InvocationContext{
		Context:     context.Background(),
		SessionID:   "s1",
		UserMessage: "echo world",
		Mode:        tool.ModeTalk,
		Session:     session,
		Tools:       registry,
		LLM:         provider,
		Bus:         bus,
		Config:      &AgentConfig{Model: "test"},
	}

	var texts []string
	var toolParts []ToolPart
	for ev := range ag.Run(invCtx) {
		if ev.Err != nil {
			t.Fatalf("unexpected error: %v", ev.Err)
		}
		if ev.Event == nil {
			continue
		}
		for _, p := range ev.Event.Parts {
			switch v := p.(type) {
			case TextPart:
				if ev.Event.Author != "user" {
					texts = append(texts, v.Text)
				}
			case ToolPart:
				toolParts = append(toolParts, v)
			}
		}
	}

	// Should have tool execution events.
	if len(toolParts) == 0 {
		t.Fatal("expected tool parts")
	}

	// Check tool was called and completed.
	foundCompleted := false
	for _, tp := range toolParts {
		if tp.ToolName == "echo" && tp.Status == ToolCompleted {
			foundCompleted = true
			if tp.Output != "world" {
				t.Errorf("expected tool output 'world', got %q", tp.Output)
			}
		}
	}
	if !foundCompleted {
		t.Error("expected a completed echo tool call")
	}

	// Should have final text response.
	joined := ""
	for _, s := range texts {
		joined += s
	}
	if joined != "The echo tool said: world" {
		t.Errorf("expected final text, got %q", joined)
	}
}

func TestReActAgent_ToolError(t *testing.T) {
	server := mockLLMServer(t,
		toolCallSSE("fail", "call_1", `{}`),
		textSSE("The tool failed, sorry."),
	)
	defer server.Close()

	provider := llm.NewOpenAIProvider("", server.URL, "test")
	bus := eventbus.New()
	defer bus.Close()

	registry := tool.NewRegistry()
	failTool := tool.Define("fail", "Always fails",
		[]tool.Mode{tool.ModeTalk},
		func(ctx *tool.ToolContext, p struct{}) (*tool.ToolResult, error) {
			return nil, fmt.Errorf("intentional failure")
		},
	)
	registry.Register(failTool)

	ag := NewReActAgent("test-agent", nil)
	session := &Session{ID: "s1", State: map[string]any{}}
	invCtx := &InvocationContext{
		Context:     context.Background(),
		SessionID:   "s1",
		UserMessage: "fail please",
		Mode:        tool.ModeTalk,
		Session:     session,
		Tools:       registry,
		LLM:         provider,
		Bus:         bus,
		Config:      &AgentConfig{Model: "test"},
	}

	var failedTools []ToolPart
	for ev := range ag.Run(invCtx) {
		if ev.Err != nil {
			t.Fatalf("unexpected error: %v", ev.Err)
		}
		if ev.Event == nil {
			continue
		}
		for _, p := range ev.Event.Parts {
			if tp, ok := p.(ToolPart); ok && tp.Status == ToolFailed {
				failedTools = append(failedTools, tp)
			}
		}
	}

	if len(failedTools) != 1 {
		t.Fatalf("expected 1 failed tool, got %d", len(failedTools))
	}
	if failedTools[0].Error != "intentional failure" {
		t.Errorf("expected error 'intentional failure', got %q", failedTools[0].Error)
	}
}

func TestReActAgent_MaxRounds(t *testing.T) {
	// LLM always requests a tool — should stop at max rounds.
	server := mockLLMServer(t,
		toolCallSSE("noop", "c1", `{}`),
		toolCallSSE("noop", "c2", `{}`),
		toolCallSSE("noop", "c3", `{}`),
		toolCallSSE("noop", "c4", `{}`),
	)
	defer server.Close()

	provider := llm.NewOpenAIProvider("", server.URL, "test")
	bus := eventbus.New()
	defer bus.Close()

	registry := tool.NewRegistry()
	registry.Register(tool.Define("noop", "No-op",
		[]tool.Mode{tool.ModeTalk},
		func(ctx *tool.ToolContext, p struct{}) (*tool.ToolResult, error) {
			return &tool.ToolResult{Output: "ok"}, nil
		},
	))

	ag := NewReActAgent("test-agent", nil)
	session := &Session{ID: "s1", State: map[string]any{}}
	invCtx := &InvocationContext{
		Context:     context.Background(),
		SessionID:   "s1",
		UserMessage: "loop forever",
		Mode:        tool.ModeTalk,
		Session:     session,
		Tools:       registry,
		LLM:         provider,
		Bus:         bus,
		Config:      &AgentConfig{Model: "test", MaxRounds: 3},
	}

	var lastText string
	for ev := range ag.Run(invCtx) {
		if ev.Event != nil {
			for _, p := range ev.Event.Parts {
				if tp, ok := p.(TextPart); ok {
					lastText = tp.Text
				}
			}
		}
	}

	if lastText != "[max tool rounds reached]" {
		t.Errorf("expected max rounds message, got %q", lastText)
	}
}

func TestReActAgent_ModeFiltering(t *testing.T) {
	// Verify that tools are filtered by mode.
	registry := tool.NewRegistry()
	// Register a coding-only tool.
	registry.Register(tool.Define("coding-only", "Only in coding mode",
		[]tool.Mode{tool.ModeCoding},
		func(ctx *tool.ToolContext, p struct{}) (*tool.ToolResult, error) {
			return &tool.ToolResult{Output: "ok"}, nil
		},
	))

	// In talk mode, the tool should not appear.
	talkTools := registry.ForLLM(tool.ModeTalk)
	codingTools := registry.ForLLM(tool.ModeCoding)

	if len(talkTools) != 0 {
		t.Errorf("expected 0 tools in talk mode, got %d", len(talkTools))
	}
	if len(codingTools) != 1 {
		t.Errorf("expected 1 tool in coding mode, got %d", len(codingTools))
	}
}

// Ensure json import is used.
var _ = json.Marshal
