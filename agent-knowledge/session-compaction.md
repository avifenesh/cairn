# Learning Guide: LLM Session Compaction and Conversation Summarization

**Generated**: 2026-03-19
**Sources**: 7 local codebases + Cairn internals analyzed
**Depth**: deep

## Prerequisites

- Familiarity with LLM context windows and token limits
- Understanding of Go agent patterns (ReAct loop, message history)
- Basic knowledge of Cairn's `agent/types.go` and `memory/context.go`

## TL;DR

- Every serious Go agent framework (Eino, Gollem, Plandex) solves context overflow differently, but converges on the same two-tier pattern: **truncate tool outputs immediately** on arrival, **summarize conversation history** lazily when the total token count crosses a threshold.
- Cairn currently has **no session compaction**. `Session.History()` returns the full event list unconditionally. Long coding sessions will exceed GLM's context window.
- The correct insert point in Cairn is in `react.go`, immediately after `messages := invCtx.Session.History()`, before the first LLM call.
- Summary messages should be tagged so they survive the next compaction cycle without being re-summarized (Eino's `contentTypeSummary` extra field pattern).
- Orphaned tool results (tool_result referencing a dropped tool_use ID) cause hard API rejections from Anthropic/OpenAI — must be cleaned up during compaction (Gollem's `stripOrphanedToolResults`).

## Core Concepts

### 1. Token Budget Estimation

All frameworks use the same heuristic: **~4 characters per token** for English text. This is the same constant used in `memory/context.go`'s `EstimateTokens`. Some frameworks refine to word-based (~1.3 tokens/word, Gollem). No framework uses exact tokenization at runtime because it adds latency and an external dependency. The 4-chars heuristic under-counts by ~10-15% for code-heavy conversations (identifiers, JSON) — a conservative trigger threshold compensates.

```go
// All frameworks, same formula:
func EstimateTokens(text string) int {
    return (len(text) + 3) / 4 // ceiling division
}
```

### 2. Strategies Observed

#### A. Hard Truncation (tool outputs only)
**Used by**: Gollem (`core/truncate.go`), Eino reduction middleware

Applied immediately when a tool returns output exceeding a byte threshold. Keeps head (60%) and tail (40%) with a marker showing how many tokens were dropped. Operates on individual tool outputs, not the whole history.

```
... [truncated 4200 tokens] ...
```

**When to use**: Any time a single tool call returns large output (file reads, search results, code output). Does not affect conversation coherence.

#### B. Sliding Window (drop oldest pairs)
**Used by**: Gollem `SlidingWindowMemory`, ADK-Go `NumRecentEvents`

Keeps only the last N message pairs. Drops oldest user+assistant pairs to maintain user/assistant alternation. Always preserves the first message (system prompt / task context). Fast but lossy — model loses awareness of early decisions.

```go
// Gollem pattern: keep system prompt + last windowSize*2 messages
start := len(messages) - windowSize*2
result = append([]ModelMessage{messages[0]}, messages[start:]...)
```

**When to use**: Short-lived sessions, chatbots, cases where early context is unimportant.

#### C. Token Budget Pruning (drop oldest until under budget)
**Used by**: Gollem `TokenBudgetMemory`

Drops message pairs from position 1 (after system prompt) until total tokens fit the budget. Drops in pairs to maintain alternation. Less abrupt than sliding window because it adapts to message size.

**When to use**: When message sizes vary widely (some tool outputs huge, most small).

#### D. LLM Summarization (the gold standard)
**Used by**: Eino (`adk/middlewares/summarization/`), Gollem (`core/autocontext.go` + `core/memory/strategy.go`), Plandex (`app/server/model/plan/tell_summary.go`)

Replaces old messages with an LLM-generated summary. Preserves semantic content at the cost of one extra LLM call.

**Pattern** (consistent across all frameworks):
1. Check if `totalTokens > threshold`
2. Separate: keep `messages[0]` (system/task), identify `messages[startRecent:]` to keep, summarize the middle
3. Call LLM with summarization prompt
4. Replace middle with one summary message (as assistant role to maintain alternation)
5. Strip orphaned tool results that reference dropped tool_use IDs

```go
// Gollem autocontext pattern
result := []ModelMessage{firstMsg, summaryMsg}
result = append(result, recentMessages...)
result = stripOrphanedToolResults(result)
```

The summary message is emitted as **assistant role** (not user or system), because Anthropic extracts system-role messages to a separate field, which would create adjacent user messages violating the alternation requirement.

#### E. Tool Output Offloading (filesystem-backed)
**Used by**: Eino reduction middleware (`adk/middlewares/reduction/`)

Two-phase:
1. **Truncation phase**: after tool execution, if output > MaxLengthForTrunc (default 50K chars), write full content to filesystem (`/tmp/trunc/{call_id}`), replace tool result with truncated preview + file path.
2. **Clear phase**: before each LLM call, if total tokens > MaxTokensForClear, iterate historical messages and offload older tool call args+results to files, replacing with `"Content saved to {path}. Use read_file tool to retrieve."`.

The agent must be given a `read_file` tool that can read from the same backend — otherwise the offloaded content is inaccessible.

**When to use**: Coding agents that read large files, run tests with verbose output, or call search APIs.

### 3. Plandex's Incremental Summary Model

Plandex uses a distinct **append-only summary** pattern:

- Each `ConvoSummary` has a `LatestConvoMessageId` and `LatestConvoMessageCreatedAt` timestamp.
- When token limit is hit, it finds the most recent pre-computed summary that brings the conversation under the limit.
- Summary generation happens **asynchronously** in `tell_stream_finish.go` after each reply.
- The summary prompt is explicit: "Treat the summary as append-only. Keep as much information as possible from the existing summary."
- Result: summaries are stored in DB and reused across sessions; the system never re-summarizes already-summarized content.

This is the most production-robust pattern for long-lived planning sessions.

### 4. User Message Preservation (Eino's insight)

Even after summarization, preserving the exact wording of user messages avoids "intent drift" — the tendency of LLM summaries to subtly paraphrase user requests. Eino's summarization middleware:

1. Generates the summary (which includes `<all_user_messages>...</all_user_messages>` XML block)
2. Post-processes: replaces the `<all_user_messages>` placeholder with the actual most recent user messages (up to 1/3 of the trigger token budget)
3. Trims only the oldest user messages that don't fit

This ensures the model always sees the real words the user typed, not a summarized version.

### 5. ADK-Go's `NumRecentEvents` Approach

Google ADK-Go does not implement in-process summarization. Instead, it provides `NumRecentEvents` on the `GetRequest` — a simple sliding window at the storage layer. Callers that need longer context must either:
- Implement their own summarization agent
- Use IncludeContents=none for agents that don't need history

The `SkipSummarization` flag on `EventActions` lets individual events opt out of function-response summarization.

### 6. Cairn's Current State and Gap

**What exists today** (`memory/context.go`):

- `memory.Compact()` — decays old unused *memories* (not conversation messages). Applies confidence decay, auto-rejects below threshold. This is about the semantic memory store, not session history.
- `ContextBuilder.Build()` — assembles memory context within a token budget (4000 tokens default). Uses RAG search, decay scoring, budget-packing. This is correct and complete for memory injection.
- `Session.History()` — returns **all events unconditionally**. No compaction.

**The gap**: `react.go` line 72 calls `invCtx.Session.History()` and uses the result as-is. For a coding session with 100 tool rounds, each reading files and running commands, the total context can easily exceed GLM-5's context window.

## Code Examples

### Basic: Token-Gated Summarization in react.go

```go
// Insert in react.go after line 72 (messages := invCtx.Session.History())
// and before building the user message (line 75).

const compactionThreshold = 50_000 // tokens, well under GLM-5's 1M limit

messages, err = compactIfNeeded(invCtx.Context, messages, invCtx.LLM, compactionThreshold)
if err != nil {
    invCtx.Logger.Warn("compaction failed, proceeding with full history", "error", err)
    // Fail open — don't abort the conversation
}
```

### Core Compaction Function (Go, Cairn-idiomatic)

```go
// internal/agent/compact.go

package agent

import (
    "context"
    "fmt"
    "strings"
    "time"

    "github.com/avifenesh/cairn/internal/llm"
)

const bytesPerToken = 4

// CompactMessages summarizes old messages when total estimated tokens exceed threshold.
// Always preserves messages[0] (contains system/task context if any) and the last
// keepRecent messages. Summary is emitted as assistant role to maintain alternation.
func CompactMessages(ctx context.Context, messages []llm.Message, provider llm.Provider, model string, threshold int, keepRecent int) ([]llm.Message, error) {
    if keepRecent <= 0 {
        keepRecent = 6 // keep last 3 exchange pairs
    }

    total := estimateMessagesTokens(messages)
    if total <= threshold {
        return messages, nil
    }

    if len(messages) <= keepRecent+1 {
        return messages, nil // can't compact further
    }

    // Determine split point. Ensure recent section starts with user role
    // to maintain proper alternation after inserting assistant-role summary.
    startRecent := len(messages) - keepRecent
    if startRecent > 1 {
        if messages[startRecent].Role == llm.RoleAssistant {
            startRecent--
        }
    }

    oldMessages := messages[1:startRecent]
    recentMessages := messages[startRecent:]
    if len(oldMessages) == 0 {
        return messages, nil
    }

    summaryText, err := generateSummary(ctx, provider, model, oldMessages)
    if err != nil {
        return messages, fmt.Errorf("compact: summary generation failed: %w", err)
    }

    summaryMsg := llm.Message{
        Role: llm.RoleAssistant,
        Content: []llm.ContentBlock{
            llm.TextBlock{Text: "[Conversation Summary]\n" + summaryText},
        },
    }

    result := make([]llm.Message, 0, 2+len(recentMessages))
    result = append(result, messages[0])
    result = append(result, summaryMsg)
    result = append(result, recentMessages...)

    // Strip orphaned tool results (tool_result with no matching tool_use).
    result = stripOrphanedToolResults(result)

    return result, nil
}

func generateSummary(ctx context.Context, provider llm.Provider, model string, messages []llm.Message) (string, error) {
    var sb strings.Builder
    sb.WriteString("Summarize this conversation concisely, preserving:\n")
    sb.WriteString("- Files created, edited, or read (exact paths)\n")
    sb.WriteString("- Commands run and key results (pass/fail counts, errors)\n")
    sb.WriteString("- Key decisions and current approach\n")
    sb.WriteString("- What approaches were tried and whether they succeeded\n")
    sb.WriteString("- Current state: what is done, what remains\n\n")

    for _, msg := range messages {
        for _, block := range msg.Content {
            switch b := block.(type) {
            case llm.TextBlock:
                role := string(msg.Role)
                text := b.Text
                if len(text) > 600 {
                    text = text[:300] + "...[truncated]..." + text[len(text)-300:]
                }
                fmt.Fprintf(&sb, "%s: %s\n", role, text)
            case llm.ToolResultBlock:
                out := b.Content
                if len(out) > 400 {
                    out = out[:200] + "...[truncated]..." + out[len(out)-200:]
                }
                fmt.Fprintf(&sb, "[tool_result: %s] %s\n", b.ToolUseID, out)
            }
        }
    }

    req := &llm.Request{
        Model: model,
        Messages: []llm.Message{{
            Role:    llm.RoleUser,
            Content: []llm.ContentBlock{llm.TextBlock{Text: sb.String()}},
        }},
    }
    ch, err := provider.Stream(ctx, req)
    if err != nil {
        return "", err
    }
    var out strings.Builder
    for event := range ch {
        if td, ok := event.(llm.TextDelta); ok {
            out.WriteString(td.Text)
        }
        if se, ok := event.(llm.StreamError); ok {
            return "", se.Err
        }
    }
    return out.String(), nil
}

func estimateMessagesTokens(messages []llm.Message) int {
    total := 0
    for _, msg := range messages {
        for _, block := range msg.Content {
            switch b := block.(type) {
            case llm.TextBlock:
                total += (len(b.Text) + 3) / 4
            case llm.ToolResultBlock:
                total += (len(b.Content) + 3) / 4
            case llm.ToolUseBlock:
                total += (len(b.Input) + 3) / 4
            }
        }
    }
    return total
}

// stripOrphanedToolResults removes ToolResultBlocks whose ToolUseID has no
// matching ToolUseBlock. APIs reject conversations with dangling tool results.
func stripOrphanedToolResults(messages []llm.Message) []llm.Message {
    // Collect all tool_use IDs present.
    callIDs := make(map[string]bool)
    for _, msg := range messages {
        for _, block := range msg.Content {
            if tu, ok := block.(llm.ToolUseBlock); ok {
                callIDs[tu.ID] = true
            }
        }
    }

    out := make([]llm.Message, 0, len(messages))
    for _, msg := range messages {
        if msg.Role != llm.RoleTool {
            out = append(out, msg)
            continue
        }
        var kept []llm.ContentBlock
        for _, block := range msg.Content {
            if tr, ok := block.(llm.ToolResultBlock); ok {
                if callIDs[tr.ToolUseID] {
                    kept = append(kept, block)
                }
                // else: orphaned — drop silently
            } else {
                kept = append(kept, block)
            }
        }
        if len(kept) > 0 {
            msg.Content = kept
            out = append(out, msg)
        }
    }
    return out
}
```

### Tool Output Truncation (Gollem-style, insert in react.go tool execution)

```go
// After executing a tool and before appending to messages:
const maxToolOutputTokens = 16_000 // ~64KB

output = truncateToolOutput(output, maxToolOutputTokens)

// truncateToolOutput keeps head 60% + tail 40% with a dropped-tokens marker.
func truncateToolOutput(content string, maxTokens int) string {
    maxBytes := maxTokens * 4 // bytesPerToken
    if len(content) <= maxBytes {
        return content
    }
    headBytes := maxBytes * 60 / 100
    tailBytes := maxBytes - headBytes
    dropped := (len(content) - maxBytes) / 4
    head := content[:headBytes]
    tail := content[len(content)-tailBytes:]
    return fmt.Sprintf("%s\n\n... [truncated %d tokens] ...\n\n%s", head, dropped, tail)
}
```

## Common Pitfalls

| Pitfall | Why It Happens | How to Avoid |
|---------|---------------|--------------|
| Adjacent user messages after compaction | Summary emitted as user role, then next real user message follows | Always emit summary as **assistant role** |
| Orphaned tool results cause 400 errors | tool_result IDs reference tool_use blocks that were dropped | Run `stripOrphanedToolResults` after every compaction |
| Summary re-summarized on next compaction | No marker distinguishing summaries from regular messages | Tag summary messages (Eino's `Extra["contentType"]="summary"`, or a `[Conversation Summary]` prefix) |
| Token budget too tight | Compaction triggered every round, summary quality degrades | Set threshold at 50-70% of model context limit, not 95% |
| First message (task/system) dropped | Compaction logic trims from front of full slice | Always `preserve messages[0]`, start trimming from `messages[1]` |
| Compaction during active tool call chain | Mid-chain compaction drops the tool_use that started the chain | Only compact at round boundaries, never during a tool call sequence |
| Empty summary on model error | Network error or rate limit during summarization | Fail open: return original messages if summary is empty or errors |
| Tool offloading with no read_file tool | Agent can't retrieve offloaded content | Only use filesystem offloading if you've given the agent a `read_file` tool for the same backend |

## Best Practices

1. **Two-tier by default**: truncate tool outputs on arrival (immediate, cheap), summarize history lazily (only when needed). (Source: Eino reduction + summarization middleware, Gollem truncate + autocontext)

2. **Threshold at 60-70% of model limit**: leaves headroom for the current response and prevents constant re-summarization. For GLM-5 (1M context): trigger at ~600K tokens. For smaller models: trigger at 60% of their limit. (Source: Eino default 190K trigger on 200K models)

3. **Preserve user messages verbatim**: the model should always see what the user actually typed, not a paraphrase. Post-process summary to splice in raw user messages up to 1/3 of the trigger budget. (Source: Eino `PreserveUserMessages`)

4. **Async incremental summaries for planning agents**: generate a summary after each reply (Plandex pattern), store it, look up the right one when the limit is hit. Avoids paying the summarization cost on every round.

5. **Use the same model for summarization**: avoids the complexity of maintaining a second model client. Accept the small latency cost. If you have a cheaper/faster model, use it only for summarization. (Source: Gollem `SummaryModel` fallback to agent model)

6. **Strip orphaned tool results unconditionally**: even a single orphaned tool_result block causes a hard 400 from Anthropic. Run the strip pass after every compaction. (Source: Gollem `autocontext.go`)

7. **Cairn-specific**: the right hook is in `react.go` after building `messages` from `invCtx.Session.History()` and before the loop. The `ContextBuilder` and memory injection happen in `BuildSystemPrompt`, not in `messages` — don't touch the system prompt side.

## Further Reading

| Resource | Type | Why Recommended |
|----------|------|-----------------|
| `/home/ubuntu/research/go-agents/eino/adk/middlewares/summarization/summarization.go` | Framework source | Most complete production summarization middleware in the codebase. Token counting, trigger conditions, user message preservation, event emission. |
| `/home/ubuntu/research/go-agents/eino/adk/middlewares/reduction/reduction.go` | Framework source | Two-phase tool output management: immediate truncation + lazy clearing. Includes filesystem offloading with `read_file` integration. |
| `/home/ubuntu/research/go-agents/gollem/core/autocontext.go` | Framework source | Clean autocompress implementation. Detailed comments on user/assistant alternation requirement. `stripOrphanedToolResults` implementation. |
| `/home/ubuntu/research/go-agents/gollem/core/memory/strategy.go` | Framework source | Three strategies side-by-side: `SlidingWindowMemory`, `TokenBudgetMemory`, `SummaryMemory`. Clean reference implementations. |
| `/home/ubuntu/research/go-agents/gollem/core/truncate.go` | Framework source | Head/tail truncation with line-boundary snapping and UTF-8 safety. |
| `/home/ubuntu/research/go-agents/plandex/app/server/model/plan/tell_summary.go` | Production agent | Append-only incremental summary pattern with DB storage and timestamp-based lookup. |
| `/home/ubuntu/research/go-agents/eino/adk/middlewares/summarization/prompt.go` | Framework source | Reference summarization prompts. The `userSummaryInstruction` constant shows what sections to ask the model to produce. |
| `/home/ubuntu/internal/agent/react.go` | Cairn source | Current insertion point for compaction (after line 72, before line 75). |
| `/home/ubuntu/internal/agent/types.go` | Cairn source | `Session.History()` — the method that returns unfiltered history. Compaction applies here or immediately after. |

---

*This guide was synthesized from 7 Go agent framework codebases analyzed directly from source.*
*See `agent-knowledge/resources/session-compaction-sources.json` for full source metadata.*

---

## Web Research

**Added**: 2026-03-19
**Web sources**: 30 resources analyzed
**See**: `resources/session-compaction-web-sources.json` for full source metadata

This section extends the codebase-derived guide above with findings from research papers, official API documentation, and ecosystem tooling (LangChain, LlamaIndex, Zep, MemGPT/Letta).

### The Context Rot Problem

Context rot is not just about hitting the token limit — degradation begins well before it. Transformer self-attention has O(n²) complexity over token relationships, creating a finite "attention budget." As conversation grows, recall of specific details from earlier turns decreases even when those tokens are technically still present.

From Anthropic's context engineering research: the goal is "the smallest set of high-signal tokens that maximize the likelihood of the desired outcome" — not maximum retention.

**Practical implication**: set compaction triggers at 50-70% of the context window, not 95%. The Go frameworks analyzed above already follow this (Eino triggers at 190K on 200K models — 95% — which is slightly high by this standard).

### Anthropic Server-Side Compaction API (2026 Beta)

Anthropic released a server-side compaction API that eliminates client-side summarization logic for Claude users. This is directly relevant for any Cairn integration targeting Anthropic models.

**API header**: `anthropic-beta: compact-2026-01-12`
**Supported models**: Claude Opus 4.6, Claude Sonnet 4.6

**How it works:**
1. Client sends request with `context_management.edits: [{type: "compact_20260112"}]`
2. API detects when input tokens exceed the trigger threshold (default: 150,000; minimum: 50,000)
3. API runs a summarization pass and creates a `compaction` block
4. Response includes the compaction block; subsequent requests automatically drop all prior content

**Default summarization prompt (Anthropic's):**
```
You have written a partial transcript for the initial task above. Please write
a summary of the transcript. The purpose of this summary is to provide
continuity so you can continue to make progress towards solving the task in a
future context, where the raw history above may not be accessible and will be
replaced with this summary. Write down anything that would be helpful,
including the state, next steps, learnings etc. You must wrap your summary in
a <summary></summary> block.
```

**Go SDK pattern:**
```go
response, err := client.Beta.Messages.New(ctx, anthropic.BetaMessageNewParams{
    Model:     anthropic.ModelClaudeOpus4_6,
    MaxTokens: 4096,
    Messages:  messages,
    ContextManagement: anthropic.BetaContextManagementConfigParam{
        Edits: []anthropic.BetaContextManagementConfigEditUnionParam{
            {OfCompact20260112: &anthropic.BetaCompact20260112EditParam{
                Trigger: anthropic.BetaInputTokensTriggerParam{Value: 100000},
            }},
        },
    },
    Betas: []anthropic.AnthropicBeta{"compact-2026-01-12"},
})
// Critical: always append full response content (includes compaction block)
messages = append(messages, response.ToParam())
```

**Key parameters:**

| Parameter | Default | Notes |
|-----------|---------|-------|
| `trigger.value` | 150,000 tokens | Minimum: 50,000 |
| `pause_after_compaction` | false | When true: returns stop_reason="compaction", client injects verbatim messages before continuing |
| `instructions` | null | Completely replaces (not supplements) the default prompt |

**Token billing caveat**: top-level `input_tokens`/`output_tokens` do NOT include the compaction pass. Must sum `usage.iterations[]` for accurate billing tracking.

**Streaming behavior**: the `compaction` block arrives as a single complete `content_block_delta` event (not token-by-token streaming).

**Current limitation**: same model is used for summarization — no option to use a cheaper model for the summary pass.

### Chain of Density Summarization

Standard one-shot summarization produces entity-sparse output. Chain of Density (CoD, arxiv:2309.04269) progressively refines summaries by iterating over missing entities:

1. Generate an initial sparse summary
2. Identify the most important entities NOT in the summary
3. Rewrite the summary to incorporate them WITHOUT expanding length
4. Repeat 3-5 times

Each iteration produces summaries that are more abstract (less tied to source order), more fused (related ideas merged), and informationally denser. Human evaluators preferred CoD output over standard summaries in blind comparisons.

**Application to Cairn**: use CoD for summarizing long coding sessions where the summary must carry critical architectural decisions. The extra cost (3-5 summarization passes) is justified for sessions that will run for many more rounds.

Template for CoD conversation summarization:
```
Summarize this conversation. Then identify the 3 most important topics NOT
mentioned. Rewrite the summary to include them without expanding length.
Repeat 2 more times. Wrap the final result in <summary></summary>.
```

### LangChain Memory Taxonomy (for API design reference)

LangChain's memory classes represent the established vocabulary for this problem:

| Class | Strategy | Token Growth | Best For |
|-------|----------|-------------|----------|
| `ConversationBufferMemory` | Store all | Linear | Short conversations |
| `ConversationTokenBufferMemory` | Sliding window (token-based) | Bounded | Recency-focused |
| `ConversationSummaryMemory` | Progressive LLM summary | Logarithmic | Long conversations |
| `ConversationSummaryBufferMemory` | Hybrid: summary + recent verbatim | Logarithmic | Production default |

The `ConversationSummaryBufferMemory` hybrid is what Gollem's `autocontext.go` implements natively (without the LangChain abstraction layer). The Cairn implementation above follows this same pattern.

### MemGPT / Letta: OS-Inspired Hierarchical Memory

MemGPT (arxiv:2310.08560, now the Letta framework) is the canonical reference for treating the LLM context window like OS main memory:

```
Context Window (RAM):
  [Core Memory Blocks] — injected every request, agent-editable
  [Recent Messages]
  [Retrieved Archival Memories]

External Storage (Disk):
  Recall Memory — full conversation history, never truly deleted
  Archival Memory — long-term knowledge, searched explicitly
```

Key architectural insight: **context eviction does not equal data loss**. All messages are persisted to a database layer before being evicted from context. The agent uses explicit `memory_search` and `memory_insert` tool calls to move information between tiers.

Memory blocks can be shared across multiple agents simultaneously — enabling shared working memory for multi-agent systems.

**Cairn relevance**: the existing `memory/context.go` implements the archival memory retrieval (RAG search + budget packing). What's missing is the recall memory layer (full session history in DB with explicit retrieval). The `Session.History()` gap documented above is exactly this.

### Generative Agents Memory Scoring

The Generative Agents paper (arxiv:2304.03442) established the canonical scoring formula for what to retrieve from memory:

```
score = α × recency_score + β × importance_score + γ × relevance_score

where:
  recency_score    = exp(-λ × hours_since_last_access)
  importance_score = LLM.rate(memory, scale=1..10) / 10
  relevance_score  = cosine_similarity(memory_embedding, query_embedding)
```

This is the basis for Cairn's existing memory decay scoring in `memory/context.go`. The three-component formula is validated by ablation studies — all three factors contribute meaningfully.

### Zep: Temporal Knowledge Graph Approach

Zep takes a more structured approach than conversation summarization: it extracts and maintains a temporal knowledge graph of facts and entities.

- **Facts**: timestamped tuples `(entity, relationship, entity, timestamp)` — precise, queryable
- **Entity summaries**: continuously updated profiles (not append-only log)
- **Temporal queries**: "what did the user want last week?"
- **Custom extraction**: domain-specific entity/relationship schemas

Zep is architecturally distinct from and complementary to conversation summarization. Summarization preserves narrative flow; the knowledge graph enables structured fact retrieval.

**When to prefer KG over summary**: long-running personal assistants where users return weeks later and need the assistant to remember specific commitments and preferences. The summary approach would require the model to re-read a long summary block; the KG retrieves exactly the relevant facts.

### Structured Distillation for Agent Memory (11x Compression)

For software engineering agents specifically (arxiv:2603.13017), structured compression to 4 fields achieves 11x token reduction (371 → 38 tokens per exchange) with 96% of retrieval quality:

```json
{
  "exchange_core": "Decided to use SQLite WAL mode for concurrent reads",
  "specific_context": "database.go:L45, concurrent reader count=3",
  "thematic_room_assignments": ["database", "concurrency"],
  "files_touched": ["internal/db/database.go", "internal/db/migrations.go"]
}
```

**Critical finding**: this approach works well with vector search (no significant degradation) but degrades with BM25/keyword search. Cairn uses embedding-based memory retrieval — this pattern is safe to apply.

### Adaptive Focus Memory: Three-Tier Fidelity

AFM (arxiv:2511.12712) improves on binary keep/drop with three fidelity levels per message:
- **Full**: verbatim — recent turns, high-importance exchanges, safety constraints
- **Compressed**: 1-2 sentence LLM summary — older but relevant exchanges
- **Placeholder**: single marker token — old low-importance turns

**Why Placeholder matters**: unlike sliding window (complete blindness), Placeholder gives the model awareness that an exchange occurred without consuming tokens for its content. This prevents "forgetting surprises" where the model acts as if early decisions never happened.

Safety constraints should always be kept at Full fidelity regardless of age — a lesson directly applicable to Cairn's system prompts and user-defined hard rules.

### Production Token Budget Management

Practical thresholds from production systems and research:

| System | Context Window | Compaction Trigger | Keep Recent |
|--------|---------------|-------------------|-------------|
| Eino middleware | 200K | 190K (95%) | Last 1/3 of trigger budget |
| Anthropic API | 200K-1M | 150K default (75-15%) | Custom via pause_after |
| LangChain SummaryBuffer | Any | `max_token_limit` param | All messages after threshold |
| MemGPT/Letta | Model limit | Always-on (paging) | Core memory blocks always present |

For Cairn with GLM-5 (1M context): trigger at 600K-700K. This is more conservative than Eino's 95% but avoids the quality degradation that happens in the last 5-10% of the window.

**Total budget tracking** (for multi-compaction sessions):
```go
// Track cumulative tokens across compaction events
compactionCount++
estimatedTotalTokens := compactionCount * compactionThreshold
if estimatedTotalTokens >= totalBudget {
    // Signal task wrap-up rather than continuing indefinitely
    appendWrapUpInstruction(messages)
}
```

### Key Insight: What to Preserve Verbatim vs Summarize

From synthesizing all sources, the consensus on what must never be summarized:

1. **The current user message** — the immediate question/instruction
2. **The last 2-3 assistant responses** — maintains conversational coherence
3. **Safety constraints and hard rules** — must never be paraphrased away
4. **Active tool call chains** — a tool_use without its tool_result is a 400 error
5. **File paths and exact identifiers** — summarization often corrupts these

What can safely be summarized:
- Exploratory discussion and reasoning
- Intermediate tool outputs that produced a final result
- Repetitive clarification rounds
- Already-completed task steps (reduce to "completed X" statement)

---

*Web research section synthesized from 30 sources. See `resources/session-compaction-web-sources.json` for full metadata.*
