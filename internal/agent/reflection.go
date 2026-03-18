package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/llm"
	"github.com/avifenesh/cairn/internal/memory"
)

// ReflectionEngine periodically analyzes journal entries and memories to
// detect patterns, propose new memories, and suggest SOUL.md patches.
type ReflectionEngine struct {
	journal  *JournalStore
	memories *memory.Service
	soul     *memory.Soul
	provider llm.Provider
	model    string
	interval time.Duration
}

// ReflectionConfig configures the reflection engine.
type ReflectionConfig struct {
	Interval time.Duration // Default: 30 minutes
}

// NewReflectionEngine creates a reflection engine.
func NewReflectionEngine(journal *JournalStore, memories *memory.Service, soul *memory.Soul, provider llm.Provider, model string, cfg ReflectionConfig) *ReflectionEngine {
	interval := cfg.Interval
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	return &ReflectionEngine{
		journal:  journal,
		memories: memories,
		soul:     soul,
		provider: provider,
		model:    model,
		interval: interval,
	}
}

// ReflectionResult holds proposed changes from a reflection cycle.
type ReflectionResult struct {
	Memories  []ProposedMemory `json:"memories"`
	SoulPatch string           `json:"soulPatch,omitempty"`
}

// ProposedMemory is a memory suggested by the reflection engine.
type ProposedMemory struct {
	Content    string `json:"content"`
	Category   string `json:"category"`
	Confidence float64 `json:"confidence"`
}

// Reflect runs a single reflection cycle: reads recent journal entries and
// existing memories, detects patterns, and proposes new memories or SOUL patches.
func (r *ReflectionEngine) Reflect(ctx context.Context) (*ReflectionResult, error) {
	// 1. Gather recent journal entries (last 48h).
	entries, err := r.journal.Recent(ctx, 48*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("reflection: journal: %w", err)
	}
	const minEntriesForReflection = 2
	if len(entries) < minEntriesForReflection {
		return &ReflectionResult{}, nil // not enough data to reflect on
	}

	if r.provider == nil {
		return nil, fmt.Errorf("reflection: no LLM provider")
	}

	// 2. Get existing accepted memories for context.
	existingMemories, err := r.memories.List(ctx, memory.ListOpts{Status: memory.StatusAccepted, Limit: 50})
	if err != nil {
		return nil, fmt.Errorf("reflection: memories: %w", err)
	}

	// 3. Get current SOUL.md content.
	soulContent := r.soul.Content()

	// 4. Build prompt.
	prompt := r.buildPrompt(entries, existingMemories, soulContent)

	// 5. Call LLM.
	result, err := r.callLLM(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("reflection: llm: %w", err)
	}

	// 6. Parse result.
	return r.parseResult(result), nil
}

// Apply takes a ReflectionResult and creates the proposed memories.
// SOUL patches are returned but not applied automatically (requires approval).
func (r *ReflectionEngine) Apply(ctx context.Context, result *ReflectionResult) error {
	for _, pm := range result.Memories {
		m := &memory.Memory{
			Content:    pm.Content,
			Category:   memory.Category(pm.Category),
			Scope:      memory.ScopeGlobal,
			Status:     memory.StatusProposed,
			Confidence: pm.Confidence,
			Source:     "reflection",
		}
		if err := r.memories.Create(ctx, m); err != nil {
			slog.Warn("reflection: failed to create memory", "error", err, "content", pm.Content)
		}
	}
	return nil
}

func (r *ReflectionEngine) buildPrompt(entries []*JournalEntry, memories []*memory.Memory, soulContent string) string {
	var b strings.Builder

	b.WriteString(`You are a reflection engine analyzing an agent's recent activity.
Your job is to detect patterns, recurring themes, and lessons across sessions.

Respond with ONLY valid JSON (no markdown fences):
{
  "memories": [{"content": "...", "category": "fact|preference|decision|hard_rule", "confidence": 0.0-1.0}],
  "soulPatch": "optional text to add to SOUL.md if a behavioral pattern is strong"
}

Rules:
- Only propose memories with confidence >= 0.6
- Don't duplicate existing memories
- soulPatch should only be set for strong, repeated patterns (3+ occurrences)
- Keep memories concise (1-2 sentences)

`)

	// Recent journal entries.
	fmt.Fprintf(&b, "## Recent Sessions (%d entries, last 48h)\n\n", len(entries))
	for _, e := range entries {
		fmt.Fprintf(&b, "- [%s] %s\n", e.Mode, e.Summary)
		if len(e.Learnings) > 0 {
			fmt.Fprintf(&b, "  Learnings: %s\n", strings.Join(e.Learnings, "; "))
		}
		if len(e.Errors) > 0 {
			fmt.Fprintf(&b, "  Errors: %s\n", strings.Join(e.Errors, "; "))
		}
	}

	// Existing memories (to avoid duplicates).
	if len(memories) > 0 {
		b.WriteString("\n## Existing Memories (do NOT duplicate)\n\n")
		for _, m := range memories {
			fmt.Fprintf(&b, "- [%s] %s\n", m.Category, m.Content)
		}
	}

	// SOUL.md excerpt.
	if soulContent != "" {
		b.WriteString("\n## Current SOUL.md\n\n")
		// Truncate for token budget.
		runes := []rune(soulContent)
		if len(runes) > 1000 {
			b.WriteString(string(runes[:1000]))
			b.WriteString("\n... (truncated)\n")
		} else {
			b.WriteString(soulContent)
		}
	}

	return b.String()
}

func (r *ReflectionEngine) parseResult(raw string) *ReflectionResult {
	// Try to extract JSON from the response (may have extra text around it).
	raw = strings.TrimSpace(raw)

	// Strip markdown fences if present.
	if strings.HasPrefix(raw, "```") {
		if idx := strings.Index(raw[3:], "\n"); idx >= 0 {
			raw = raw[3+idx+1:]
		}
		if idx := strings.LastIndex(raw, "```"); idx >= 0 {
			raw = raw[:idx]
		}
		raw = strings.TrimSpace(raw)
	}

	var result ReflectionResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		slog.Warn("reflection: failed to parse LLM result", "error", err)
		return &ReflectionResult{}
	}

	// Filter out low-confidence memories.
	var filtered []ProposedMemory
	for _, m := range result.Memories {
		if m.Confidence >= 0.6 && m.Content != "" {
			filtered = append(filtered, m)
		}
	}
	result.Memories = filtered

	return &result
}

func (r *ReflectionEngine) callLLM(ctx context.Context, prompt string) (string, error) {
	req := &llm.Request{
		Model: r.model,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock{Text: prompt}}},
		},
		MaxTokens: 1024,
	}

	ch, err := r.provider.Stream(ctx, req)
	if err != nil {
		return "", err
	}

	var result strings.Builder
	for ev := range ch {
		if td, ok := ev.(llm.TextDelta); ok {
			result.WriteString(td.Text)
		}
	}
	return result.String(), nil
}
