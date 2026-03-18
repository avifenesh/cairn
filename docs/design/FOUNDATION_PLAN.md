# Pre-Phase 6: Foundation Hardening

> These two pieces are prerequisites for Phase 6-8. MCP, A2A, channels, and intelligence all depend on a proper context builder and plugin system. Built from patterns proven in ADK-Go (7.2k stars), Eino (10k stars), Gollem (typed agents), and mcp-go (8.4k stars).

## Current Gaps

| Piece | Spec | Current State | Risk if Not Fixed |
|-------|------|---------------|-------------------|
| 6.6 Context Builder | Token-budgeted 5-stage pipeline | Basic string concat in modes.go | MCP clients overflow context, hard rules silently dropped, no diversity in memory injection |
| 8.5-8.8 Plugin Hooks | ADK-Go lifecycle hooks at agent/model/tool levels | Nothing | Can't intercept tool calls for logging/budget/security, no extension point for MCP/A2A integration |

---

## PR A: Context Builder (`internal/memory/context.go`)

**Pattern source**: Eino's independent section builders + Gollem's cached dynamic prompts + our spec's 5-stage pipeline.

**Principle**: Each section builder is independent, fails gracefully (returns empty string), and the assembler packs them in priority order within a token budget.

### Architecture

```
┌─────────────────────────────────────────────────┐
│                Context Builder                   │
│                                                  │
│  Budget: 4000 tokens (configurable)              │
│  Hard Rule Reserve: 500 tokens (guaranteed)      │
│                                                  │
│  Stage 1: Hard Rules (always, reserved budget)   │
│  Stage 2: RAG Memories (remaining budget)        │
│  Stage 3: Journal Digest (last 48h)              │
│  Stage 4: Soul Identity                          │
│  Stage 5: Skills (always + on-demand)            │
│                                                  │
│  Output: assembled system prompt section          │
└─────────────────────────────────────────────────┘
```

### Subphases

| # | What | Reference |
|---|------|-----------|
| A.1 | Token estimation (`estimateTokens(text) int` — chars/4 heuristic) | Standard practice |
| A.2 | Adversarial sanitization (`sanitizeForPrompt(content) string` — strip injection tags, collapse newlines, truncate) | Security best practice |
| A.3 | Section builders — each returns `(section string, tokenCost int, err error)`, each has its own try/catch equivalent | Eino: independent section builders with graceful failure |
| A.4 | Hard rule packing — fetch all `hard_rule` category memories, pack first with Infinity score, guaranteed within reserved budget | Spec: hard rules are MANDATORY, never silently dropped |
| A.5 | RAG memory packing — search by user query, apply decay scoring (half-life) + staleness penalty, MMR re-rank for diversity, pack in score order within remaining budget | Existing search.go MMR + spec pipeline |
| A.6 | Journal digest — fetch last 48h entries from JournalStore, format as compact section | Spec: episodic memory in context |
| A.7 | Soul identity — include Soul.Content() as procedural memory section | Spec: SOUL.md always in context |
| A.8 | Budget assembler — `Build(ctx, query, mode) ContextResult` — runs all stages in priority order, respects budget, returns assembled string + stats | Gollem: cached per-turn, Eino: pipeline |
| A.9 | Memory usage tracking — mark injected memories as used (access_count++, last_accessed_at) | Spec: decay depends on usage |
| A.10 | Format with boundaries — `<memory_context>`, `<memory>` tags with id/category/scope attributes | Spec: per-memory boundaries |
| A.11 | Wire into modes.go — replace current basic injection with ContextBuilder.Build() | Integration |
| A.12 | Tests — budget overflow, hard rule guarantee, sanitization, empty states, MMR diversity | Coverage |

### Types

```go
type ContextBuilder struct {
    memories    *memory.Service
    journal     *agent.JournalStore
    soul        *memory.Soul
    skills      *skill.Service
    config      ContextConfig
}

type ContextConfig struct {
    TokenBudget      int     // Default: 4000
    HardRuleReserve  int     // Default: 500
    DecayHalfLife    float64 // Days, default: 30
    StaleThreshold   float64 // Days, default: 14
    MMRLambda        float64 // Default: 0.7
    MaxMemoryLength  int     // Per-memory cap, default: 2000 chars
}

type ContextResult struct {
    Text              string   // Assembled context string
    InjectedMemoryIDs []string // For usage tracking
    TokenEstimate     int
    Stats             ContextStats
}

type ContextStats struct {
    HardRulesIncluded    int
    MemoriesInjected     int
    JournalEntriesUsed   int
    BudgetUsed           int
    BudgetTotal          int
}
```

### Config

```bash
MEMORY_CONTEXT_BUDGET=4000      # Total token budget for memory context
MEMORY_HARD_RULE_RESERVE=500    # Reserved tokens for hard rules
MEMORY_DECAY_HALF_LIFE=30       # Days — memory relevance half-life
MEMORY_STALE_THRESHOLD=14       # Days — penalty for unused memories
MEMORY_MMR_LAMBDA=0.7           # MMR diversity (0=diverse, 1=relevant)
```

---

## PR B: Plugin Hook System (`internal/plugin/`)

**Pattern source**: ADK-Go's 10 lifecycle hooks + Eino's context-returning handlers + mcp-go's middleware chain.

**Principle**: Plugins are ordered interceptors at agent and tool levels. Each hook receives context and returns updated context (Eino pattern). First non-nil error or override stops the chain (ADK-Go pattern). Plugins execute before agent-level callbacks.

### Architecture

```
┌──────────────────────────────────────────────────┐
│                   Plugin Manager                  │
│                                                   │
│  Registered plugins (ordered):                    │
│    [logging] → [budget] → [memory-tracking] → ... │
│                                                   │
│  Hook points:                                     │
│                                                   │
│  Agent level:                                     │
│    BeforeAgentRun(ctx, invocation) → ctx           │
│    AfterAgentRun(ctx, invocation, result) → ctx    │
│    OnAgentError(ctx, invocation, err) → ctx        │
│                                                   │
│  Tool level:                                      │
│    BeforeToolCall(ctx, toolName, input) → ctx      │
│    AfterToolCall(ctx, toolName, result) → ctx      │
│    OnToolError(ctx, toolName, err) → ctx           │
│                                                   │
│  LLM level:                                       │
│    BeforeLLMCall(ctx, request) → ctx               │
│    AfterLLMCall(ctx, request, response) → ctx      │
│    OnLLMError(ctx, request, err) → ctx             │
│                                                   │
│  Stream level:                                    │
│    OnStreamStart(ctx, sessionID) → ctx             │
│    OnStreamEnd(ctx, sessionID, stats) → ctx        │
│                                                   │
└──────────────────────────────────────────────────┘
```

### Subphases

| # | What | Reference |
|---|------|-----------|
| B.1 | Plugin interface — each hook is an optional method (nil = skip). Plugins implement only the hooks they care about. | ADK-Go: 10 optional callbacks |
| B.2 | Hook context propagation — each hook receives `context.Context` and returns `context.Context`. State flows through the chain. | Eino: context-returning handlers |
| B.3 | Plugin manager — `Register(plugin)`, ordered execution, early exit on error/override | ADK-Go: plugins first, then agent callbacks, first non-nil wins |
| B.4 | Tool middleware — `ToolMiddleware func(ToolHandlerFunc) ToolHandlerFunc` for wrapping tool execution | mcp-go: reverse-order middleware chain |
| B.5 | Built-in: LoggingPlugin — logs agent/tool/LLM lifecycle events via slog | ADK-Go: loggingplugin |
| B.6 | Built-in: BudgetPlugin — checks spend before LLM calls, aborts if budget exceeded | Gollem: budget tracker integration |
| B.7 | Built-in: MemoryTrackingPlugin — tracks which memories were used, updates access_count after agent run | Spec: memory decay depends on usage |
| B.8 | Wire into ReAct loop — call BeforeAgentRun/AfterAgentRun, BeforeLLMCall/AfterLLMCall, BeforeToolCall/AfterToolCall at appropriate points in react.go | Integration |
| B.9 | Wire into tool registry — apply tool middleware chain in Execute() | Integration |
| B.10 | Tests — plugin ordering, early exit, context propagation, middleware chain, built-in plugins | Coverage |

### Types

```go
// Plugin is the extension interface. Implement only the hooks you need.
// Nil methods are skipped.
type Plugin interface {
    Name() string
}

// Agent-level hooks
type AgentHooks interface {
    BeforeAgentRun(ctx context.Context, inv *Invocation) (context.Context, error)
    AfterAgentRun(ctx context.Context, inv *Invocation, result *Result) context.Context
    OnAgentError(ctx context.Context, inv *Invocation, err error) context.Context
}

// Tool-level hooks
type ToolHooks interface {
    BeforeToolCall(ctx context.Context, toolName string, input json.RawMessage) (context.Context, error)
    AfterToolCall(ctx context.Context, toolName string, result *tool.Result) context.Context
    OnToolError(ctx context.Context, toolName string, err error) context.Context
}

// LLM-level hooks
type LLMHooks interface {
    BeforeLLMCall(ctx context.Context, req *llm.Request) (context.Context, error)
    AfterLLMCall(ctx context.Context, req *llm.Request, tokenUsage TokenUsage) context.Context
    OnLLMError(ctx context.Context, req *llm.Request, err error) context.Context
}

// Manager runs hooks in registration order
type Manager struct {
    plugins []Plugin
}

func (m *Manager) Register(p Plugin)
func (m *Manager) RunBeforeAgentRun(ctx context.Context, inv *Invocation) (context.Context, error)
func (m *Manager) RunAfterAgentRun(ctx context.Context, inv *Invocation, result *Result) context.Context
// ... etc for each hook point

// Invocation carries per-run metadata
type Invocation struct {
    SessionID   string
    UserMessage string
    Mode        tool.Mode
    Model       string
}

// Result carries agent run outcome
type Result struct {
    Text       string
    ToolCalls  int
    Rounds     int
    DurationMs int64
}

// TokenUsage from LLM response
type TokenUsage struct {
    InputTokens  int
    OutputTokens int
    Model        string
}
```

### Integration Points (where hooks fire in react.go)

```
BeforeAgentRun
  │
  ├── for each round:
  │     │
  │     ├── BeforeLLMCall
  │     ├── [LLM stream]
  │     ├── AfterLLMCall / OnLLMError
  │     │
  │     └── for each tool call:
  │           ├── BeforeToolCall
  │           ├── [tool.Execute]
  │           └── AfterToolCall / OnToolError
  │
  └── AfterAgentRun / OnAgentError
```

### Config

```bash
# No config needed — plugins are registered programmatically.
# Built-in plugins enabled by default (logging, budget, memory-tracking).
# Custom plugins via the Plugin interface.
```

---

## Dependency Graph

```
PR A (Context Builder) ← independent
PR B (Plugin Hooks)    ← independent
         │
         ▼
Phase 6a (MCP Server)  ← needs both: context for tool descriptions,
                          hooks for tool execution interception
Phase 6b (MCP Client)  ← needs hooks: external tool calls go through pipeline
Phase 6c (A2A Server)  ← needs both: context for agent card, hooks for task lifecycle
```

**PR A and PR B are independent** — can be built in parallel. Both must be done before Phase 6.

## Quality Bar

These are core infrastructure. The standard is:
- Every public function has a test
- Edge cases: empty state, budget overflow, nil dependencies, concurrent access
- No panics — graceful degradation on any failure
- Follow the research patterns exactly — ADK-Go for hooks, Eino for context propagation, Gollem for types
- Review comments: fix every single one, no exceptions
