# Cron System + Agent Resilience — Research Notes

> 108 sources analyzed across Pub, Gollem, Eino, Plandex, Uzi, ADK-Go, OpenCode.
> Compiled 2026-03-19 for Cairn cron system + always-on agent resilience.

## Key Research Findings

### Cron Evaluation Pattern (from Pub)
- Pub evaluates ALL schedule triggers every tick (no dedicated scheduler)
- Uses `croner` library (TypeScript) for minute-level cron matching
- `isScheduleDue(trigger, nowMs)` normalizes to whole-minute boundaries
- Caches parsed Cron instances in module-level Map
- Cooldown + budget caps prevent duplicate fires and cost overrun
- Execution log tracks: fired, task_created, skipped_cooldown, skipped_budget, error

### Go Cron Libraries
- **robfig/cron/v3** — de facto standard, 5-field expressions, minimal deps, EntryID system
  - `cron.ParseStandard(expr)` for validation
  - `Schedule.Next(time.Time)` for next fire time
  - `WithChain(SkipIfStillRunning(...))` middleware for overlap prevention
- **go-co-op/gocron** — modern alternative, fluent API, job tags, slightly heavier
- **Recommendation**: robfig/cron — simpler, more standard, sufficient for our needs

### Agent-Driven Cron vs Traditional Cron
- Traditional: execute shell commands, fire-and-forget
- Agent-driven: interpret NL instructions, multi-step reasoning, memory-aware
- Each fire creates a task → agent claims → executes with full tool access
- Instructions can reference memories: "Check if Serena responded to my email"
- Cron jobs should have their own sessions (independent, journaled)

### Always-On Agent Patterns

**Cairn (current):**
- Fixed 60s tick, task-pull model (claim pending → execute)
- No startup recovery, no state persistence
- Reaper exists but never started
- Reflection every 30min (timing lost on restart)

**Pub:**
- LLM-driven decision loop: "what should I do now?" each tick
- Scanners for proactive actions (research, reach_out, curate_memory)
- Adaptive tick intervals (2min active, 5min idle)

**Gollem:**
- RunStateSnapshot for checkpoint/resume
- RecoveryManager sweeps expired leases independently
- Decoupled recovery — runs as startup hook, not background

**Eino:**
- CheckPointStore interface (pluggable: memory, DB, file)
- Graph-aware checkpoints (per-node state + subgraphs)
- Interrupt/resume for human-in-the-loop

**Plandex:**
- Queue-based operation batching (single writer principle)
- Context-based cancellation with ordered shutdown
- Async status tracking to DB

### Resilience Gaps in Cairn

| Gap | Impact | Fix |
|-----|--------|-----|
| No startup recovery | Stuck tasks accumulate after crash | Scan + fail expired on startup |
| Tick count lost | Metrics reset, can't track uptime | Persist to agent_loop_state table |
| Reflection timing lost | Reflects immediately after restart instead of waiting | Persist last_reflection_at |
| Reaper not started | Expired leases never cleaned | Call StartReaper(1min) in main.go |
| Mid-session state lost | Crash during task = full retry from start | Accept for now (task retry handles it) |

### State That Already Survives Restart
- Tasks (all lifecycle in SQLite)
- Sessions + messages (persisted after completion)
- Journal entries (written after session)
- Memories (embedded, in SQLite)
- Source state (polling cursors in source_state table)

## Cron Expression Syntax Reference

Standard 5-field format (no seconds):
```
┌───────── minute (0-59)
│ ┌─────── hour (0-23)
│ │ ┌───── day of month (1-31)
│ │ │ ┌─── month (1-12)
│ │ │ │ ┌─ day of week (0-6, 0=Sunday)
│ │ │ │ │
* * * * *

Operators: * (all), - (range), , (list), / (step)

Examples:
  0 9 * * 1-5       Weekdays at 9am
  */30 * * * *       Every 30 minutes
  0 0 1 * *          First day of month at midnight
  0 9,17 * * *       9am and 5pm daily
  0 */4 * * *        Every 4 hours
```

## Schema Design Notes

### cron_jobs table
- `schedule` stores raw expression (e.g. "0 9 * * 1-5")
- `instruction` is natural language (agent interprets each fire)
- `next_run_at` computed after each fire for display + efficient due-job query
- `cooldown_ms` prevents duplicate fires (default 1h)
- `timezone` for display only — all DB times are UTC

### cron_executions table
- Links to tasks table via `task_id`
- `status`: fired (task created), completed, failed, skipped_cooldown
- Cascade delete when cron job deleted

### agent_loop_state table
- Single row (id='agent'), upserted after each tick
- Stores tick_count + last_reflection_at
- Restored on startup before loop begins
