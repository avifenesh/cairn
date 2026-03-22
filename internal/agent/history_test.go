package agent

import (
	"encoding/json"
	"testing"

	"github.com/avifenesh/cairn/internal/llm"
)

func TestHistory_Empty(t *testing.T) {
	s := &Session{}
	msgs := s.History()
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestHistory_UserOnly(t *testing.T) {
	s := &Session{
		Events: []*Event{
			{Author: "user", Parts: []Part{TextPart{Text: "hello"}}},
		},
	}
	msgs := s.History()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != llm.RoleUser {
		t.Errorf("role: got %s, want user", msgs[0].Role)
	}
	if tb, ok := msgs[0].Content[0].(llm.TextBlock); !ok || tb.Text != "hello" {
		t.Errorf("content: got %v", msgs[0].Content[0])
	}
}

func TestHistory_ReconstructsToolResults(t *testing.T) {
	// Simulate a ReAct round: user → assistant(text + tool_use) → tool_result → assistant(final text).
	// Events are stored as separate DB rows per event.
	inputJSON := json.RawMessage(`{"path":"foo.go"}`)

	s := &Session{
		Events: []*Event{
			// User message.
			{Author: "user", Parts: []Part{TextPart{Text: "read foo.go"}}},
			// Assistant requests tool call.
			{Author: "agent", Parts: []Part{
				TextPart{Text: "I'll read that file."},
				ToolPart{ToolName: "readFile", CallID: "call-1", Status: ToolRunning, Input: inputJSON},
			}},
			// Tool result (separate event).
			{Author: "agent", Parts: []Part{
				ToolPart{ToolName: "readFile", CallID: "call-1", Status: ToolCompleted, Output: "package main"},
			}},
			// Final assistant response.
			{Author: "agent", Parts: []Part{TextPart{Text: "The file contains a main package."}}},
		},
	}

	msgs := s.History()

	// Expected: user → assistant(text + tool_use) → tool(result) → assistant(text)
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}

	// 1. User message.
	if msgs[0].Role != llm.RoleUser {
		t.Errorf("msg[0] role: got %s, want user", msgs[0].Role)
	}

	// 2. Assistant with text + ToolUseBlock.
	if msgs[1].Role != llm.RoleAssistant {
		t.Errorf("msg[1] role: got %s, want assistant", msgs[1].Role)
	}
	if len(msgs[1].Content) != 2 {
		t.Fatalf("msg[1] content count: got %d, want 2", len(msgs[1].Content))
	}
	if _, ok := msgs[1].Content[0].(llm.TextBlock); !ok {
		t.Error("msg[1].content[0] should be TextBlock")
	}
	if tu, ok := msgs[1].Content[1].(llm.ToolUseBlock); !ok {
		t.Error("msg[1].content[1] should be ToolUseBlock")
	} else if tu.ID != "call-1" || tu.Name != "readFile" {
		t.Errorf("ToolUseBlock: id=%s name=%s", tu.ID, tu.Name)
	}

	// 3. Tool result.
	if msgs[2].Role != llm.RoleTool {
		t.Errorf("msg[2] role: got %s, want tool", msgs[2].Role)
	}
	if tr, ok := msgs[2].Content[0].(llm.ToolResultBlock); !ok {
		t.Error("msg[2].content[0] should be ToolResultBlock")
	} else {
		if tr.ToolUseID != "call-1" {
			t.Errorf("ToolResultBlock.ToolUseID: got %s, want call-1", tr.ToolUseID)
		}
		if tr.Content != "package main" {
			t.Errorf("ToolResultBlock.Content: got %q", tr.Content)
		}
		if tr.IsError {
			t.Error("ToolResultBlock.IsError should be false")
		}
	}

	// 4. Final assistant text.
	if msgs[3].Role != llm.RoleAssistant {
		t.Errorf("msg[3] role: got %s, want assistant", msgs[3].Role)
	}
}

func TestHistory_FailedToolResult(t *testing.T) {
	s := &Session{
		Events: []*Event{
			{Author: "agent", Parts: []Part{
				ToolPart{ToolName: "shell", CallID: "c1", Status: ToolRunning, Input: json.RawMessage(`{}`)},
			}},
			{Author: "agent", Parts: []Part{
				ToolPart{ToolName: "shell", CallID: "c1", Status: ToolFailed, Error: "permission denied"},
			}},
		},
	}

	msgs := s.History()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	// Tool result should have IsError=true and content from Error field.
	tr, ok := msgs[1].Content[0].(llm.ToolResultBlock)
	if !ok {
		t.Fatal("expected ToolResultBlock")
	}
	if !tr.IsError {
		t.Error("expected IsError=true")
	}
	if tr.Content != "permission denied" {
		t.Errorf("error content: got %q, want 'permission denied'", tr.Content)
	}
}

func TestHistory_ReasoningPart(t *testing.T) {
	s := &Session{
		Events: []*Event{
			{Author: "agent", Parts: []Part{
				ReasoningPart{Text: "thinking..."},
				TextPart{Text: "here is my answer"},
			}},
		},
	}

	msgs := s.History()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != llm.RoleAssistant {
		t.Errorf("role: got %s", msgs[0].Role)
	}
	if len(msgs[0].Content) != 2 {
		t.Fatalf("content count: got %d, want 2", len(msgs[0].Content))
	}
	if _, ok := msgs[0].Content[0].(llm.ReasoningBlock); !ok {
		t.Error("expected ReasoningBlock first")
	}
}

func TestHistory_SkipsEmptyEvents(t *testing.T) {
	s := &Session{
		Events: []*Event{
			{Author: "user", Parts: []Part{}},
			{Author: "agent", Parts: nil},
			{Author: "user", Parts: []Part{TextPart{Text: "actual message"}}},
		},
	}

	msgs := s.History()
	if len(msgs) != 1 {
		t.Errorf("expected 1 message (skipping empty), got %d", len(msgs))
	}
}

func TestHistory_RoundTrip(t *testing.T) {
	// Persist events through SessionStore, load them back, verify History() works.
	d := setupTestDB(t)
	store := NewSessionStore(d)
	ctx := t.Context()

	session := &Session{ID: "test-rt", Mode: "talk", State: map[string]any{}}
	store.Create(ctx, session)

	// Simulate a round: user → agent text + tool call → tool result → agent text.
	store.AppendEvent(ctx, "test-rt", &Event{
		Author: "user",
		Parts:  []Part{TextPart{Text: "hello"}},
	})
	store.AppendEvent(ctx, "test-rt", &Event{
		Author: "agent",
		Parts: []Part{
			TextPart{Text: "let me check"},
			ToolPart{ToolName: "readFile", CallID: "c1", Status: ToolRunning, Input: json.RawMessage(`{"path":"x"}`)},
		},
	})
	store.AppendEvent(ctx, "test-rt", &Event{
		Author: "agent",
		Parts: []Part{
			ToolPart{ToolName: "readFile", CallID: "c1", Status: ToolCompleted, Output: "file content"},
		},
	})
	store.AppendEvent(ctx, "test-rt", &Event{
		Author: "agent",
		Parts:  []Part{TextPart{Text: "done"}},
	})

	// Reload session from DB.
	loaded, err := store.Get(ctx, "test-rt")
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	msgs := loaded.History()
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages from round-trip, got %d", len(msgs))
	}
	if msgs[0].Role != llm.RoleUser {
		t.Errorf("msg[0]: got %s, want user", msgs[0].Role)
	}
	if msgs[1].Role != llm.RoleAssistant {
		t.Errorf("msg[1]: got %s, want assistant", msgs[1].Role)
	}
	if msgs[2].Role != llm.RoleTool {
		t.Errorf("msg[2]: got %s, want tool", msgs[2].Role)
	}
	if msgs[3].Role != llm.RoleAssistant {
		t.Errorf("msg[3]: got %s, want assistant", msgs[3].Role)
	}
}
