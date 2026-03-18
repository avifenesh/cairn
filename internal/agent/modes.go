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

// BuildSystemPrompt assembles the full system prompt using the context builder
// for token-budgeted memory injection with hard rule reservation, decay scoring,
// adversarial sanitization, and journal digest.
func BuildSystemPrompt(ctx *InvocationContext, modeConfig *ModeConfig, ctxBuilder *memory.ContextBuilder, journalEntries []memory.JournalDigestEntry) string {
	var parts []string

	// Identity.
	parts = append(parts, "You are Cairn, a personal agent operating system.")

	// Mode instructions.
	parts = append(parts, fmt.Sprintf("## Mode: %s\n%s", modeConfig.Mode, modeConfig.Prompt))

	// Context builder: soul + memories + journal (token-budgeted).
	if ctxBuilder != nil {
		soulContent := ""
		if ctx.Soul != nil {
			soulContent = ctx.Soul.Content()
		}

		result := ctxBuilder.Build(ctx.Context, ctx.UserMessage, soulContent, journalEntries)
		if result.Text != "" {
			parts = append(parts, result.Text)
		}

		// Track memory usage (fire-and-forget).
		if len(result.InjectedMemoryIDs) > 0 {
			go ctxBuilder.MarkUsed(ctx.Context, result.InjectedMemoryIDs)
		}
	} else {
		// Fallback: basic soul injection when no context builder available.
		if ctx.Soul != nil {
			if content := ctx.Soul.Content(); content != "" {
				parts = append(parts, fmt.Sprintf("## Soul\n%s", content))
			}
		}

		// Fallback: basic memory injection.
		if ctx.Memory != nil && ctx.UserMessage != "" {
			memories := injectMemoriesBasic(ctx, 4000)
			if memories != "" {
				parts = append(parts, fmt.Sprintf("## Relevant Memories\n%s", memories))
			}
		}
	}

	return strings.Join(parts, "\n\n")
}

// injectMemoriesBasic is the simple fallback when no ContextBuilder is configured.
func injectMemoriesBasic(ctx *InvocationContext, tokenBudget int) string {
	if ctx.Memory == nil {
		return ""
	}

	results, err := ctx.Memory.Search(ctx.Context, ctx.UserMessage, 10)
	if err != nil || len(results) == 0 {
		return ""
	}

	var sb strings.Builder
	charBudget := tokenBudget * 4

	for _, r := range results {
		line := formatMemorySimple(r)
		if sb.Len()+len(line) > charBudget {
			break
		}
		sb.WriteString(line)
		sb.WriteByte('\n')
	}

	return sb.String()
}

func formatMemorySimple(r memory.SearchResult) string {
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
