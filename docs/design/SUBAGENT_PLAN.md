# Subagent System Plan

> Agent-as-tool delegation: the parent agent spawns isolated child agents for tasks, streams progress via SSE, and collects results. Two-level max. No grandchildren.

Implemented as subphase 4.6 (Done, PR #146). See `pieces/04-agent-core.md` for current status.

## Research Summary

22 sources analyzed (see `agent-knowledge/subagents.md`). Key findings:
- Agent-as-tool is the dominant pattern across Claude Code, LangGraph, OpenAI SDK, Vercel AI SDK, CrewAI, AutoGen
- Two-level hierarchies outperform flat and deeper (3+) architectures
- Context isolation is the primary value - prevents parent context bloat
- Anthropic uses 15x more tokens for multi-agent vs chat, but 90.2% improvement on research tasks
- 41-87% production failure rates documented - guardrails (max turns, timeouts) are essential

## Architecture

```
User (Chat UI)
  └── Parent Agent (ReAct loop, always-on)
        ├── cairn.spawnSubagent tool
        │     ├── Subagent: researcher (read-only, web search)
        │     ├── Subagent: coder (worktree-isolated, full tools)
        │     ├── Subagent: reviewer (read-only, code analysis)
        │     └── Subagent: custom (from SKILL.md definitions)
        └── Results flow back as tool results
```

### Design Decisions

1. **Agent-as-tool** - Subagents are invoked via `cairn.spawnSubagent`, a registered tool. The LLM decides when to delegate based on the tool description.
2. **Two-level max** - Subagents cannot spawn subagents. If deeper delegation is needed, chain from the parent.
3. **Task engine integration** - Each subagent runs as a Task (existing piece 5). Gets its own session, goroutine, and optional worktree.
4. **Context isolation** - Subagent gets a clean context: system prompt + task prompt + allowed tools. No parent history unless explicitly passed.
5. **Dual output** - Full execution streams to the UI via SSE. Parent model receives only a condensed summary.
6. **Permission inheritance** - Subagent inherits the parent session's permission rules but can be further restricted via tool allowlist.

## Backend: Go Types

### Subagent Definition

```go
// internal/agent/subagent.go

// SubagentDef defines a reusable subagent type.
// Loaded from skills/, config, or defined inline.
type SubagentDef struct {
    Name         string      `json:"name"`
    Description  string      `json:"description"`   // LLM uses this to decide when to delegate
    SystemPrompt string      `json:"system_prompt"`
    Tools        []string    `json:"tools"`          // allowed tool names, nil = inherit all
    DenyTools    []string    `json:"deny_tools"`     // tools to exclude
    Model        string      `json:"model"`          // "" = inherit parent model
    MaxTurns     int         `json:"max_turns"`      // 0 = default (10)
    Background   bool        `json:"background"`     // run without blocking parent
    Isolation    string      `json:"isolation"`      // "none" | "worktree"
    Mode         tool.Mode   `json:"mode"`           // override agent mode
}

// SubagentRegistry holds available subagent definitions.
type SubagentRegistry struct {
    mu   sync.RWMutex
    defs map[string]*SubagentDef
}

func (r *SubagentRegistry) Register(def *SubagentDef) error
func (r *SubagentRegistry) Get(name string) (*SubagentDef, bool)
func (r *SubagentRegistry) List() []*SubagentDef
```

### Built-in Subagent Types

```go
var BuiltinSubagents = []*SubagentDef{
    {
        Name:         "researcher",
        Description:  "Deep research and information gathering. Use for web search, documentation lookup, codebase exploration. Read-only - cannot modify files.",
        Tools:        []string{"readFile", "search", "webSearch", "zread", "glob", "grep", "memorySearch"},
        MaxTurns:     15,
        Background:   false,
    },
    {
        Name:         "coder",
        Description:  "Implement code changes in an isolated worktree. Use for features, bug fixes, refactoring. Gets its own git branch.",
        Tools:        nil, // inherit all
        DenyTools:    []string{"spawnSubagent"}, // no grandchildren
        MaxTurns:     50,
        Isolation:    "worktree",
    },
    {
        Name:         "reviewer",
        Description:  "Review code for quality, security, and correctness. Read-only analysis with structured feedback.",
        Tools:        []string{"readFile", "glob", "grep", "shell"},
        MaxTurns:     10,
        Background:   true,
    },
    {
        Name:         "executor",
        Description:  "Execute shell commands and operational tasks. Use for running tests, builds, deployments. Requires approval for destructive ops.",
        Tools:        []string{"shell", "readFile", "writeFile"},
        MaxTurns:     10,
    },
}
```

### Spawn Tool

```go
// internal/tool/builtin/subagent.go

type SpawnSubagentParams struct {
    AgentType string `json:"agent_type" desc:"Which subagent to spawn (researcher, coder, reviewer, executor, or custom)"`
    Task      string `json:"task"       desc:"Detailed task description with file paths, success criteria, and expected output format"`
    Context   string `json:"context"    desc:"Optional additional context from the parent conversation"`
}

type SpawnSubagentResult struct {
    SubagentID string `json:"subagent_id"`
    Status     string `json:"status"`     // "running" | "completed" | "failed"
    Result     string `json:"result"`     // condensed summary for parent model
    Turns      int    `json:"turns"`
    ToolCalls  int    `json:"tool_calls"`
    Duration   string `json:"duration"`
}

// Tool registration
var SpawnSubagent = tool.Define[SpawnSubagentParams](
    "spawnSubagent",
    "Spawn a subagent to handle a delegated task. The subagent runs in its own context with restricted tools and returns a result summary.",
    func(ctx *tool.Context, p SpawnSubagentParams) (string, error) {
        // 1. Look up subagent definition
        // 2. Create child session (Session.Branch)
        // 3. Submit task to task engine
        // 4. If background: return task ID immediately
        // 5. If foreground: block, stream progress via SSE, return result
    },
)
```

### Subagent Lifecycle

```go
// internal/agent/subagent_runner.go

type SubagentRunner struct {
    engine    task.Engine
    sessions  SessionStore
    registry  *SubagentRegistry
    llm       llm.Client
    tools     *tool.Registry
    bus       *eventbus.Bus
    worktrees *task.WorktreeManager
}

// Run executes a subagent to completion.
func (r *SubagentRunner) Run(ctx context.Context, parentSession *Session, def *SubagentDef, taskPrompt string) (*SubagentResult, error) {
    // 1. Create child session (branched from parent, clean history)
    childSession := r.sessions.Branch(ctx, parentSession.ID)

    // 2. Build tool registry scoped to allowed tools
    childTools := r.tools.Scope(def.Tools, def.DenyTools)
    // Remove spawnSubagent from child tools (no grandchildren)
    childTools.Remove("spawnSubagent")

    // 3. If isolation=worktree, create worktree
    var worktreeDir string
    if def.Isolation == "worktree" {
        worktreeDir, _, _ = r.worktrees.Create(childSession.ID, "main")
    }

    // 4. Build invocation context
    invCtx := &InvocationContext{
        Context:     ctx,
        SessionID:   childSession.ID,
        UserMessage: taskPrompt,
        Mode:        def.Mode,
        Session:     childSession,
        Tools:       childTools,
        LLM:         r.llm, // or override model if def.Model != ""
        Bus:         r.bus,
    }

    // 5. Run ReAct loop with max turns
    agent := &ReActAgent{
        name:      def.Name,
        mode:      def.Mode,
        maxRounds: def.MaxTurns,
        systemBuild: func(ctx *InvocationContext) string {
            return def.SystemPrompt
        },
    }

    // 6. Collect events, publish progress via event bus
    var result SubagentResult
    for event, err := range agent.Run(invCtx) {
        if err != nil {
            result.Error = err.Error()
            break
        }
        result.Turns++
        for _, part := range event.Parts {
            if _, ok := part.(ToolPart); ok {
                result.ToolCalls++
            }
        }
        // Publish progress event for SSE
        eventbus.Publish(r.bus, SubagentProgressEvent{
            SubagentID: childSession.ID,
            AgentType:  def.Name,
            Event:      event,
        })
    }

    // 7. Cleanup worktree if no changes
    if worktreeDir != "" {
        r.worktrees.Remove(childSession.ID)
    }

    // 8. Summarize result for parent (not full history)
    result.Summary = summarizeForParent(childSession)
    return &result, nil
}
```

### Event Bus Events

```go
// internal/agent/subagent_events.go

// Published when a subagent is spawned
type SubagentSpawnEvent struct {
    SubagentID string `json:"subagent_id"`
    AgentType  string `json:"agent_type"`
    Task       string `json:"task"`
    Background bool   `json:"background"`
    ParentID   string `json:"parent_session_id"`
}

// Published on each ReAct turn
type SubagentProgressEvent struct {
    SubagentID string `json:"subagent_id"`
    AgentType  string `json:"agent_type"`
    Turn       int    `json:"turn"`
    MaxTurns   int    `json:"max_turns"`
    ToolName   string `json:"tool_name"`   // last tool called
    ToolStatus string `json:"tool_status"` // running | completed | failed
    TextDelta  string `json:"text_delta"`  // streaming text
}

// Published when subagent completes or fails
type SubagentCompleteEvent struct {
    SubagentID string `json:"subagent_id"`
    AgentType  string `json:"agent_type"`
    Status     string `json:"status"` // completed | failed | canceled
    Summary    string `json:"summary"`
    Turns      int    `json:"turns"`
    ToolCalls  int    `json:"tool_calls"`
    Duration   int64  `json:"duration_ms"`
    Error      string `json:"error,omitempty"`
}
```

## REST API

```
POST   /v1/subagents/spawn          Spawn a subagent (used by tool, also available via API)
GET    /v1/subagents                 List active subagents
GET    /v1/subagents/:id             Get subagent status and result
POST   /v1/subagents/:id/cancel     Cancel a running subagent
POST   /v1/subagents/:id/message    Send a message to a running subagent (user intervention)
GET    /v1/subagents/types           List available subagent type definitions
```

### SSE Events

```
event: subagent_spawn
data: {"subagent_id":"sa-abc","agent_type":"researcher","task":"...","background":false}

event: subagent_progress
data: {"subagent_id":"sa-abc","turn":3,"max_turns":15,"tool_name":"webSearch","tool_status":"running"}

event: subagent_stream
data: {"subagent_id":"sa-abc","text_delta":"Found 5 relevant results for..."}

event: subagent_complete
data: {"subagent_id":"sa-abc","status":"completed","summary":"...","turns":7,"tool_calls":12,"duration_ms":45000}

event: subagent_error
data: {"subagent_id":"sa-abc","status":"failed","error":"max turns exceeded","turns":15}
```

## Frontend: Svelte Components

### Chat Integration

Subagent events render inline in the chat conversation, between the user message and the agent's synthesized response.

#### SubagentSpawnBubble

Shows when the parent agent decides to delegate. Appears as a compact card in the chat flow.

```
┌─────────────────────────────────────────┐
│ ◆ Spawning researcher                   │
│ Task: Research OAuth2 PKCE flow for     │
│ single-page apps...                     │
│                              [Background]│
└─────────────────────────────────────────┘
```

#### SubagentProgressCard

Live-updating card showing subagent execution. Replaces SpawnBubble once running.

```
┌─────────────────────────────────────────┐
│ ◆ researcher ──── turn 3/15 ── 12s     │
│ ├ webSearch: "OAuth2 PKCE SPA"    [OK]  │
│ ├ zread: rfc7636 section 4       [OK]  │
│ └ webSearch: "PKCE vs implicit"   [...]  │
│                                          │
│ Found 5 relevant results covering...     │
│                          [Cancel] [▼]   │
└─────────────────────────────────────────┘
```

- Turn counter and elapsed time update in real-time via SSE
- Tool calls show name + status (spinner for running, checkmark for OK, X for failed)
- Text streaming appears below tool calls
- [Cancel] stops the subagent
- [▼] collapses to a single line

#### SubagentResultCollapse

After completion, collapses into a summary with expandable detail.

```
┌─────────────────────────────────────────┐
│ ✓ researcher ── 7 turns, 12 tools, 45s │
│ OAuth2 PKCE uses code_verifier +        │
│ code_challenge to prevent...            │
│                              [Expand ▸] │
└─────────────────────────────────────────┘
```

Expand shows the full subagent conversation history (all tool calls, text output).

#### ChatModeDropdown

Cursor-style mode switcher in the chat input area. Controls how the agent handles messages.

```
┌──────────────┐
│ ▾ Auto       │  ← LLM decides when to delegate
│   Direct     │  ← Never delegate, handle everything inline
│   Delegate   │  ← Always spawn a subagent for tasks
│   Research   │  ← Auto-spawn researcher for questions
└──────────────┘
```

Stored in `appStore` as `chatMode: "auto" | "direct" | "delegate" | "research"`.

### Store: subagent.svelte.ts

```typescript
// frontend/src/lib/stores/subagent.svelte.ts

interface SubagentState {
    id: string;
    type: string;
    task: string;
    status: 'spawning' | 'running' | 'completed' | 'failed' | 'canceled';
    background: boolean;
    turn: number;
    maxTurns: number;
    toolCalls: ToolCallEntry[];
    textStream: string;
    result: string | null;
    error: string | null;
    startedAt: number;
    duration: number | null;
}

interface ToolCallEntry {
    name: string;
    status: 'running' | 'completed' | 'failed';
    input?: string;
}

// Reactive state
let subagents = $state<Map<string, SubagentState>>(new Map());

// SSE handlers
function handleSubagentSpawn(data: SubagentSpawnEvent) { ... }
function handleSubagentProgress(data: SubagentProgressEvent) { ... }
function handleSubagentStream(data: SubagentStreamEvent) { ... }
function handleSubagentComplete(data: SubagentCompleteEvent) { ... }

// Actions
async function cancelSubagent(id: string) { ... }
async function messageSubagent(id: string, text: string) { ... }
```

## Database Schema

```sql
-- Migration: add subagent tracking
CREATE TABLE subagent_runs (
    id           TEXT PRIMARY KEY,
    parent_id    TEXT NOT NULL REFERENCES sessions(id),
    session_id   TEXT NOT NULL REFERENCES sessions(id),  -- child session
    agent_type   TEXT NOT NULL,
    task         TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'running',  -- running, completed, failed, canceled
    background   INTEGER NOT NULL DEFAULT 0,
    isolation    TEXT NOT NULL DEFAULT 'none',      -- none, worktree
    turns        INTEGER NOT NULL DEFAULT 0,
    tool_calls   INTEGER NOT NULL DEFAULT 0,
    result       TEXT,
    error        TEXT,
    started_at   INTEGER NOT NULL,
    completed_at INTEGER,
    duration_ms  INTEGER,
    cost_usd     REAL NOT NULL DEFAULT 0
);

CREATE INDEX idx_subagent_parent ON subagent_runs(parent_id);
CREATE INDEX idx_subagent_status ON subagent_runs(status);
```

## Implementation Tasks

| # | Task | Package | Depends On | Est. |
|---|------|---------|------------|------|
| 1 | SubagentDef + SubagentRegistry types | `internal/agent/` | - | S |
| 2 | Built-in subagent definitions | `internal/agent/` | 1 | S |
| 3 | SubagentRunner (lifecycle, context isolation) | `internal/agent/` | 1, existing ReAct loop | M |
| 4 | spawnSubagent tool (sync + background modes) | `internal/tool/builtin/` | 3 | M |
| 5 | SubagentRunner summary generation (dual output) | `internal/agent/` | 3 | S |
| 6 | Event bus events (spawn, progress, complete) | `internal/agent/` | 3 | S |
| 7 | SSE event broadcasting | `internal/server/` | 6 | S |
| 8 | REST routes (/v1/subagents/*) | `internal/server/` | 3, 7 | M |
| 9 | DB migration (subagent_runs table) | `internal/db/` | - | S |
| 10 | Subagent persistence (start, heartbeat, complete) | `internal/agent/` | 3, 9 | M |
| 11 | Worktree integration for coder subagent | `internal/agent/` | 3, existing WorktreeManager | S |
| 12 | Tool scoping (allowlist + denylist) | `internal/tool/` | - | S |
| 13 | Cancel + user message to running subagent | `internal/agent/` | 3 | S |
| 14 | Subagent store (subagent.svelte.ts) | `frontend/` | 7 | M |
| 15 | SubagentSpawnBubble component | `frontend/` | 14 | S |
| 16 | SubagentProgressCard component | `frontend/` | 14 | M |
| 17 | SubagentResultCollapse component | `frontend/` | 14 | S |
| 18 | ChatModeDropdown component | `frontend/` | 14 | S |
| 19 | Chat panel integration (render subagent cards inline) | `frontend/` | 15, 16, 17 | M |
| 20 | Go tests (SubagentRunner, tool, registry) | `internal/agent/` | 3, 4, 12 | M |
| 21 | Frontend tests (store, components) | `frontend/` | 14-19 | M |
| 22 | Custom subagent loading from SKILL.md | `internal/skill/` | 1, 8 | S |
| 23 | Subagent activity in /ops observability tab | `frontend/` | 14, 8 | S |

S = small (< half day), M = medium (half day to full day)

## Dependency Graph

```
                    ┌─── 1 (types) ───┐
                    │                  │
              ┌── 2 (builtins)   12 (tool scope)
              │                        │
              └──── 3 (runner) ────────┘
                    │
         ┌────┬────┼────┬────┬────┐
         │    │    │    │    │    │
         4    5    6    9   11   13
        tool  sum  bus  db  wt  cancel
              │    │    │
              │    7    10
              │   sse  persist
              │    │
              │    8 (routes)
              │    │
              └────┼──── 22 (skill loading)
                   │
            ┌──────┼──────┐
           14     23      │
          store   ops     │
            │             │
    ┌───┬───┼───┐         │
   15  16  17  18         │
   spawn prog result mode │
            │             │
           19 ────────────┘
          chat integration
            │
        ┌───┼───┐
       20      21
      go tests  fe tests
```

## Guardrails

- **Max turns**: Default 10, configurable per subagent type. Hard cap at 100.
- **Timeout**: Subagents auto-fail after 10 minutes (configurable). Heartbeat via task engine lease.
- **No grandchildren**: `spawnSubagent` is always removed from child tool registries.
- **Budget isolation**: Each subagent's token spend tracked separately in `subagent_runs.cost_usd`. Parent sees cumulative cost.
- **Permission inheritance**: Child inherits parent's permission rules. `tools`/`deny_tools` further restrict. Child cannot escalate beyond parent.
- **Worktree cleanup**: If coder subagent fails or is canceled, worktree is removed. If it has uncommitted changes, warn but still remove (changes are in the child session history).
- **Background pre-approval**: Background subagents auto-deny any permission prompts. All tools must be pre-approved via the allowlist at spawn time.

## Out of Scope (for now)

- Sub-sub-agents (grandchildren) - enforce two-level max
- A2A protocol integration - separate piece if needed
- Persistent subagent memory across invocations - each run is stateless
- Visual workflow builder for subagent chains - text-based for now
- Model override per subagent (all use parent model initially)
