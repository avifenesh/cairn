# Phase 8 Intelligence — Making Cairn Smart

Research-backed plan for session compaction, auto-memory extraction, and contradiction detection.
Based on 108 sources analyzed (Eino, Gollem, Plandex, ADK-Go, MemGPT, Zep, Mem0, LangChain, 42 papers/blogs).
See `agent-knowledge/session-compaction.md` and `agent-knowledge/auto-memory-extraction.md` for full research.

## Current State — ALL COMPLETE

| PR | Feature | Status |
|----|---------|--------|
| #63 | Embeddings (local Ollama nomic-embed-text 768d) | Merged, deployed |
| #67 | Session Compaction (SummaryBuffer 80K trigger) | Merged, deployed |
| #70 | Auto-Extract Memories (Mem0 pipeline) | Merged, deployed |
| #79 | Contradiction Detection (LLM YES/NO judge) | Merged, deployed |
| #73-78 | Voice STT/TTS (whisper.cpp + edge-tts + Telegram) | Merged, deployed |

All three intelligence PRs (A, B, C) plus voice are complete. 25 accepted memories with embeddings, semantic search confirmed working.

---

## PR A — Session Compaction (`internal/agent/compaction.go`)

### Pattern: SummaryBufferMemory (production standard)

Keep system prompt + last N messages verbatim + summarize everything older.
Research confirms this is the universal pattern (Eino, Gollem, LangChain, MemGPT).

### Architecture

```
Before compaction:
  [system] [user₁] [assistant₁] [tool_use₁] [tool_result₁] ... [userₙ] [assistantₙ]

After compaction:
  [system] [summary_message] [user_recent] [assistant_recent] ...
```

### Implementation

#### 1. `internal/agent/compaction.go` — Core compaction logic

```go
type CompactionConfig struct {
    TriggerTokens    int     // Trigger threshold (default: 100K tokens)
    KeepRecentPairs  int     // Keep last N user+assistant pairs verbatim (default: 10)
    SummaryModel     string  // Model for summarization (default: same as chat)
    MaxSummaryTokens int     // Max summary length (default: 2000)
}

func CompactSession(ctx context.Context, events []*Event, llm Provider, cfg CompactionConfig) ([]*Event, error)
```

**Steps** (from Eino + Gollem patterns):
1. Estimate total tokens (`EstimateTokens` from context.go)
2. If under threshold → return events unchanged
3. Split: `system` + `old_events` + `recent_events` (last `KeepRecentPairs` pairs)
4. **Truncate tool outputs** in old events first (Gollem pattern: keep head 60% + tail 40%, max 500 chars each)
5. Summarize old events via LLM:
   ```
   Summarize this conversation segment concisely. Preserve:
   - Decisions made and their reasoning
   - Files modified and changes made
   - Errors encountered and how they were resolved
   - User preferences expressed
   - Task progress and current state
   Format as a structured summary, not a transcript.
   ```
6. **Strip orphaned tool results** — tool_result referencing dropped tool_use IDs (Gollem's critical fix)
7. Tag summary event with `metadata: {"compacted": true}` so it won't be re-summarized
8. Return: `[system, summary_event, ...recent_events]`

#### 2. Integration point: `internal/agent/react.go`

Insert before the LLM call (line ~72, after `messages := invCtx.Session.History()`):

```go
if len(messages) > 0 && estimateTokens(messages) > compactionThreshold {
    compacted, err := CompactSession(ctx, messages, llm, compactionCfg)
    if err != nil {
        logger.Warn("compaction failed, using full history", "error", err)
    } else {
        messages = compacted
        // Persist compacted session
        invCtx.Session.ReplaceHistory(compacted)
    }
}
```

#### 3. Tool output truncation: `internal/agent/truncate.go`

Applied immediately when tool results arrive (before storage), not during compaction:

```go
const MaxToolOutputChars = 8000

func TruncateToolOutput(output string) string {
    if len(output) <= MaxToolOutputChars {
        return output
    }
    head := output[:MaxToolOutputChars*6/10]  // 60% head
    tail := output[len(output)-MaxToolOutputChars*4/10:]  // 40% tail
    dropped := len(output) - MaxToolOutputChars
    return head + fmt.Sprintf("\n\n... [%d chars truncated] ...\n\n", dropped) + tail
}
```

#### 4. Config additions

```
COMPACTION_TRIGGER_TOKENS=100000    # default 100K
COMPACTION_KEEP_RECENT=10          # keep last 10 message pairs
```

#### 5. Tests

- `TestCompactSession_UnderThreshold` — no-op
- `TestCompactSession_OverThreshold` — summary created, old events removed
- `TestTruncateToolOutput` — head/tail preserved, middle dropped
- `TestStripOrphanedToolResults` — dangling references removed
- `TestCompactSession_TaggedSummarySkipped` — already-compacted summaries not re-summarized

### Reference Research

- Eino: `adk/middlewares/summarization/` — token-threshold triggered, splices back user messages
- Gollem: `core/autocontext.go` — `stripOrphanedToolResults`, composable strategies
- Plandex: `app/server/model/plan/tell_summary.go` — incremental async summaries
- Web: ConversationSummaryBufferMemory (LangChain), Anthropic compact API, Chain of Density (arxiv:2309.04269)

---

## PR B — Auto-Extract Memories (`internal/memory/extractor.go`)

### Pattern: Mem0 two-stage pipeline (extract → classify)

After each session ends, extract facts from the conversation and classify against existing memories.

### Architecture

```
Conversation ends
    ↓
Stage 1: Extract facts (LLM call)
    → ["User prefers dark mode", "Project uses Go 1.25", ...]
    ↓
Stage 2: Classify each fact (LLM call)
    → ADD (new fact) / UPDATE (refines existing) / DELETE (contradicts) / NONE (already known)
    ↓
Stage 3: Apply changes
    → Create proposed memories (ADD)
    → Update existing memory content (UPDATE)
    → Reject contradicted memory (DELETE)
```

### Implementation

#### 1. `internal/memory/extractor.go` — Fact extraction + classification

```go
type Extractor struct {
    llm      Provider
    store    *Store
    embedder Embedder
    model    string
}

type ExtractedFact struct {
    Content    string   // The fact
    Category   string   // fact, preference, decision, hard_rule
    Action     string   // ADD, UPDATE, DELETE, NONE
    ExistingID string   // ID of existing memory (for UPDATE/DELETE)
    Confidence float64  // 0-1
}

func (e *Extractor) ExtractAndClassify(ctx context.Context, events []*Event) ([]ExtractedFact, error)
```

**Stage 1 prompt** (from Mem0 + Zep patterns):
```
Analyze this conversation and extract discrete facts, preferences, and decisions.

Rules:
- Each fact should be a single, self-contained statement
- Include the category: fact, preference, decision, or hard_rule
- Only extract information that would be useful to remember across sessions
- Do NOT extract transient task details (file paths being edited, current errors)
- DO extract: user preferences, project conventions, architectural decisions, tool preferences

Output JSON array: [{"content": "...", "category": "..."}]
```

**Stage 2 prompt** (from Mem0 classify pattern):
```
For each extracted fact, classify it against these existing memories:
{existing_memories}

For each fact, respond with:
- ADD: This is genuinely new information
- UPDATE: This refines/updates existing memory {id} — provide the updated content
- DELETE: This contradicts existing memory {id}
- NONE: This is already captured by existing memory {id}

Output JSON array: [{"content": "...", "action": "ADD|UPDATE|DELETE|NONE", "existing_id": "...", "confidence": 0.0-1.0}]
```

#### 2. Integration: Post-session hook in `internal/agent/react.go`

After the ReAct loop completes:
```go
// Extract memories from completed session (fire-and-forget).
if extractor != nil && len(events) > 4 { // skip trivial sessions
    go func() {
        ectx, ecancel := context.WithTimeout(context.Background(), 2*time.Minute)
        defer ecancel()
        facts, err := extractor.ExtractAndClassify(ectx, events)
        if err != nil {
            logger.Warn("memory extraction failed", "error", err)
            return
        }
        applyExtractedFacts(ectx, memService, facts)
    }()
}
```

#### 3. `applyExtractedFacts` — Apply classified changes

```go
func applyExtractedFacts(ctx context.Context, svc *Service, facts []ExtractedFact) {
    for _, f := range facts {
        switch f.Action {
        case "ADD":
            m := &Memory{Content: f.Content, Category: f.Category, Confidence: f.Confidence}
            svc.Create(ctx, m)  // Creates as "proposed"
        case "UPDATE":
            existing, _ := svc.Get(ctx, f.ExistingID)
            if existing != nil {
                existing.Content = f.Content
                svc.Update(ctx, existing)  // Re-embeds
            }
        case "DELETE":
            svc.Reject(ctx, f.ExistingID)
        }
    }
}
```

#### 4. Config

```
MEMORY_AUTO_EXTRACT=true           # enable post-session extraction
MEMORY_EXTRACT_MIN_EVENTS=4       # skip trivial sessions
```

#### 5. Tests

- `TestExtractor_ExtractsPreferences` — conversation about preferences → preference memories
- `TestExtractor_ClassifiesADD` — new fact → proposed memory created
- `TestExtractor_ClassifiesUPDATE` — refined fact → existing memory updated
- `TestExtractor_ClassifiesDELETE` — contradicting fact → existing memory rejected
- `TestExtractor_ClassifiesNONE` — known fact → no change
- `TestExtractor_SkipsShortSessions` — < min events → no extraction

### Reference Research

- Mem0: two-stage extract→classify pipeline, temp UUID anti-hallucination
- Zep: temporal knowledge graph, `valid_until` timestamps for fact invalidation
- MemGPT/Letta: 5 memory management tools (core_memory_append/replace, archival_memory_insert/search)
- Cairn existing: SessionJournaler (entities), ReflectionEngine (periodic), memory lifecycle

---

## PR C — Contradiction Detection (`internal/memory/contradiction.go`)

### Pattern: Embedding similarity + LLM judge (Mem0 + Zep hybrid)

Before accepting a new memory, check if it contradicts existing ones.

### Implementation

```go
func DetectContradictions(ctx context.Context, newContent string, store *Store, embedder Embedder, llm Provider) ([]Contradiction, error) {
    // 1. Find semantically similar memories (cosine > 0.7)
    similar := findSimilar(ctx, newContent, store, embedder, 10)

    // 2. If any are found, ask LLM to check for contradictions
    // Prompt: "Does the new fact contradict any of these existing facts? ..."

    // 3. Return contradictions with suggested resolution
}

type Contradiction struct {
    ExistingID string
    Existing   string
    New        string
    Resolution string // "replace", "merge", "keep_both"
}
```

Integrated into `Service.Create()` — when embedder is active and there are similar memories, run contradiction check before proposing.

---

## Implementation Order

```
PR A: Session Compaction          ─── 3-4 files, prevents context overflow
  ↓
PR B: Auto-Extract Memories       ─── 2 files, learning loop from conversations
  ↓  (depends on A: compacted sessions produce cleaner extractions)
PR C: Contradiction Detection     ─── 1 file, keeps memory store clean
  ↓  (depends on B: extraction generates memories that need dedup)
```

**PR A first** — it's the most critical (prevents crashes in long sessions) and the cleanest to implement.

## Verification

### PR A
- Unit tests for compaction, truncation, orphan stripping
- Integration: start 50+ round coding session, verify compaction triggers
- Check logs: "session compacted: X events → Y events (saved Z tokens)"

### PR B
- Unit tests with mock LLM for extraction + classification
- Integration: have a conversation, check that memories are auto-proposed
- Verify: extracted memories have embeddings (from embedder)

### PR C
- Unit tests with mock similar memories + LLM judge
- Integration: create contradicting fact, verify old memory flagged
