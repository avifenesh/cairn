# Learning Guide: Automation Rules Engines for Personal Agent OS

**Generated**: 2026-03-22
**Sources**: 42 resources analyzed
**Depth**: deep

## Prerequisites

- Familiarity with event-driven architectures and pub/sub patterns
- Understanding of Go generics (used in Cairn's event bus: `Subscribe[E](bus, handler)`)
- Basic knowledge of SQLite and SQL triggers
- Understanding of expression evaluation concepts (boolean logic, field access)

## TL;DR

- A rules engine decouples "when X happens, do Y" logic from the orchestrator LLM, executing deterministic reactions instantly via the event bus.
- The canonical model is **Trigger + Condition + Action** (TCA) - used by Home Assistant, IFTTT, Zapier, and GitHub Actions with minor variations.
- For Cairn's Go stack, **expr-lang/expr** is the best expression evaluator: safe, fast (bytecode VM), type-checked at compile time, used by Google, Uber, and Kubernetes.
- Rules should be stored in SQLite with a normalized schema: `rules`, `rule_triggers`, `rule_conditions`, `rule_actions`, `rule_executions` tables.
- The matcher must be O(rules * conditions) per event - use compiled expression caching and trigger-type indexes to keep latency under 1ms for hundreds of rules.

## Core Concepts

### 1. The Trigger-Condition-Action (TCA) Model

Every automation system converges on the same fundamental pattern:

**Trigger**: What event starts the rule? An event type match, a cron schedule, a webhook, or manual invocation.

**Condition**: Should the rule actually fire? Field comparisons, expression evaluation, threshold checks, time windows.

**Action**: What happens when the rule fires? Run a skill, submit a task, send a notification, call a tool, spawn a subagent.

Home Assistant calls these "triggers, conditions, actions." GitHub Actions uses "on, if, steps." IFTTT simplifies to "if this, then that." Zapier calls them "triggers, filters, actions." The abstraction is universal.

**Key insight from Home Assistant**: Triggers observe events that *happened*; conditions check *current state*. A trigger fires when a GitHub PR is opened. A condition checks whether the PR is in a watched repo. This distinction prevents race conditions and simplifies rule design.

### 2. Trigger Types

Drawn from all surveyed systems, triggers fall into four categories:

**Event Match** (most common): Fire when an event matching a type and optional field filter arrives on the bus.
- Home Assistant: state triggers, numeric state triggers, event triggers, MQTT triggers
- GitHub Actions: `on: push`, `on: issues`, `on: pull_request` with activity type filters
- IFTTT: polling triggers (hourly check) vs realtime/webhook triggers (instant)
- Inngest: event name match with CEL filter expressions

**Schedule/Cron**: Fire on a time schedule.
- Home Assistant: time triggers, time pattern triggers, sun triggers
- GitHub Actions: `schedule:` with cron syntax
- Inngest: cron triggers with timezone support (`TZ=America/New_York 0 9 * * 1-5`)
- Windmill: schedule triggers with state tracking
- Go implementation: `robfig/cron` library with `cron.WithSeconds()`, job wrapping, timezone via `CRON_TZ`

**Webhook**: Fire when an external HTTP request arrives.
- Home Assistant: webhook triggers with allowed methods and local-only option
- Windmill: webhook, email, MQTT, Kafka, NATS, SQS, Postgres, WebSocket triggers
- Cairn already has: HMAC-verified webhook ingestion

**Manual/API**: Fire on explicit human or API request.
- GitHub Actions: `workflow_dispatch` with input parameters
- n8n: manual trigger node
- Useful for testing rules and one-off execution

### 3. Condition Evaluation

Conditions gate whether a triggered rule proceeds to actions. Systems use three approaches:

**Field Matching** (simplest): Direct field equality/comparison.
```yaml
# Home Assistant style
conditions:
  - condition: state
    entity_id: sensor.pr_repo
    state: "cairn"
```

**Expression Languages** (most flexible): Evaluate arbitrary boolean expressions against event data.
```
# expr-lang/expr style (recommended for Cairn)
event.Source == "github" && event.Type == "pr_opened" && event.Priority >= "medium"
```

**Template Evaluation**: Jinja2 (Home Assistant), `${{ }}` (GitHub Actions), JavaScript (n8n).
```yaml
# GitHub Actions
if: ${{ github.event.pull_request.merged == true && contains(github.event.pull_request.labels.*.name, 'auto-deploy') }}
```

**For Cairn**: expr-lang/expr is the clear winner. It is memory-safe, side-effect-free, always-terminating, compiles to bytecode, and integrates natively with Go structs. Compiled programs are thread-safe and reusable. Alternative: CEL (cel-go) is also excellent but heavier; better suited if you need cross-language compatibility or protobuf-native typing.

### 4. Expression Evaluator Comparison (Go)

| Library | Syntax | Safety | Performance | Type Safety | Status |
|---------|--------|--------|-------------|-------------|--------|
| **expr-lang/expr** | C-like, intuitive | Memory-safe, no side effects, always terminates | Bytecode VM, optimizing compiler | Compile-time checking | Active, 2.9k stars, used by Google/Uber |
| **cel-go** | C/Java-like | Linear time, no mutation, not Turing-complete | Good (protobuf-native) | Strong, gradual typing | Active, Google-maintained |
| **gval** | Go-like | Standard | ~3.5-350ns/op depending on complexity | Runtime | Active |
| **go-bexpr** | Simple `field == value` | Struct tag control | Good for simple filters | Struct-tag based | HashiCorp, limited operators |
| **nikunjy/rules** | ANTLR-based, SCIM-like | Standard | Depends on ANTLR parser | Runtime | Community |
| **grule** | Drools-like GRL | Standard | ~0.009ms/100 rules, ~0.568ms/1000 rules | Runtime | Active, Rete-based |
| **govaluate** | C-like | Standard | ~1.4us parse, ~30ns eval | Runtime | **Archived** |
| **goja** | Full JavaScript (ES5.1+) | Sandboxed but powerful | Slower than expr | None (JS dynamic) | Active, not goroutine-safe |

**Recommendation**: Use **expr-lang/expr** for rule conditions. It has the best safety guarantees, best Go integration, compile-time type checking, and the strongest adoption. Reserve **goja** only if users need full JavaScript scripting (unlikely for rules).

### 5. Action Types

Actions are what rules execute. For Cairn, these map directly to existing capabilities:

| Action Type | Implementation | Example |
|-------------|---------------|---------|
| **Run Skill** | `skill.Execute(ctx, name, params)` | Run `morning-digest` skill |
| **Submit Task** | `task.Submit(ctx, task)` | Create a task for the agent |
| **Notify** | `channel.Send(ctx, channel, message)` | Send Telegram/Discord alert |
| **Call Tool** | `tool.Execute(ctx, name, params)` | Call `git_create_branch` tool |
| **Spawn Subagent** | `agent.SpawnSubagent(ctx, type, prompt)` | Spawn researcher subagent |
| **Fire Event** | `eventbus.Publish(ctx, event)` | Emit event for other rules |
| **HTTP Request** | `http.Do(ctx, request)` | Call external webhook |
| **Update Memory** | `memory.Store(ctx, memory)` | Add/update agent memory |
| **Set Variable** | `rule.SetVar(ctx, key, value)` | Store state for later conditions |

**Chaining**: Actions can fire events that trigger other rules, enabling composition. Guard against infinite loops with a max chain depth (e.g., 5) and cycle detection.

### 6. Multi-Step Workflows vs Simple Rules

The surveyed systems split into two categories:

**Simple Rules** (IFTTT, Zapier basic, Home Assistant):
- Single trigger, conditions, single action (or action list)
- Stateless: no memory between executions
- Fast to create, easy to understand
- Sufficient for 80% of automation needs

**Workflow Engines** (n8n, Temporal, Inngest, Windmill):
- Multi-step with branching, loops, delays, approvals
- Stateful: maintain execution context across steps
- Support parallel paths, error handlers, retries
- Necessary for complex orchestration

**For Cairn**: Start with simple TCA rules. The orchestrator already handles complex multi-step workflows. Rules should handle the reactive, deterministic automations that do not need LLM reasoning. If a rule needs to "think," it should submit a task to the orchestrator instead.

## Architecture Patterns

### Event-Driven Architecture Fundamentals

Martin Fowler identifies four distinct event-driven patterns, often conflated:

1. **Event Notification**: Source emits event, receivers react independently. Low coupling. This is what Cairn's event bus does.

2. **Event-Carried State Transfer**: Events include full data, receivers maintain local copies. Cairn's signal events already carry full data (PR details, email content).

3. **Event Sourcing**: All state changes are events; current state is derived by replaying. Useful for audit trails and rule execution history.

4. **CQRS**: Separate read and write models. Relevant if rule execution needs high-throughput writes but rare reads.

For the rules engine, **Event Notification** is the primary pattern. The event bus publishes typed events; the rules engine subscribes and evaluates matches.

### Saga Pattern for Multi-Action Rules

When a rule triggers multiple actions that can partially fail:

**Choreography**: Each action publishes events; other actions react. No central coordinator. Simpler but harder to debug.

**Orchestration** (recommended for Cairn): A rule executor coordinates all actions sequentially or in parallel, tracks success/failure, and runs compensating actions on failure.

```
Rule fires → Executor starts
  → Action 1: Create branch (success)
  → Action 2: Assign reviewer (success)
  → Action 3: Post to Slack (failure)
  → Compensating: Log failure, retry later (don't undo branch/reviewer)
```

For most rules, simple sequential execution with error logging is sufficient. Full saga compensation is only needed for rules with irreversible actions.

### Circuit Breaker for External Actions

When rule actions call external services (webhooks, APIs), use the circuit breaker pattern:

**States**: Closed (normal) -> Open (failing, block calls) -> Half-Open (test recovery)

**For Cairn**: Track failure counts per action destination. After N failures in M minutes, open the circuit. Retry with exponential backoff. Log to rule execution history.

```go
type CircuitBreaker struct {
    State         CircuitState
    FailureCount  int
    FailureThreshold int
    ResetTimeout  time.Duration
    LastFailure   time.Time
}
```

### Temporal/Inngest Durable Execution Model

For long-running rule chains or rules that need to wait for future events:

- **Step-level retries**: Each action is a retriable checkpoint
- **Sleep/delay**: Rule waits for a duration before next action
- **Wait for event**: Rule pauses until a specific event arrives (e.g., "wait for PR approval after creating PR")
- **Deterministic replay**: On failure, replay from event history to recover state

Cairn does not need a full Temporal deployment. The key insight is to persist rule execution state in SQLite and use the event bus for wake-up signals.

## SQLite Schema Design

### Core Tables

```sql
-- Rules definition
CREATE TABLE rules (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    name        TEXT NOT NULL,
    description TEXT DEFAULT '',
    enabled     INTEGER NOT NULL DEFAULT 1,
    priority    INTEGER NOT NULL DEFAULT 0,  -- higher = evaluated first
    max_chain   INTEGER NOT NULL DEFAULT 5,  -- max event chain depth
    cooldown_ms INTEGER DEFAULT NULL,        -- min ms between firings (NULL = no cooldown)
    mode        TEXT NOT NULL DEFAULT 'single', -- single|parallel|queued
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now')),
    created_by  TEXT DEFAULT 'user',          -- user|system|llm
    tags        TEXT DEFAULT '[]'             -- JSON array of tags
);

-- Trigger definitions (what starts a rule)
CREATE TABLE rule_triggers (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    rule_id     TEXT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
    type        TEXT NOT NULL,  -- event|cron|webhook|manual
    -- For event triggers:
    event_type  TEXT,           -- e.g., "signal.github", "signal.gmail"
    event_filter TEXT,          -- expr expression: event.Type == "pr_opened"
    -- For cron triggers:
    cron_expr   TEXT,           -- "0 9 * * 1-5"
    cron_tz     TEXT DEFAULT 'UTC',
    -- For webhook triggers:
    webhook_path TEXT,          -- "/hooks/my-rule"
    webhook_secret TEXT,        -- HMAC secret
    enabled     INTEGER NOT NULL DEFAULT 1,
    position    INTEGER NOT NULL DEFAULT 0  -- ordering within rule
);

-- Condition definitions (gate: should rule fire?)
CREATE TABLE rule_conditions (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    rule_id     TEXT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
    expression  TEXT NOT NULL,   -- expr expression evaluated against event + state
    logic_group TEXT DEFAULT 'AND', -- AND|OR group name
    position    INTEGER NOT NULL DEFAULT 0
);

-- Action definitions (what to do)
CREATE TABLE rule_actions (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    rule_id     TEXT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
    type        TEXT NOT NULL,  -- skill|task|notify|tool|subagent|event|http|set_var
    config      TEXT NOT NULL DEFAULT '{}', -- JSON: skill name, tool name, template, etc.
    position    INTEGER NOT NULL DEFAULT 0, -- execution order
    on_error    TEXT NOT NULL DEFAULT 'continue', -- continue|stop|retry
    max_retries INTEGER NOT NULL DEFAULT 0,
    retry_delay_ms INTEGER DEFAULT 1000,
    timeout_ms  INTEGER DEFAULT 30000
);

-- Execution log (audit trail)
CREATE TABLE rule_executions (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(8)))),
    rule_id     TEXT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
    trigger_id  TEXT REFERENCES rule_triggers(id),
    started_at  TEXT NOT NULL DEFAULT (datetime('now')),
    finished_at TEXT,
    status      TEXT NOT NULL DEFAULT 'running', -- running|success|partial|failed|skipped
    trigger_data TEXT DEFAULT '{}',  -- JSON: the event/webhook that triggered
    results     TEXT DEFAULT '[]',   -- JSON array: per-action results
    error       TEXT,
    chain_depth INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER
);

-- Rule variables (persistent state across executions)
CREATE TABLE rule_variables (
    rule_id TEXT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
    key     TEXT NOT NULL,
    value   TEXT NOT NULL,      -- JSON-encoded value
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (rule_id, key)
);

-- Indexes for performance
CREATE INDEX idx_rules_enabled ON rules(enabled, priority DESC);
CREATE INDEX idx_triggers_type ON rule_triggers(type, event_type) WHERE enabled = 1;
CREATE INDEX idx_triggers_rule ON rule_triggers(rule_id);
CREATE INDEX idx_conditions_rule ON rule_conditions(rule_id);
CREATE INDEX idx_actions_rule ON rule_actions(rule_id, position);
CREATE INDEX idx_executions_rule ON rule_executions(rule_id, started_at DESC);
CREATE INDEX idx_executions_status ON rule_executions(status) WHERE status = 'running';
```

### Query Patterns

```sql
-- Find rules matching an event type (hot path - must be fast)
SELECT r.*, rt.id as trigger_id, rt.event_filter
FROM rules r
JOIN rule_triggers rt ON rt.rule_id = r.id
WHERE r.enabled = 1
  AND rt.enabled = 1
  AND rt.type = 'event'
  AND rt.event_type = ?
ORDER BY r.priority DESC;

-- Check cooldown (skip if fired too recently)
SELECT 1 FROM rule_executions
WHERE rule_id = ?
  AND started_at > datetime('now', '-' || ? || ' seconds')
  AND status IN ('success', 'partial')
LIMIT 1;

-- Get pending cron rules
SELECT r.*, rt.*
FROM rules r
JOIN rule_triggers rt ON rt.rule_id = r.id
WHERE r.enabled = 1
  AND rt.enabled = 1
  AND rt.type = 'cron';
```

## Go Implementation Patterns

### Rule Engine Core

```go
// Rule engine that subscribes to the event bus
type RuleEngine struct {
    db        *sql.DB
    bus       *eventbus.Bus
    tools     *tool.Registry
    skills    *skill.Registry
    compiled  sync.Map  // map[string]*vm.Program - cached compiled expressions
    breakers  sync.Map  // map[string]*CircuitBreaker - per-action-target
    logger    *slog.Logger
}

// Subscribe to all signal events on the bus
func (e *RuleEngine) Start(ctx context.Context) error {
    eventbus.Subscribe[signal.Event](e.bus, func(ctx context.Context, ev signal.Event) {
        e.evaluateRules(ctx, "event", ev.Source, ev)
    })
    // Also subscribe to task events, approval events, etc.
    return nil
}

// Hot path: match event against rules, evaluate conditions, execute actions
func (e *RuleEngine) evaluateRules(ctx context.Context, triggerType, eventType string, data any) {
    rules, err := e.findMatchingRules(ctx, triggerType, eventType)
    if err != nil {
        e.logger.Error("rule lookup failed", "error", err)
        return
    }
    for _, rule := range rules {
        go e.executeRule(ctx, rule, data, 0) // chain depth 0
    }
}
```

### Expression Compilation and Caching

```go
import "github.com/expr-lang/expr"

// Compile and cache expressions for reuse
func (e *RuleEngine) compileExpr(exprStr string, env any) (*vm.Program, error) {
    if cached, ok := e.compiled.Load(exprStr); ok {
        return cached.(*vm.Program), nil
    }
    program, err := expr.Compile(exprStr,
        expr.Env(env),
        expr.AsBool(),          // conditions must return bool
        expr.AllowUndefinedVariables(),
    )
    if err != nil {
        return nil, err
    }
    e.compiled.Store(exprStr, program)
    return program, nil
}

// Evaluate a condition expression against event data
func (e *RuleEngine) evalCondition(program *vm.Program, data map[string]any) (bool, error) {
    output, err := expr.Run(program, data)
    if err != nil {
        return false, err
    }
    return output.(bool), nil
}
```

### Expression Environment for Rules

```go
// The environment struct available to all rule expressions
type RuleEnv struct {
    Event    EventData         `expr:"event"`    // the triggering event
    Rule     RuleData          `expr:"rule"`     // the rule being evaluated
    Vars     map[string]any    `expr:"vars"`     // persistent rule variables
    Time     time.Time         `expr:"time"`     // current time
    Weekday  string            `expr:"weekday"`  // "Monday", "Tuesday", etc.
    Hour     int               `expr:"hour"`     // 0-23
}

type EventData struct {
    Source   string         `expr:"source"`   // "github", "gmail", etc.
    Type     string         `expr:"type"`     // "pr_opened", "email_received"
    Title    string         `expr:"title"`
    Body     string         `expr:"body"`
    Priority string         `expr:"priority"` // "low", "medium", "high"
    Tags     []string       `expr:"tags"`
    Meta     map[string]any `expr:"meta"`     // source-specific metadata
}
```

Example expressions users can write:
```
event.source == "github" && event.type == "pr_opened"
event.priority in ["high", "critical"]
event.meta.repo == "cairn" && event.meta.author != "dependabot"
len(event.tags) > 0 && "urgent" in event.tags
hour >= 9 && hour <= 17 && weekday not in ["Saturday", "Sunday"]
```

## Error Handling

### Retry Strategy

| Level | Approach | Config |
|-------|----------|--------|
| **Action** | Per-action retry with backoff | `max_retries`, `retry_delay_ms` in `rule_actions` |
| **Rule** | `on_error`: continue (try next action), stop (halt rule), retry (retry failed action) | In `rule_actions.on_error` |
| **Circuit Breaker** | Per-destination failure tracking | `failure_threshold`, `reset_timeout` per target |
| **Dead Letter** | Failed executions logged to `rule_executions` with full context | Query `status='failed'` for review |

### Error Flow

```
Action fails
  → Check on_error policy
    → "retry": retry up to max_retries with exponential backoff
      → All retries exhausted: log to rule_executions, check next policy
    → "continue": log error, proceed to next action
    → "stop": log error, mark rule execution as "partial" or "failed"
  → Check circuit breaker for target
    → If open: skip action immediately, log as "circuit_open"
    → If half-open: allow one attempt, update breaker state
```

### Preventing Runaway Rules

| Risk | Mitigation |
|------|------------|
| Infinite event loops | `max_chain` depth per rule (default 5), `chain_depth` tracked in execution |
| Rule fires too often | `cooldown_ms` per rule, checked before execution |
| Action takes too long | `timeout_ms` per action, context cancellation |
| Too many concurrent rules | `mode: single` prevents parallel execution of same rule |
| Expression bombs | `expr.MaxNodes(100)` limits expression complexity |
| Resource exhaustion | Rate limit rule evaluations per second globally |

## Common Pitfalls

| Pitfall | Why It Happens | How to Avoid |
|---------|---------------|--------------|
| Trigger-condition race | Trigger fires on event, but condition checks current state which may have changed | Design conditions to evaluate against event data, not current state |
| Expression injection | User-supplied expressions could access unsafe data | expr-lang/expr is memory-safe and side-effect-free by design; additionally restrict available functions |
| Cascading rule fires | Rule A fires event that triggers Rule B that fires event that triggers Rule A | Track chain depth, enforce max_chain, detect cycles by rule ID |
| Silent failures | Action errors swallowed, rule appears to "do nothing" | Log every execution to rule_executions, expose in UI |
| Stale compiled expressions | Rule updated but cached compiled expression not invalidated | Invalidate compiled cache entry when rule is updated |
| Cron drift | Multiple rule evaluations at same cron tick | Use atomic "claim" pattern with SQLite (same as Cairn's existing cron) |
| Over-engineering | Building a full workflow engine when simple TCA suffices | Start simple, add complexity only when needed |
| Condition ordering | AND/OR logic evaluated in wrong order | Explicit logic_group field, evaluate AND groups first |

## Best Practices

1. **Compile once, evaluate many**: Use `expr.Compile()` upfront and cache the program. Evaluation is 100x faster than parsing. (Source: expr-lang/expr benchmarks)

2. **Index triggers by event type**: The hot path is `event arrives -> find matching rules`. Index `rule_triggers(type, event_type)` for O(1) lookup instead of scanning all rules. (Source: SQLite query optimization)

3. **Separate trigger matching from condition evaluation**: First find rules whose triggers match the event type (fast SQL query). Then evaluate conditions only for matched rules (expr evaluation). This is the same two-phase approach used by Home Assistant and Drools. (Source: Home Assistant, Drools Rete algorithm)

4. **Use the event bus, not polling**: IFTTT polls triggers hourly. Home Assistant instant triggers fire immediately. Cairn already has a typed event bus - rules should subscribe to it. Only use polling for external sources without webhooks. (Source: IFTTT API docs)

5. **Persist execution history**: Every rule firing should be logged with trigger data, action results, errors, and duration. This is essential for debugging and for the orchestrator to learn which rules are effective. (Source: Temporal event history, n8n execution tracking)

6. **Cooldown prevents spam**: A rule that fires on every GitHub event will overwhelm notifications. Default cooldown of 5 minutes (configurable to 0 for no cooldown) prevents this. (Source: Cairn's existing cron cooldown pattern)

7. **Natural language rule creation**: Let the LLM translate "notify me when someone opens a PR on cairn" into a structured rule with trigger, conditions, and actions. The LLM proposes, the user approves. (Source: Home Assistant Sentence triggers, Cairn's approval model)

8. **Test rules before enabling**: Provide a "dry run" mode that evaluates the rule against recent events and shows what would have happened without executing actions. (Source: n8n manual execution, GitHub Actions dry run)

9. **Version rules**: When a rule is updated, keep the previous version for rollback. Store version history in a separate table or use soft-update with a version counter. (Source: Event sourcing pattern)

10. **Circuit breakers for external calls**: Any action that calls an external API (webhook, email, Slack) should go through a circuit breaker. Three failures in 5 minutes opens the circuit. (Source: Azure Circuit Breaker pattern)

## UI Patterns for Rule Creation

### Three Approaches

**Form-Based** (Zapier/IFTTT style):
- Step-by-step wizard: choose trigger type -> configure trigger -> add conditions -> choose action -> configure action
- Best for non-technical users
- Limited expressiveness

**Code-Based** (GitHub Actions/Home Assistant style):
- YAML or JSON editor with syntax highlighting
- Full expressiveness
- Requires technical knowledge

**Natural Language** (recommended for Cairn):
- User types: "When a high-priority GitHub PR is opened, notify me on Telegram"
- LLM parses into structured rule
- User reviews and approves the generated rule
- Combines ease of use with full expressiveness
- Aligns with Cairn's "models propose, humans dispose" principle

### Recommended UI Components

1. **Rule List**: Table of all rules with name, status (enabled/disabled), last fired, execution count, error rate
2. **Rule Editor**: Trigger/condition/action sections with form inputs + expression editor
3. **Expression Playground**: Test expressions against sample event data in real-time
4. **Execution History**: Timeline of rule firings with expandable details (trigger data, action results, errors)
5. **Rule Templates**: Pre-built rules for common patterns (e.g., "PR notification", "daily digest trigger")

## Performance Considerations

### Event Volume Estimation

Cairn's 11 pollers at 5-minute intervals produce ~130 events/hour base load. Webhook bursts (CI pipelines, active PRs) can spike to 10-50 events/minute.

### Performance Budget

| Operation | Target | Approach |
|-----------|--------|----------|
| Trigger matching | <1ms | SQL index on `(type, event_type)`, result cache |
| Condition evaluation | <0.1ms per expression | Pre-compiled expr programs, bytecode VM |
| Action dispatch | <5ms | Async dispatch via goroutines, non-blocking |
| Total per event | <10ms for 100 rules | Two-phase: SQL filter then expr eval |

### Optimization Strategies

1. **Trigger index**: Group rules by trigger event type in memory. When event arrives, only evaluate rules for that event type.
2. **Expression cache**: `sync.Map` of expression string -> compiled program. Invalidate on rule update.
3. **Batch evaluation**: When multiple events arrive simultaneously (poll batch), evaluate rules once per unique event type.
4. **Lazy loading**: Load rule conditions and actions only after trigger matches (not on every event).
5. **Worker pool**: Execute actions in a bounded goroutine pool to prevent resource exhaustion.

## Security Considerations

### Expression Safety

expr-lang/expr provides strong guarantees:
- **Memory-safe**: Cannot access unrelated memory
- **Side-effect-free**: Expressions only compute outputs from inputs
- **Always terminates**: No infinite loops possible
- **No I/O**: Cannot make network calls or access filesystem

Additional hardening:
- `expr.MaxNodes(100)`: Limit expression AST size
- `expr.DisableAllBuiltins()` + selective `expr.EnableBuiltin()`: Whitelist only needed functions
- Custom `expr.Env()`: Only expose event fields, not internal state

### Rule Injection Prevention

- Rules created via NL -> LLM must go through approval before activation
- Expression validation at save time (compile and type-check)
- Action types are enumerated (cannot execute arbitrary code)
- Action parameters are templated, not raw shell commands
- Webhook secrets are stored encrypted, not in rule config

### Resource Limits

| Resource | Limit | Enforcement |
|----------|-------|-------------|
| Max rules per user | 100 | Database constraint |
| Max actions per rule | 10 | Validation at save |
| Max conditions per rule | 20 | Validation at save |
| Expression complexity | 100 AST nodes | `expr.MaxNodes()` |
| Action timeout | 30s default | Context cancellation |
| Execution rate | 60 rule firings/minute | Token bucket rate limiter |
| Chain depth | 5 | Tracked in execution context |

## Platform Comparison Summary

| Platform | Trigger Model | Condition Engine | Action Model | Storage | Open Source |
|----------|--------------|-----------------|--------------|---------|-------------|
| IFTTT | Poll (hourly) + realtime webhook | Simple field match | Single action per applet | Cloud | No |
| Zapier | Poll + webhook | Filters (field compare) | Multi-step with paths | Cloud | No |
| Make | Poll + instant (webhook) | Filters + routers | Module chain | Cloud | No |
| Home Assistant | 20+ trigger types | State/numeric/template/AND/OR/NOT | Service calls, delays, choose, repeat | YAML + SQLite | Yes |
| GitHub Actions | 30+ event types with activity filters | `${{ }}` expressions with functions | Steps with `if` conditions | YAML in repo | Partial |
| n8n | Webhooks, schedules, app triggers | IF/Switch nodes, expressions | Node chain with branching | SQLite/Postgres | Yes |
| Windmill | 12+ types (schedule, webhook, Kafka, MQTT, Postgres) | Script-based | Flow steps with error handlers | Postgres | Yes |
| Temporal | Events, signals, schedules | Workflow code (deterministic) | Activities with retry policies | MySQL/Postgres/Cassandra | Yes |
| Inngest | Events + cron + webhook | CEL filter expressions | Step functions with durable execution | Cloud/self-host | Yes |

## Recommended Architecture for Cairn

```
                    Event Bus (typed pub/sub)
                           |
                    +------+------+
                    |             |
              Rule Engine    Orchestrator (LLM)
              (deterministic)  (intelligent)
                    |
         +----+----+----+
         |    |    |    |
      Trigger Cond  Action  History
      Index   Eval  Dispatch  Log
      (SQL)  (expr) (async)  (SQLite)
```

**Division of labor**:
- Rules engine: fast, deterministic reactions ("when PR opened on cairn, notify Telegram")
- Orchestrator: complex decisions requiring reasoning ("should I auto-approve this memory?")
- Rules can submit tasks to the orchestrator for decisions that need LLM reasoning

**Implementation phases**:
1. Schema + rule CRUD API
2. expr-lang/expr integration for condition evaluation
3. Event bus subscription + trigger matching
4. Action executor (start with notify + run skill)
5. Cron trigger integration (reuse existing cron scheduler)
6. Execution history + UI
7. NL rule creation via LLM
8. Circuit breaker + retry policies

## Further Reading

| Resource | Type | Why Recommended |
|----------|------|-----------------|
| [expr-lang/expr](https://github.com/expr-lang/expr) | Go library | Primary expression evaluator - safe, fast, excellent Go integration |
| [cel-go](https://github.com/google/cel-go) | Go library | Alternative expression evaluator if cross-language compatibility needed |
| [Home Assistant Automations](https://www.home-assistant.io/docs/automation/) | Documentation | Best-in-class TCA model with 20+ trigger types, comprehensive conditions |
| [GitHub Actions Events](https://docs.github.com/en/actions/writing-workflows/choosing-when-your-workflow-runs/events-that-trigger-workflows) | Documentation | Comprehensive event trigger model with expression syntax |
| [Inngest Function API](https://www.inngest.com/docs/reference/functions/create) | Documentation | Modern event-driven function model with concurrency, throttling, batching |
| [Martin Fowler: Event-Driven](https://martinfowler.com/articles/201701-event-driven.html) | Article | Definitive taxonomy of event-driven patterns |
| [Temporal Workflow Principles](https://temporal.io/blog/workflow-engine-principles) | Article | Deep dive on durable execution, deterministic replay, transactional queues |
| [Saga Pattern (Microsoft)](https://learn.microsoft.com/en-us/azure/architecture/patterns/saga) | Architecture guide | Choreography vs orchestration for multi-step distributed transactions |
| [Circuit Breaker Pattern (Microsoft)](https://learn.microsoft.com/en-us/azure/architecture/patterns/circuit-breaker) | Architecture guide | Failure handling with states, thresholds, and recovery |
| [OPA/Rego Policy Language](https://www.openpolicyagent.org/docs/latest/policy-language/) | Documentation | Declarative policy evaluation if complex policy logic needed |
| [robfig/cron](https://github.com/robfig/cron) | Go library | Cron scheduling with timezone support, job wrapping |
| [GoRules Zen](https://github.com/gorules/zen) | Rules engine | Decision tables + expression language, JSON-based rules |
| [grule-rule-engine](https://github.com/hyperjumptech/grule-rule-engine) | Go library | Drools-like GRL syntax with Rete evaluation |
| [SQLite Triggers](https://www.sqlite.org/lang_createtrigger.html) | Documentation | Database-level automation for audit trails and cascading updates |
| [n8n Workflows](https://docs.n8n.io/workflows/) | Documentation | Node-based workflow patterns, error handling, branching |

---

*This guide was synthesized from 42 sources. See `resources/automation-rules-engines-sources.json` for full source list.*
