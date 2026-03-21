package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/avifenesh/cairn/internal/llm"
)

const extractionPrompt = `Analyze this conversation and extract facts worth remembering across sessions.

Extract ONLY:
- User preferences (editor settings, language choices, workflow habits)
- Project conventions (branching strategy, naming patterns, tech stack decisions)
- Architectural decisions (why X was chosen over Y)
- Hard rules (things that must always/never be done)

Do NOT extract:
- Transient task details (files currently being edited, in-progress errors)
- Information that would already be in the system prompt
- Obvious or generic knowledge everyone knows
- Greetings, pleasantries, or meta-conversation

Output a JSON array. Each item must have "content" (a single self-contained statement) and "category" (one of: fact, preference, decision, hard_rule).
If there is nothing worth remembering, output an empty array: []

Example output:
[{"content": "User prefers dark mode in all editors", "category": "preference"}, {"content": "Always create feature branches, never commit to main", "category": "hard_rule"}]`

// Similarity thresholds for dedup classification.
// These are compared against blended hybrid search scores (0.3*keyword + 0.7*cosine),
// NOT raw cosine similarity. With nomic-embed-text, near-identical paraphrases
// produce blended scores ~0.35-0.57 depending on keyword overlap.
const (
	thresholdDuplicate = 0.40 // Above this = already known, skip
	thresholdUpdate    = 0.30 // Between update and duplicate = refine existing
)

// Extractor automatically extracts memories from completed conversations.
// It follows the journaler pattern: fire-and-forget after session ends.
type Extractor struct {
	memService *Service
	provider   llm.Provider
	model      string
	logger     *slog.Logger
}

// NewExtractor creates a memory extractor.
func NewExtractor(memService *Service, provider llm.Provider, model string, logger *slog.Logger) *Extractor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Extractor{
		memService: memService,
		provider:   provider,
		model:      model,
		logger:     logger,
	}
}

type extractedFact struct {
	Content  string `json:"content"`
	Category string `json:"category"`
}

// Extract analyzes conversation events and creates proposed memories.
// Designed to be called in a fire-and-forget goroutine after session completion.
func (e *Extractor) Extract(ctx context.Context, transcript string) {
	if e.provider == nil || transcript == "" {
		return
	}

	// Stage 1: Extract facts from conversation via LLM.
	facts, err := e.extractFacts(ctx, transcript)
	if err != nil {
		e.logger.Warn("memory extraction: LLM call failed", "error", err)
		return
	}
	if len(facts) == 0 {
		return
	}

	// Stage 2: Classify each fact against existing memories and apply.
	added, updated, skipped, contradicted := 0, 0, 0, 0
	for _, fact := range facts {
		action := e.classifyFact(ctx, fact)
		switch action {
		case "add":
			cat := normCategory(fact.Category)
			m := &Memory{
				Content:    fact.Content,
				Category:   cat,
				Source:     "auto-extract",
				Confidence: 0.6,
			}
			if err := e.memService.Create(ctx, m); err != nil {
				e.logger.Warn("memory extraction: create failed", "content", truncate(fact.Content, 80), "error", err)
			} else {
				added++
			}
		case "update":
			updated++
		case "skip":
			skipped++
		case "contradict":
			// Old memory was rejected — add the new fact as proposed.
			cat := normCategory(fact.Category)
			m := &Memory{
				Content:    fact.Content,
				Category:   cat,
				Source:     "auto-extract",
				Confidence: 0.7, // Slightly higher confidence — it replaced something.
			}
			if err := e.memService.Create(ctx, m); err != nil {
				e.logger.Warn("memory extraction: create after contradiction failed", "error", err)
			} else {
				added++
			}
			contradicted++
		}
	}

	if added > 0 || updated > 0 || contradicted > 0 {
		e.logger.Info("memory extraction complete",
			"extracted", len(facts), "added", added, "updated", updated,
			"skipped", skipped, "contradicted", contradicted,
		)
	}
}

// extractFacts calls the LLM to extract structured facts from the conversation.
func (e *Extractor) extractFacts(ctx context.Context, transcript string) ([]extractedFact, error) {
	req := &llm.Request{
		Model: e.model,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock{Text: transcript}}},
		},
		System:    extractionPrompt,
		MaxTokens: 4096,
	}

	ch, err := e.provider.Stream(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("stream: %w", err)
	}

	var result strings.Builder
	for ev := range ch {
		switch e := ev.(type) {
		case llm.TextDelta:
			result.WriteString(e.Text)
		case llm.StreamError:
			return nil, fmt.Errorf("stream error: %w", e.Err)
		}
	}

	// Parse JSON from response (handle markdown fences).
	raw := strings.TrimSpace(result.String())
	raw = stripMarkdownFences(raw)

	var facts []extractedFact
	if err := json.Unmarshal([]byte(raw), &facts); err != nil {
		return nil, fmt.Errorf("parse JSON: %w (raw: %s)", err, truncate(raw, 200))
	}

	// Filter empty/invalid facts.
	valid := facts[:0]
	for _, f := range facts {
		if strings.TrimSpace(f.Content) != "" {
			valid = append(valid, f)
		}
	}
	return valid, nil
}

// classifyFact checks if a fact is new, a duplicate, or an update to existing memory.
// Uses embedding-based semantic search instead of a second LLM call.
func (e *Extractor) classifyFact(ctx context.Context, fact extractedFact) string {
	results, err := e.memService.Search(ctx, fact.Content, 5)
	if err != nil {
		// Search failed — treat as new (safe default).
		return "add"
	}

	for _, r := range results {
		if r.Score >= thresholdDuplicate {
			// Very similar — already known, skip.
			return "skip"
		}
		if r.Score >= thresholdUpdate {
			// Similar but different — check for contradiction before updating.
			contradicts, cErr := CheckContradiction(ctx, r.Memory.Content, fact.Content, e.provider, e.model)
			if cErr != nil {
				e.logger.Warn("contradiction check failed, defaulting to update", "error", cErr)
			}
			if contradicts {
				// Reject old memory, return "contradict" so new fact gets added as proposed.
				if err := e.memService.Reject(ctx, r.Memory.ID); err != nil {
					e.logger.Warn("memory extraction: reject contradicted failed", "id", r.Memory.ID, "error", err)
				}
				e.logger.Info("memory contradiction detected",
					"oldID", r.Memory.ID,
					"old", truncate(r.Memory.Content, 80),
					"new", truncate(fact.Content, 80),
				)
				return "contradict"
			}
			// No contradiction — update existing memory.
			r.Memory.Content = fact.Content
			r.Memory.Category = normCategory(fact.Category)
			if err := e.memService.Update(ctx, r.Memory); err != nil {
				e.logger.Warn("memory extraction: update failed",
					"id", r.Memory.ID, "error", err)
			}
			return "update"
		}
	}

	// No similar memory found — this is genuinely new.
	return "add"
}

// normCategory normalizes extraction categories to valid Memory categories.
func normCategory(cat string) Category {
	switch strings.ToLower(strings.TrimSpace(cat)) {
	case "preference":
		return CatPreference
	case "hard_rule":
		return CatHardRule
	case "decision":
		return CatDecision
	case "fact":
		return CatFact
	default:
		return CatFact
	}
}

// stripMarkdownFences removes ```json ... ``` fences that LLMs commonly wrap JSON in.
func stripMarkdownFences(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// Find the first newline (end of opening fence).
	start := strings.IndexByte(s, '\n')
	if start < 0 {
		// Single-line fence like ```json [...] ``` — strip prefix and suffix.
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
		return strings.TrimSpace(s)
	}
	// Multi-line: strip first and last lines.
	end := strings.LastIndex(s, "```")
	if end <= start {
		return s[start+1:]
	}
	return strings.TrimSpace(s[start+1 : end])
}

// truncate shortens a string for logging.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
