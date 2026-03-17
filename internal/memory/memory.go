// Package memory implements the three-tier memory system:
// semantic (facts/preferences), episodic (journal), and procedural (soul).
//
// It provides SQLite-backed storage, keyword + vector search with MMR
// re-ranking, and a hot-reloadable SOUL.md loader.
package memory

import "time"

// Category classifies what kind of knowledge a memory represents.
type Category string

const (
	CatFact         Category = "fact"
	CatPreference   Category = "preference"
	CatHardRule     Category = "hard_rule"
	CatDecision     Category = "decision"
	CatWritingStyle Category = "writing_style"
)

// Scope defines the visibility boundary of a memory.
type Scope string

const (
	ScopePersonal Scope = "personal"
	ScopeProject  Scope = "project"
	ScopeGlobal   Scope = "global"
)

// Status tracks the lifecycle of a proposed memory.
type Status string

const (
	StatusProposed Status = "proposed"
	StatusAccepted Status = "accepted"
	StatusRejected Status = "rejected"
)

// Memory is a semantic memory entry — a fact, preference, rule, or decision.
type Memory struct {
	ID         string
	Content    string
	Category   Category
	Scope      Scope
	Status     Status
	Confidence float64
	Source     string
	UseCount   int
	Embedding  []float32
	CreatedAt  time.Time
	UpdatedAt  time.Time
	LastUsedAt *time.Time
	Metadata   map[string]any
}

// SearchResult pairs a memory with its relevance score (0.0–1.0).
type SearchResult struct {
	Memory *Memory
	Score  float64
}

// ListOpts configures filtering for memory listing.
type ListOpts struct {
	Status   Status
	Category Category
	Scope    Scope
	Limit    int
}
