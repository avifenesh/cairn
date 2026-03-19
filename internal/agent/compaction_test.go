package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/avifenesh/cairn/internal/llm"
)

func TestEstimateMessageTokens(t *testing.T) {
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock{Text: "hello world"}}},            // 11 chars
		{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.TextBlock{Text: "hi there friend"}}},   // 15 chars
		{Role: llm.RoleTool, Content: []llm.ContentBlock{llm.ToolResultBlock{Content: "result data x"}}}, // 13 chars
	}
	// Total = 39 chars → (39+3)/4 = 10 tokens
	tokens := EstimateMessageTokens(messages)
	if tokens != 10 {
		t.Fatalf("expected 10 tokens, got %d", tokens)
	}
}

func TestEstimateMessageTokens_Empty(t *testing.T) {
	tokens := EstimateMessageTokens(nil)
	if tokens != 0 {
		t.Fatalf("expected 0 tokens for nil, got %d", tokens)
	}
}

func TestTruncateToolOutput_Short(t *testing.T) {
	output := "short output"
	result := TruncateToolOutput(output, 100)
	if result != output {
		t.Fatalf("expected unchanged output, got %q", result)
	}
}

func TestTruncateToolOutput_Long(t *testing.T) {
	output := "AAAA" + "BBBBBBBBBB" + "CCCC" // 18 chars
	result := TruncateToolOutput(output, 10)

	// Head: 6 chars, tail: 4 chars, middle dropped
	if len(result) <= 10 {
		t.Fatalf("expected result longer than max (includes marker), got %d", len(result))
	}
	if result[:6] != "AAAABB" {
		t.Fatalf("expected head 'AAAABB', got %q", result[:6])
	}
	if result[len(result)-4:] != "CCCC" {
		t.Fatalf("expected tail 'CCCC', got %q", result[len(result)-4:])
	}
	if !strings.Contains(result, "truncated") {
		t.Fatalf("expected truncation marker, got %q", result)
	}
}

func TestTruncateToolOutput_ZeroMax(t *testing.T) {
	result := TruncateToolOutput("anything", 0)
	if result != "anything" {
		t.Fatalf("expected unchanged for max=0, got %q", result)
	}
}

func TestStripOrphanedToolResults(t *testing.T) {
	messages := []llm.Message{
		// Assistant with tool use
		{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
			llm.ToolUseBlock{ID: "call_1", Name: "test"},
		}},
		// Valid tool result
		{Role: llm.RoleTool, Content: []llm.ContentBlock{
			llm.ToolResultBlock{ToolUseID: "call_1", Content: "ok"},
		}},
		// Orphaned tool result (call_0 not in messages)
		{Role: llm.RoleTool, Content: []llm.ContentBlock{
			llm.ToolResultBlock{ToolUseID: "call_0", Content: "orphaned"},
		}},
		// User message (should be preserved)
		{Role: llm.RoleUser, Content: []llm.ContentBlock{
			llm.TextBlock{Text: "hello"},
		}},
	}

	cleaned := stripOrphanedToolResults(messages)
	if len(cleaned) != 3 {
		t.Fatalf("expected 3 messages (orphan removed), got %d", len(cleaned))
	}

	for _, msg := range cleaned {
		for _, block := range msg.Content {
			if tr, ok := block.(llm.ToolResultBlock); ok {
				if tr.ToolUseID == "call_0" {
					t.Fatal("orphaned tool result should have been removed")
				}
			}
		}
	}
}

func TestStripOrphanedToolResults_NoOrphans(t *testing.T) {
	messages := []llm.Message{
		{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
			llm.ToolUseBlock{ID: "call_1", Name: "test"},
		}},
		{Role: llm.RoleTool, Content: []llm.ContentBlock{
			llm.ToolResultBlock{ToolUseID: "call_1", Content: "ok"},
		}},
	}
	cleaned := stripOrphanedToolResults(messages)
	if len(cleaned) != 2 {
		t.Fatalf("expected 2 messages unchanged, got %d", len(cleaned))
	}
}

func TestCompactMessages_UnderThreshold(t *testing.T) {
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock{Text: "short"}}},
		{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.TextBlock{Text: "reply"}}},
	}

	cfg := CompactionConfig{TriggerTokens: 100000, KeepRecentPairs: 10}
	result, err := CompactMessages(context.Background(), messages, nil, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != len(messages) {
		t.Fatalf("expected unchanged (under threshold), got %d messages", len(result))
	}
}

func TestCompactMessages_DisabledWithZeroThreshold(t *testing.T) {
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock{Text: "test"}}},
	}

	cfg := CompactionConfig{TriggerTokens: 0}
	result, err := CompactMessages(context.Background(), messages, nil, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected unchanged when disabled, got %d", len(result))
	}
}

// mockProvider is a test LLM provider that returns a fixed response.
type mockProvider struct {
	response string
}

func (m *mockProvider) ID() string { return "mock" }
func (m *mockProvider) Models() []llm.ModelInfo {
	return []llm.ModelInfo{{ID: "mock"}}
}
func (m *mockProvider) Stream(_ context.Context, _ *llm.Request) (<-chan llm.Event, error) {
	ch := make(chan llm.Event, 2)
	ch <- llm.TextDelta{Text: m.response}
	ch <- llm.MessageEnd{FinishReason: "stop"}
	close(ch)
	return ch, nil
}

func TestCompactMessages_OverThreshold(t *testing.T) {
	// Build a conversation that exceeds a low threshold.
	longText := strings.Repeat("x", 1000) // 1000 chars = ~250 tokens
	messages := []llm.Message{
		// Old messages (will be summarized)
		{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock{Text: "old question 1"}}},
		{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.TextBlock{Text: longText}}},
		{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock{Text: "old question 2"}}},
		{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.TextBlock{Text: longText}}},
		// Recent messages (will be kept)
		{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock{Text: "recent question"}}},
		{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.TextBlock{Text: "recent answer"}}},
	}

	mock := &mockProvider{response: "Summary of the old conversation."}
	cfg := CompactionConfig{
		TriggerTokens:   100, // Low threshold to force compaction
		KeepRecentPairs: 1,   // Keep only last 1 pair (2 messages)
	}

	result, err := CompactMessages(context.Background(), messages, mock, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect: 1 summary + 2 recent = 3 messages
	if len(result) != 3 {
		t.Fatalf("expected 3 messages (summary + 2 recent), got %d", len(result))
	}

	// First message should be the summary (assistant role).
	if result[0].Role != llm.RoleAssistant {
		t.Fatalf("expected summary role=assistant, got %s", result[0].Role)
	}
	summaryText := ""
	for _, block := range result[0].Content {
		if tb, ok := block.(llm.TextBlock); ok {
			summaryText = tb.Text
		}
	}
	if !strings.Contains(summaryText, "Summary of the old conversation") {
		t.Fatalf("expected summary content, got %q", summaryText)
	}

	// Last two messages should be the recent ones.
	recentText := ""
	for _, block := range result[1].Content {
		if tb, ok := block.(llm.TextBlock); ok {
			recentText = tb.Text
		}
	}
	if recentText != "recent question" {
		t.Fatalf("expected recent question preserved, got %q", recentText)
	}
}
