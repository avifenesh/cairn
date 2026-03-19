package agent

import (
	"context"
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
	if !contains(result, "truncated") {
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

	// Verify the orphaned one is gone
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

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
