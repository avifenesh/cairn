# Learning Guide: Orchestrator Agent Pattern for Autonomous Systems

**Generated**: 2026-03-21
**Sources**: 16 resources analyzed
**Depth**: deep

## Prerequisites

- Understanding of multi-agent architectures (supervisor, router, swarm)
- Familiarity with the subagent pattern (see `subagents.md`)
- Knowledge of Cairn's architecture (ReAct loop, task engine, memory system, signal plane)

## TL;DR

- The orchestrator is a **thin management layer** that never does work itself - it only spawns, monitors, verifies, and decides
- It has **read access to everything** (memory, sessions, tasks, events, config) but **writes only through delegated workers**
- Key responsibilities: task triage, worker spawning, result verification, memory approval, session completion validation
- Anthropic's production system uses this exact pattern: "Lead Researcher" orchestrates subagents, verifies results, decides if more rounds needed
- The orchestrator is NOT another ReAct agent — it's a **decision loop** that runs on every tick and inspects system state
- For Cairn: replace the current idle tick with an orchestrator that manages all autonomous behavior

## Core Concepts

### 1. What Is the Orchestrator Pattern?

The orchestrator (also called supervisor, lead agent, or conductor) is a dedicated agent whose only job is **management, not execution**. It:

- **Observes** the full system state (events, tasks, memories, sessions, signals)
- **Decides** what needs to happen (spawn coder, run reflection, approve memory, check PR)
- **Delegates** work to specialized subagents (researcher, coder, reviewer, executor)
- **Verifies** results from subagents before accepting them
- **Escalates** to the human only for irreversible external actions

It never writes code, never searches the web, never edits files. It only manages.

### 2. Orchestrator vs Current Cairn Architecture

**Current Cairn (idle loop)**:
```
tick()
  → checkDueCrons()
  → executePendingTask()    ← generic agent handles everything
  → idleTick()             ← same agent does idle reasoning
  → runReflection()        ← separate reflection engine
```

The problem: the same ReAct agent that handles user chat also handles autonomous tasks, idle reasoning, and needs to "remember" to check PRs, verify memories, etc. It's one brain doing everything.

**Proposed orchestrator pattern**:
```
tick()
  → orchestrator.Evaluate()
      → reads: pending tasks, open PRs, proposed memories, signal events
      → reads: recent sessions, reflection history, skill suggestions
      → decides: what actions to take this tick
      → spawns: subagent(coder, "fix CI on PR #147")
      → spawns: subagent(reviewer, "review changes in session X")
      → approves: 3 fact memories that passed quality check
      → verifies: coding session Y completed (CI green + 0 unresolved)
      → skips: nothing actionable right now
```

The orchestrator is the **single decision-maker** that has global context. Workers are specialized and context-isolated.

### 3. The Thin Orchestrator Principle

From Anthropic's multi-agent research system and LangGraph's supervisor pattern:

> "The orchestrator should be as thin as possible. Its job is to route, not to reason about domain-specific problems."

| DO | DON'T |
|----|-------|
| Inspect system state | Write code |
| Spawn specialized workers | Search the web |
| Verify worker results | Edit files |
| Approve/reject memories | Handle user conversations |
| Decide if sessions are complete | Run shell commands |
| Triage incoming signals | Parse API responses |
| Escalate to human when needed | Do any domain-specific work |

The orchestrator's tools are:
- `spawnSubagent` (delegate to workers)
- `approveMemory` / `rejectMemory` (memory management)
- `completeSession` / `continueSession` (session lifecycle)
- `notify` (escalate to human)
- `read*` (read anything in the system - tasks, sessions, memories, events)

It does NOT get: `editFile`, `writeFile`, `shell`, `gitRun`, `webSearch`.

### 4. Orchestrator Decision Model

On each tick, the orchestrator runs a structured evaluation:

```
1. SCAN: What has changed since last tick?
   - New signal events (GitHub PRs, emails, HN posts)
   - Completed subagent sessions
   - New proposed memories
   - Due cron jobs
   - Pending approvals
   - Open PRs with new comments

2. TRIAGE: What needs action?
   For each item, classify:
   - DELEGATE: spawn a subagent (coding, research, review)
   - APPROVE: auto-approve (fact/preference memories, passing quality gates)
   - VERIFY: check if a running session is truly complete
   - ESCALATE: notify human (irreversible actions, policy decisions)
   - DEFER: not urgent, handle later

3. ACT: Execute highest-priority decisions
   - Spawn subagents for delegated work
   - Approve/reject memories in batch
   - Mark complete sessions that passed verification
   - Send notifications for escalated items

4. RECORD: Log what was decided and why
   - Activity entry for observability
   - Memory extraction from orchestrator reasoning
```

### 5. Anthropic's Lead Researcher Pattern

From Anthropic's production multi-agent system (their most detailed public architecture):

**How the Lead Researcher (orchestrator) works:**
- Analyzes incoming query and develops research strategy
- Saves plan to external memory (prevents context window loss)
- Spawns 3-5 subagents with clearly divided responsibilities
- Scaling rules: 1 agent for simple tasks, 10+ for complex research
- After subagents complete, evaluates if more rounds needed
- Uses a separate Citation Agent to verify every claim against sources
- Extended thinking as "controllable scratchpads" for reasoning

**Key lessons:**
1. The orchestrator saves its plan externally — doesn't rely on context window alone
2. It spawns subagents with explicit instructions per task
3. It verifies results with a specialized verifier (not itself)
4. It can decide "not done yet" and spawn more rounds

### 6. AIOS Kernel Pattern

The AIOS (AI Agent Operating System) takes the orchestrator concept to its extreme — the orchestrator becomes an OS kernel:

- **AgentScheduler**: FIFO, Round Robin, Priority, or Shortest Job First scheduling
- **LLM Manager**: Routes requests across providers, implements rate limiting, cost tracking, response caching
- **Memory Manager**: Short-term (active contexts) + long-term (persistent knowledge)
- **Context Switching**: 0.1s context switch vs 2.1s traditionally
- **Tool Manager**: Sandboxed tool access with authorization control

This maps directly to Cairn's existing modules. The orchestrator layer would sit on top of all of them.

### 7. State Machine Orchestration (LangGraph)

LangGraph models orchestrators as state machines, not chatbots:

```
                    ┌──────────────┐
                    │  SCAN STATE  │
                    └──────┬───────┘
                           │
                    ┌──────▼───────┐
              ┌─────│   TRIAGE     │─────┐
              │     └──────────────┘     │
              │            │             │
        ┌─────▼────┐ ┌────▼─────┐ ┌─────▼────┐
        │ DELEGATE │ │ APPROVE  │ │ ESCALATE │
        └─────┬────┘ └────┬─────┘ └─────┬────┘
              │            │             │
              └────────────┼─────────────┘
                           │
                    ┌──────▼───────┐
                    │   RECORD     │
                    └──────┬───────┘
                           │
                    ┌──────▼───────┐
                    │    DONE      │
                    └──────────────┘
```

This is a deterministic state machine, not a free-form ReAct loop. The orchestrator follows a fixed evaluation pipeline every tick, making it predictable and debuggable.

### 8. Verification Responsibilities

The orchestrator's most important job is **verification** — checking that autonomous work meets quality standards:

#### Memory Verification
```
For each proposed memory:
  - Is it a fact or preference? → auto-accept (already in extractor)
  - Is it a hard rule or decision? → inspect content
    - Does it contradict existing rules? → reject
    - Is it overly specific (session-scoped)? → reject
    - Is it genuinely reusable across sessions? → accept
```

#### Coding Session Verification
```
For each completed coding session:
  - Was a PR created?
    - No → session incomplete, spawn continuation
  - Is CI green?
    - No → spawn ci-fixer subagent
  - Are all review comments resolved?
    - No → spawn comment-fixer subagent
  - All pass → mark session verified, notify human for merge
```

#### Signal Triage
```
For each new signal event:
  - GitHub: new issue, PR review, CI failure → prioritize and spawn
  - Email: check if actionable → triage or archive
  - HN/Reddit: check if relevant → bookmark or skip
```

### 9. Architecture for Cairn

```go
// Orchestrator replaces the current idle tick with a structured
// decision loop that manages all autonomous behavior.
type Orchestrator struct {
    subagents  *SubagentRunner
    memories   *memory.Service
    tasks      *task.Engine
    sessions   *SessionStore
    events     *signal.EventStore
    soul       *memory.Soul
    bus        *eventbus.Bus
    provider   llm.Provider
    logger     *slog.Logger
}

// Evaluate runs one orchestrator cycle. Called on each Loop tick.
func (o *Orchestrator) Evaluate(ctx context.Context) *OrchestratorDecision {
    // 1. SCAN: gather current state
    state := o.scanState(ctx)

    // 2. TRIAGE: LLM decides what to do
    // The orchestrator prompt includes ALL state context but
    // can only call management tools (spawn, approve, verify, notify)
    decision := o.triage(ctx, state)

    // 3. ACT: execute decisions
    o.execute(ctx, decision)

    // 4. RECORD: log activity
    o.record(ctx, decision)

    return decision
}

type SystemState struct {
    PendingTasks       []*task.Task
    ProposedMemories   []*memory.Memory
    OpenPRs            []PRStatus
    ActiveSubagents    []SubagentStatus
    RecentSignals      []*signal.Event
    DueCrons           []CronJob
    PendingApprovals   []*Approval
    LastReflection     time.Time
}

type OrchestratorDecision struct {
    SpawnRequests    []SubagentSpawnRequest
    MemoryApprovals  []string   // IDs to accept
    MemoryRejections []string   // IDs to reject
    SessionVerified  []string   // session IDs confirmed complete
    Escalations      []string   // messages to send to human
    Deferred         []string   // items to handle later
}
```

### 10. Orchestrator Prompt Design

The orchestrator's system prompt is fundamentally different from a worker agent. It's not "be helpful" — it's "manage this system":

```
You are the orchestrator for Cairn, a personal agent OS. Your job is
management, not execution. You never write code, search the web, or
edit files. You only:

1. SPAWN subagents for tasks that need work
2. APPROVE/REJECT proposed memories
3. VERIFY completed sessions
4. ESCALATE to the human for irreversible actions
5. DEFER items that aren't urgent

Current system state is provided below. For each item, decide
what action to take and output a structured JSON decision.

RULES:
- Facts and preferences: auto-approve if they survived dedup
- Hard rules and decisions: inspect carefully, reject if too specific
- Coding sessions: verify CI green + 0 unresolved before marking complete
- New signals: only spawn subagents for actionable items
- Budget: never exceed daily cap, prefer cheaper models for simple tasks
- Human escalation: merge, deploy, external messages, policy changes ONLY
```

## Common Pitfalls

| Pitfall | Why It Happens | How to Avoid |
|---------|---------------|--------------|
| Orchestrator does too much itself | Temptation to "just handle it" | Restrict tool access — no edit, write, shell |
| Decision paralysis | Too many items to evaluate | Priority-based evaluation, cap at 5 actions per tick |
| Orchestrator hallucinated state | Relies on LLM memory of system state | Always pass fresh state data, never rely on history |
| Subagent results unchecked | Orchestrator trusts workers blindly | Always verify: CI green, tests pass, review clean |
| Over-spawning | Every signal triggers a subagent | Cooldown periods + priority thresholds |
| Context window exhaustion | Full system state is too large | Summarize state, only include actionable items |

## Best Practices

1. **Thin orchestrator**: It manages, never executes. Restrict its tools to read + spawn + approve + notify.
2. **Structured output**: The orchestrator returns JSON decisions, not free-form text. Parse and execute deterministically.
3. **State-driven, not memory-driven**: Pass fresh system state every tick. Don't rely on the LLM remembering previous decisions.
4. **Priority-based evaluation**: Not everything needs action every tick. Triage by urgency.
5. **Verification before completion**: No session is "done" until the orchestrator verifies quality gates.
6. **External memory for plans**: Save orchestrator decisions and plans to DB/files, not just context.
7. **Budget awareness**: The orchestrator tracks LLM spend and adjusts subagent model selection accordingly.
8. **Audit trail**: Every decision logged with reasoning for observability.

## Further Reading

| Resource | Type | Why Recommended |
|----------|------|-----------------|
| [Anthropic Multi-Agent System (ByteByteGo)](https://blog.bytebytego.com/p/how-anthropic-built-a-multi-agent) | Engineering | Production orchestrator pattern with Lead Researcher |
| [LangGraph Supervisor Pattern](https://dev.to/sreeni5018/building-multi-agent-systems-with-langgraph-supervisor-138i) | Tutorial | Code examples for supervisor-based orchestration |
| [AIOS: AI Agent Operating System](https://www.labellerr.com/blog/aios-explained/) | Architecture | Kernel-level agent scheduling and resource management |
| [Multi-Agent Orchestration Frameworks (n8n)](https://blog.n8n.io/ai-agent-orchestration-frameworks/) | Comparison | LangGraph vs CrewAI vs n8n orchestration approaches |
| [LangChain: When to Build Multi-Agent](https://blog.langchain.com/how-and-when-to-build-multi-agent-systems/) | Guide | Context engineering and read vs write complexity |
| [Blueprint Architecture for AI Agents](https://www.preprints.org/manuscript/202509.0077/v1) | Paper | Formal architecture for real-time agent coordination |

---

*Generated by /learn from 16 sources. Depth: deep.*
*See `resources/orchestrator-pattern-sources.json` for full source metadata.*
