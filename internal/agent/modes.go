package agent

import (
	"fmt"
	"strings"

	"github.com/avifenesh/cairn/internal/memory"
	"github.com/avifenesh/cairn/internal/tool"
)

// ModeConfig defines behavior for each agent mode.
type ModeConfig struct {
	Mode      tool.Mode
	MaxRounds int
	Prompt    string // mode-specific system prompt addendum
}

// DefaultModes returns the built-in mode configurations.
func DefaultModes() map[tool.Mode]*ModeConfig {
	return map[tool.Mode]*ModeConfig{
		tool.ModeTalk: {
			Mode:      tool.ModeTalk,
			MaxRounds: 10,
			Prompt:    "You are in talk mode. Give concise, helpful answers. Use tools to look things up when needed.",
		},
		tool.ModeWork: {
			Mode:      tool.ModeWork,
			MaxRounds: 10,
			Prompt:    "You are in work mode. Complete tasks thoroughly: write files, run commands, create artifacts. Be systematic and verify your work.",
		},
		tool.ModeCoding: {
			Mode:      tool.ModeCoding,
			MaxRounds: 100,
			Prompt:    "You are in coding mode. Write, edit, test, and commit code. Follow project conventions. Run tests after changes. Create PRs when work is complete.",
		},
	}
}

// BuildSystemPrompt assembles the full system prompt from mode config,
// soul content, and relevant memories.
func BuildSystemPrompt(ctx *InvocationContext, modeConfig *ModeConfig) string {
	var parts []string

	// Identity.
	parts = append(parts, "You are Cairn, a personal agent operating system.")

	// Soul content (procedural memory).
	if ctx.Soul != nil {
		if content := ctx.Soul.Content(); content != "" {
			parts = append(parts, fmt.Sprintf("## Soul\n%s", content))
		}
	}

	// Mode instructions.
	parts = append(parts, fmt.Sprintf("## Mode: %s\n%s", modeConfig.Mode, modeConfig.Prompt))

	// Relevant memories (semantic memory, token-budgeted).
	if ctx.Memory != nil && ctx.UserMessage != "" {
		memories := injectMemories(ctx, 4000) // ~4000 token budget
		if memories != "" {
			parts = append(parts, fmt.Sprintf("## Relevant Memories\n%s", memories))
		}
	}

	return strings.Join(parts, "\n\n")
}

// injectMemories searches for relevant memories and formats them for injection.
func injectMemories(ctx *InvocationContext, tokenBudget int) string {
	if ctx.Memory == nil {
		return ""
	}

	results, err := ctx.Memory.Search(ctx.Context, ctx.UserMessage, 10)
	if err != nil || len(results) == 0 {
		return ""
	}

	var sb strings.Builder
	charBudget := tokenBudget * 4 // rough: 1 token ≈ 4 chars

	for _, r := range results {
		line := formatMemory(r)
		if sb.Len()+len(line) > charBudget {
			break
		}
		sb.WriteString(line)
		sb.WriteByte('\n')
	}

	return sb.String()
}

func formatMemory(r memory.SearchResult) string {
	m := r.Memory
	prefix := ""
	switch m.Category {
	case memory.CatHardRule:
		prefix = "[RULE] "
	case memory.CatPreference:
		prefix = "[PREF] "
	case memory.CatFact:
		prefix = "[FACT] "
	case memory.CatDecision:
		prefix = "[DECISION] "
	}
	return fmt.Sprintf("- %s%s", prefix, m.Content)
}
