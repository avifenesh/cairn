package agent

import (
	"fmt"
	"sort"
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
			MaxRounds: 20,
			Prompt:    "You are in talk mode. Give concise, helpful answers. Use tools to look things up when needed.",
		},
		tool.ModeWork: {
			Mode:      tool.ModeWork,
			MaxRounds: 40,
			Prompt:    "You are in work mode. Complete tasks thoroughly: write files, run commands, create artifacts. Be systematic and verify your work.",
		},
		tool.ModeCoding: {
			Mode:      tool.ModeCoding,
			MaxRounds: 200,
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

		// Track memory usage (fire-and-forget with detached context).
		if len(result.InjectedMemoryIDs) > 0 {
			go ctxBuilder.MarkUsed(result.InjectedMemoryIDs)
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

	// Skill catalog: always show available skills by name + description (frontmatter only).
	if ctx.ToolSkills != nil {
		skills := ctx.ToolSkills.List()
		if len(skills) > 0 {
			sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
			var sb strings.Builder
			sb.WriteString("## Available Skills\n")
			sb.WriteString("Use `cairn.loadSkill` to activate a skill when a task matches.\n\n")
			for _, sk := range skills {
				fmt.Fprintf(&sb, "- **%s**: %s\n", sk.Name, sk.Description)
			}
			parts = append(parts, sb.String())
		}
	}

	// Always-included skills: inject full content of skills with inclusion=always.
	// These provide core behavioral guidance (proactive-agent, self-improving, etc.)
	if ctx.ToolSkills != nil {
		var alwaysBuf strings.Builder
		for _, sk := range ctx.ToolSkills.List() {
			if sk.Inclusion == "always" && sk.Content != "" {
				if alwaysBuf.Len() == 0 {
					alwaysBuf.WriteString("## Core Skills (always active)\n")
				}
				fmt.Fprintf(&alwaysBuf, "### %s\n%s\n\n", sk.Name, sk.Content)
			}
		}
		if alwaysBuf.Len() > 0 {
			parts = append(parts, alwaysBuf.String())
		}
	}

	// Session-loaded skills: inject full content of explicitly loaded skills.
	if ctx.Session != nil && len(ctx.Session.ActiveSkills) > 0 {
		var sb strings.Builder
		sb.WriteString("## Active Skills (session-loaded)\n")
		for _, sk := range ctx.Session.ActiveSkills {
			fmt.Fprintf(&sb, "### %s\n%s\n\n", sk.Name, sk.Content)
		}
		parts = append(parts, sb.String())
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
