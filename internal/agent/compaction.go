package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/avifenesh/cairn/internal/llm"
)

// CompactionConfig controls session compaction behavior.
type CompactionConfig struct {
	TriggerTokens   int // Compact when estimated tokens exceed this (default: 80000)
	KeepRecentPairs int // Keep last N user+assistant message pairs verbatim (default: 10)
	MaxToolOutput   int // Truncate individual tool outputs to this many chars (default: 8000)
}

// DefaultCompactionConfig returns sensible defaults for GLM-5-turbo (128K context).
func DefaultCompactionConfig() CompactionConfig {
	return CompactionConfig{
		TriggerTokens:   80000,
		KeepRecentPairs: 10,
		MaxToolOutput:   8000,
	}
}

const compactionPrompt = `Summarize this conversation segment concisely. Preserve:
- Decisions made and their reasoning
- Files modified and changes made
- Errors encountered and how they were resolved
- User preferences expressed
- Task progress and current state
- Key tool outputs and their significance

Format as a structured summary, not a transcript. Be concise but complete.`

// CompactMessages compresses conversation history by summarizing old messages
// and keeping recent ones verbatim. Returns the original messages unchanged
// if under the token threshold.
func CompactMessages(ctx context.Context, messages []llm.Message, provider llm.Provider, cfg CompactionConfig) ([]llm.Message, error) {
	if cfg.TriggerTokens <= 0 || len(messages) == 0 {
		return messages, nil
	}

	tokens := EstimateMessageTokens(messages)
	if tokens <= cfg.TriggerTokens {
		return messages, nil
	}

	// Determine split point: keep last KeepRecentPairs*2 messages.
	keepCount := cfg.KeepRecentPairs * 2
	if keepCount >= len(messages) {
		return messages, nil // Not enough messages to compact
	}

	old := messages[:len(messages)-keepCount]
	recent := messages[len(messages)-keepCount:]

	// Format old messages as text for summarization.
	var transcript strings.Builder
	for _, msg := range old {
		role := string(msg.Role)
		transcript.WriteString(fmt.Sprintf("[%s] ", role))
		for _, block := range msg.Content {
			switch b := block.(type) {
			case llm.TextBlock:
				transcript.WriteString(b.Text)
			case llm.ToolUseBlock:
				transcript.WriteString(fmt.Sprintf("<tool:%s>", b.Name))
			case llm.ToolResultBlock:
				// Truncate tool output for the summary input too.
				output := TruncateToolOutput(b.Content, 500)
				transcript.WriteString(fmt.Sprintf("<result:%s>", output))
			case llm.ReasoningBlock:
				// Skip reasoning in summary input — it's internal.
			}
		}
		transcript.WriteString("\n")
	}

	// Ask LLM to summarize.
	summaryReq := &llm.Request{
		Messages: []llm.Message{
			{
				Role:    llm.RoleUser,
				Content: []llm.ContentBlock{llm.TextBlock{Text: transcript.String()}},
			},
		},
		System:    compactionPrompt,
		MaxTokens: 8192,
	}

	var summary strings.Builder
	events, err := provider.Stream(ctx, summaryReq)
	if err != nil {
		return nil, fmt.Errorf("compaction: LLM stream: %w", err)
	}
	for ev := range events {
		switch e := ev.(type) {
		case llm.TextDelta:
			summary.WriteString(e.Text)
		case llm.StreamError:
			return nil, fmt.Errorf("compaction: LLM error: %w", e.Err)
		}
	}

	if summary.Len() == 0 {
		return nil, fmt.Errorf("compaction: LLM returned empty summary")
	}

	// Build compacted message list: summary + cleaned recent messages.
	summaryMsg := llm.Message{
		Role: llm.RoleAssistant,
		Content: []llm.ContentBlock{
			llm.TextBlock{Text: "[Previous conversation summary]\n" + summary.String()},
		},
	}

	cleaned := stripOrphanedToolResults(recent)
	result := make([]llm.Message, 0, 1+len(cleaned))
	result = append(result, summaryMsg)
	result = append(result, cleaned...)

	return result, nil
}

// EstimateMessageTokens estimates the total token count for a slice of messages.
// Uses the standard ~4 characters per token heuristic.
func EstimateMessageTokens(messages []llm.Message) int {
	total := 0
	for _, msg := range messages {
		for _, block := range msg.Content {
			switch b := block.(type) {
			case llm.TextBlock:
				total += len(b.Text)
			case llm.ToolUseBlock:
				total += len(b.Name) + len(b.Input)
			case llm.ToolResultBlock:
				total += len(b.Content)
			case llm.ReasoningBlock:
				total += len(b.Text)
			}
		}
	}
	return (total + 3) / 4 // ceiling division
}

// TruncateToolOutput truncates a tool output string to max characters,
// keeping head (60%) and tail (40%) with a truncation marker in the middle.
func TruncateToolOutput(output string, max int) string {
	if max <= 0 || len(output) <= max {
		return output
	}

	headSize := max * 6 / 10
	tailSize := max - headSize
	dropped := len(output) - max

	return output[:headSize] +
		fmt.Sprintf("\n\n... [%d chars truncated] ...\n\n", dropped) +
		output[len(output)-tailSize:]
}

// stripOrphanedToolResults removes ToolResultBlocks that reference
// ToolUseBlocks not present in the message list. APIs reject orphaned references.
func stripOrphanedToolResults(messages []llm.Message) []llm.Message {
	// Collect all ToolUseBlock IDs.
	toolUseIDs := make(map[string]bool)
	for _, msg := range messages {
		for _, block := range msg.Content {
			if tu, ok := block.(llm.ToolUseBlock); ok {
				toolUseIDs[tu.ID] = true
			}
		}
	}

	// Filter out orphaned ToolResultBlocks.
	result := make([]llm.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == llm.RoleTool {
			// Check if ALL tool results in this message have valid references.
			var validBlocks []llm.ContentBlock
			for _, block := range msg.Content {
				if tr, ok := block.(llm.ToolResultBlock); ok {
					if toolUseIDs[tr.ToolUseID] {
						validBlocks = append(validBlocks, block)
					}
				} else {
					validBlocks = append(validBlocks, block)
				}
			}
			if len(validBlocks) > 0 {
				result = append(result, llm.Message{Role: msg.Role, Content: validBlocks})
			}
			continue
		}
		result = append(result, msg)
	}
	return result
}
