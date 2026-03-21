package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
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
	repoDir  string // Git repo for recent changes context (empty = skip)
}

// ReflectionConfig configures the reflection engine.
type ReflectionConfig struct {
	Interval time.Duration // Default: 30 minutes
	RepoDir  string        // Git repo path for change context (empty = skip)
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
		repoDir:  cfg.RepoDir,
	}
}

// ReflectionResult holds proposed changes from a reflection cycle.
type ReflectionResult struct {
	Memories       []ProposedMemory `json:"memories"`
	StaleMemoryIDs []string         `json:"staleMemoryIds,omitempty"` // existing memory IDs that are now outdated
	SoulPatch      string           `json:"soulPatch,omitempty"`
}

// ProposedMemory is a memory suggested by the reflection engine.
type ProposedMemory struct {
	Content    string  `json:"content"`
	Category   string  `json:"category"`
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

	// 4. Gather recent code changes (merged PRs, commits) for ground truth.
	recentChanges := r.gatherRecentChanges()

	// 5. Build prompt.
	prompt := r.buildPrompt(entries, existingMemories, soulContent, recentChanges)

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
// validCategories is the set of known memory categories.
var validCategories = map[memory.Category]bool{
	memory.CatFact:         true,
	memory.CatPreference:   true,
	memory.CatDecision:     true,
	memory.CatHardRule:     true,
	memory.CatWritingStyle: true,
}

func (r *ReflectionEngine) Apply(ctx context.Context, result *ReflectionResult) error {
	// Create proposed memories.
	for _, pm := range result.Memories {
		cat := memory.Category(pm.Category)
		if !validCategories[cat] {
			cat = memory.CatFact
		}
		m := &memory.Memory{
			Content:    pm.Content,
			Category:   cat,
			Scope:      memory.ScopeGlobal,
			Status:     memory.StatusProposed,
			Confidence: pm.Confidence,
			Source:     "reflection",
		}
		if err := r.memories.Create(ctx, m); err != nil {
			slog.Warn("reflection: failed to create memory", "error", err, "content", pm.Content)
		}
	}

	// Reject stale memories identified by the LLM.
	for _, id := range result.StaleMemoryIDs {
		if id == "" {
			continue
		}
		if err := r.memories.Reject(ctx, id); err != nil {
			slog.Warn("reflection: failed to reject stale memory", "id", id, "error", err)
		} else {
			slog.Info("reflection: rejected stale memory", "id", id)
		}
	}

	return nil
}

func (r *ReflectionEngine) buildPrompt(entries []*JournalEntry, memories []*memory.Memory, soulContent, recentChanges string) string {
	var b strings.Builder

	b.WriteString(`You are a reflection engine analyzing an agent's recent activity.
Your THREE jobs:
1. Detect patterns and propose NEW memories from recent sessions
2. Review EXISTING memories and flag any that are NOW STALE or WRONG
3. Cross-reference memories against RECENT CODE CHANGES — if a PR or commit
   fixed an issue described in a memory, that memory is STALE

Facts change: bugs get fixed, configs change, tools get upgraded, projects evolve.
A memory that was correct last week may be wrong today.

CRITICAL: The "Recent Code Changes" section shows what was actually merged/deployed.
If a memory says "X is broken" or "X has high failure rate" but a PR was merged
that fixes X, the memory is STALE. Flag it. Don't wait for sessions to confirm —
the code change IS the ground truth.

Respond with ONLY valid JSON (no markdown fences):
{
  "memories": [{"content": "...", "category": "fact|preference|decision|hard_rule", "confidence": 0.0-1.0}],
  "staleMemoryIds": ["id1", "id2"],
  "soulPatch": "optional text to add to SOUL.md if a behavioral pattern is strong"
}

Rules:
- Only propose memories with confidence >= 0.6
- Don't duplicate existing memories
- soulPatch should only be set for strong, repeated patterns (3+ occurrences)
- Keep memories concise (1-2 sentences)
- staleMemoryIds: list IDs of existing memories that recent sessions OR code changes CONTRADICT
  (e.g., "shell has 50% failure rate" when a PR fixed shell reliability)
- Be aggressive about staleness: merged PRs are ground truth, not just evidence

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

	// Existing memories — include IDs so the LLM can flag stale ones.
	if len(memories) > 0 {
		b.WriteString("\n## Existing Memories (do NOT duplicate — flag stale ones by ID)\n\n")
		for _, m := range memories {
			fmt.Fprintf(&b, "- [%s] (id:%s) %s\n", m.Category, m.ID, m.Content)
		}
	}

	// Recent code changes (merged PRs, commits) — ground truth for staleness.
	if recentChanges != "" {
		b.WriteString("\n## Recent Code Changes (GROUND TRUTH — use to invalidate stale memories)\n\n")
		b.WriteString(recentChanges)
		b.WriteString("\n")
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

// gatherRecentChanges collects recent git commits and merged PRs from the repo.
// This provides ground truth for the LLM to invalidate stale memories.
func (r *ReflectionEngine) gatherRecentChanges() string {
	if r.repoDir == "" {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var b strings.Builder

	// Recent commits on main (last 48h).
	cmd := exec.CommandContext(ctx, "git", "log", "main", "--oneline", "--since=48 hours ago", "--no-merges", "-20")
	cmd.Dir = r.repoDir
	if out, err := cmd.Output(); err == nil && len(out) > 0 {
		b.WriteString("Recent commits on main (last 48h):\n")
		b.Write(out)
		b.WriteString("\n")
	}

	// Recently merged PRs on main (last 48h) — key ground truth.
	// Include both merge commits and PR-style non-merge commits (e.g., squash/rebase merges).
	var prSubjects []string

	// 1) Traditional merge commits, which typically represent merged PRs.
	cmd = exec.CommandContext(ctx, "git", "log", "main", "--merges", "--since=48 hours ago", "-20",
		"--format=%s") // subject line only (includes PR title)
	cmd.Dir = r.repoDir
	if out, err := cmd.Output(); err == nil && len(out) > 0 {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				prSubjects = append(prSubjects, line)
			}
		}
	}

	// 2) Squash/rebase merges on main: non-merge commits whose subject looks like a PR title.
	cmd = exec.CommandContext(ctx, "git", "log", "main", "--no-merges", "--since=48 hours ago", "-100",
		"--format=%s") // subject line only
	cmd.Dir = r.repoDir
	if out, err := cmd.Output(); err == nil && len(out) > 0 {
		for _, line := range strings.Split(string(out), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			// Heuristics for PR-style commits, e.g. "Add feature X (#123)" or "Merge pull request #123".
			if strings.Contains(trimmed, "(#") || strings.Contains(trimmed, "pull request") {
				prSubjects = append(prSubjects, trimmed)
			}
		}
	}

	if len(prSubjects) > 0 {
		b.WriteString("Recently merged PRs:\n")
		b.WriteString(strings.Join(prSubjects, "\n"))
		b.WriteString("\n\n")
	}

	return b.String()
}

func (r *ReflectionEngine) parseResult(raw string) *ReflectionResult {
	raw = stripMarkdownFences(raw)

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
		MaxTokens: 8192, // unlimited sub — let it think deeply about patterns
	}

	ch, err := r.provider.Stream(ctx, req)
	if err != nil {
		return "", err
	}

	var result strings.Builder
	for ev := range ch {
		switch e := ev.(type) {
		case llm.TextDelta:
			result.WriteString(e.Text)
		case llm.ReasoningDelta:
			// Thinking — let it reason about patterns, JSON comes in TextDelta
		case llm.StreamError:
			return "", fmt.Errorf("stream error: %w", e.Err)
		}
	}

	if result.Len() == 0 {
		return "", fmt.Errorf("empty LLM response")
	}
	return result.String(), nil
}
