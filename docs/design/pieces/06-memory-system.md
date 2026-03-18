# Piece 6: Memory System

> Three-tier memory: semantic (facts), episodic (experiences), procedural (rules/soul).

## Architecture

```
┌─────────────┐  ┌──────────────┐  ┌──────────────┐
│  Semantic    │  │  Episodic    │  │  Procedural  │
│  (facts)    │  │  (journal)   │  │  (soul)      │
│             │  │              │  │              │
│ Memories DB │  │ Session logs │  │ SOUL.md      │
│ Embeddings  │  │ Summaries    │  │ Skills       │
│ RAG search  │  │ Last 48h     │  │ Hard rules   │
└──────┬──────┘  └──────┬───────┘  └──────┬───────┘
       │                │                  │
       └────────────────┴──────────────────┘
                        │
              ┌─────────┴──────────┐
              │  Context Builder   │
              │  (token-budgeted)  │
              └────────────────────┘
```

## Interface

```go
type MemoryService struct {
    store      MemoryStore
    embedder   Embedder
    bus        *eventbus.Bus
}

type Memory struct {
    ID         string
    Content    string
    Category   Category    // fact, preference, hard_rule, decision, writing_style
    Scope      Scope       // personal, project, global
    Status     Status      // proposed, accepted, rejected
    Confidence float64     // 0.0-1.0
    UseCount   int
    Embedding  []float32   // 1024-dim vector
    CreatedAt  time.Time
    UpdatedAt  time.Time
    LastUsedAt time.Time
}

type Category string
const (
    Fact         Category = "fact"
    Preference   Category = "preference"
    HardRule     Category = "hard_rule"
    Decision     Category = "decision"
    WritingStyle Category = "writing_style"
)

// Core operations
func (s *MemoryService) Create(ctx context.Context, m *Memory) error
func (s *MemoryService) Search(ctx context.Context, query string, limit int) ([]*Memory, error) // RAG
func (s *MemoryService) Accept(ctx context.Context, id string) error
func (s *MemoryService) Reject(ctx context.Context, id string) error
func (s *MemoryService) Compact(ctx context.Context) error  // merge duplicates, decay old
```

## Episodic Memory (Session Journal)

```go
type JournalEntry struct {
    ID          string
    TaskID      string
    SessionID   string
    Summary     string      // LLM-generated summary of what happened
    Decisions   []string    // Key decisions made
    Errors      []string    // Errors encountered
    Learnings   []string    // What was learned
    Entities    []string    // Files, PRs, people mentioned
    CreatedAt   time.Time
}

// Auto-generated after each task completion
// Last 48h of journal entries injected into agent context
```

## Procedural Memory (Soul)

```go
type Soul struct {
    Content   string        // SOUL.md content
    FilePath  string        // path to SOUL.md
    watcher   *fsnotify.Watcher // hot-reload on change
}

func (s *Soul) Load() error
func (s *Soul) Watch() error  // re-read on file change
func (s *Soul) Propose(patch string) error // create doc_patch artifact for review
```

## Context Builder (token-budgeted injection)

```go
type ContextBuilder struct {
    tokenBudget int          // e.g., 4000 tokens for memories
    hardRuleReserve int      // e.g., 500 tokens always reserved for hard rules
}

func (b *ContextBuilder) Build(ctx context.Context, query string, memories *MemoryService, journal []*JournalEntry, soul *Soul) string {
    // 1. Always include hard rules (reserved budget)
    // 2. RAG search for relevant memories (remaining budget)
    // 3. MMR re-ranking for diversity
    // 4. Include last 48h journal summaries
    // 5. Include soul identity section
    // Return assembled context string
}
```

## Reflection Engine

```go
type ReflectionEngine struct {
    llm      llm.Client
    memories *MemoryService
    journal  *JournalStore
    soul     *Soul
    interval time.Duration  // default: 30min
}

// Periodic: reads journal + memories, detects patterns, proposes new memories + soul patches
func (r *ReflectionEngine) Reflect(ctx context.Context) error
```

## Subphases

| # | Subphase | Depends On | Status |
|---|----------|------------|--------|
| 6.1 | Memory store (SQLite + embeddings) | Nothing | Done (PR #2) |
| 6.2 | Embedding service (local or API) | Nothing | Done (NoopEmbedder, keyword-only) |
| 6.3 | RAG search with MMR re-ranking | 6.1, 6.2 | Done (PR #2) |
| 6.4 | Session journaler | 4 (agent), 2 (LLM) | Done (PR #10, in agent pkg) |
| 6.5 | Soul loader + hot-reload | Nothing | Done (PR #2) |
| 6.6 | Context builder (token-budgeted) | 6.1, 6.3, 6.4, 6.5 | Partial (modes.go) |
| 6.7 | Reflection engine | 6.1, 6.4, 6.5, 2 (LLM) | Done (PR #10, in agent pkg) |
| 6.8 | Memory compaction + decay | 6.1 | Done (PR #2) |
| 6.9 | Tests | All | Done (24 memory + 19 agent) |
