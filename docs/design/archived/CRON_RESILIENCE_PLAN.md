# Cron System + Agent Resilience

> Scheduled tasks driven by the agent loop, with crash recovery and state persistence.
> Research: Pub automation engine, robfig/cron, Gollem RunStateSnapshot, Eino CheckPointStore, Plandex queue.

## Current State

Agent loop ticks every 60s, claims tasks, runs reflection every 30min. 436 tests, 8 pollers, 36 tools.
Task engine has lease-based claiming + reaper pattern (exists but **never auto-started**).
No cron tables. No agent state persistence. Tick count + reflection timing lost on restart.

---

## PR 1: Cron System

### Schema (new migration)

```sql
CREATE TABLE cron_jobs (
    id          TEXT PRIMARY KEY,
    enabled     INTEGER NOT NULL DEFAULT 1,
    name        TEXT NOT NULL UNIQUE,
    description TEXT,
    schedule    TEXT NOT NULL,           -- 5-field cron: "0 9 * * 1-5"
    instruction TEXT NOT NULL,           -- NL: "Check email and summarize"
    timezone    TEXT DEFAULT 'UTC',
    priority    INTEGER DEFAULT 3,       -- task priority (0=critical, 4=idle)
    cooldown_ms INTEGER DEFAULT 3600000, -- 1h min between fires
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    last_run_at TEXT,
    next_run_at TEXT
);

CREATE TABLE cron_executions (
    id          TEXT PRIMARY KEY,
    cron_job_id TEXT NOT NULL REFERENCES cron_jobs(id) ON DELETE CASCADE,
    task_id     TEXT,
    status      TEXT NOT NULL,           -- fired, completed, failed, skipped_cooldown
    error       TEXT,
    created_at  TEXT NOT NULL
);

CREATE INDEX idx_cron_jobs_enabled ON cron_jobs(enabled, next_run_at);
CREATE INDEX idx_cron_executions_job ON cron_executions(cron_job_id, created_at DESC);
```

### Cron Store (`internal/cron/store.go`)

```go
type Store struct { db *sql.DB }

type CronJob struct {
    ID, Name, Description, Schedule, Instruction, Timezone string
    Enabled    bool
    Priority   int
    CooldownMs int64
    CreatedAt, UpdatedAt time.Time
    LastRunAt, NextRunAt *time.Time
}

func (s *Store) Create(ctx, job) (*CronJob, error)
func (s *Store) List(ctx) ([]*CronJob, error)
func (s *Store) Get(ctx, id) (*CronJob, error)
func (s *Store) Update(ctx, id, updates) (*CronJob, error)
func (s *Store) Delete(ctx, id) error
func (s *Store) GetDueJobs(ctx, now) ([]*CronJob, error)    // enabled + next_run <= now + cooldown check
func (s *Store) RecordExecution(ctx, cronID, taskID, status, err) error
func (s *Store) UpdateNextRun(ctx, id, nextRun) error
```

### Cron Helper (`internal/cron/cron.go`)

Uses `robfig/cron/v3` (add to go.mod):

```go
func Validate(expr string) error                    // parse 5-field expression
func NextRun(expr string, after time.Time) time.Time // compute next fire time
func IsDue(expr string, now time.Time) bool          // minute-level granularity check
```

### Agent Loop Integration

In `loop.tick()`, after task execution and before reflection:

```go
// Check for due cron jobs.
if l.cronStore != nil {
    dueJobs, _ := l.cronStore.GetDueJobs(ctx, time.Now())
    for _, job := range dueJobs {
        task, err := l.tasks.Submit(ctx, &task.SubmitRequest{
            Type:        "cron",
            Priority:    job.Priority,
            Description: "Cron: " + job.Name,
            Input:       json.Marshal(map[string]string{"instruction": job.Instruction}),
        })
        if err == nil {
            l.cronStore.RecordExecution(ctx, job.ID, task.ID, "fired", nil)
            l.cronStore.UpdateNextRun(ctx, job.ID, cron.NextRun(job.Schedule, time.Now()))
        }
    }
}
```

### Tools

```go
cairn.createCron  — name, schedule, instruction, priority, timezone
cairn.listCrons   — returns all jobs with next_run, last_run, enabled
cairn.deleteCron  — by ID or name
```

All modes (talk/work/coding). Needs `CronService` interface on ToolContext.

### REST API

```
POST   /v1/crons              — create cron job (validate schedule expression)
GET    /v1/crons              — list all jobs + next/last run
GET    /v1/crons/{id}         — get job + recent executions
PATCH  /v1/crons/{id}         — update (enable/disable, change schedule/instruction)
DELETE /v1/crons/{id}         — delete job + cascade executions
```

### Key Decisions

1. **Natural language instructions** — agent interprets dynamically each fire, not shell commands
2. **robfig/cron/v3** — industry standard, minimal deps, 5-field expressions
3. **Skip missed executions** — if agent was offline, don't catch up (simpler, less spam)
4. **Cooldown per job** — default 1h, prevents duplicate fires if tick happens twice in same minute
5. **Minute-level granularity** — normalize to whole-minute boundaries for deterministic matching
6. **Each fire = task submission** — reuses existing task engine (priority, retry, lease)
7. **Cron jobs create their own sessions** — each execution is independent, journaled

### Files

| File | Action |
|------|--------|
| `internal/cron/store.go` | **NEW** — CronJob CRUD + GetDueJobs |
| `internal/cron/cron.go` | **NEW** — Validate, NextRun, IsDue (wraps robfig/cron) |
| `internal/cron/store_test.go` | **NEW** — CRUD + due job detection tests |
| `internal/cron/cron_test.go` | **NEW** — expression validation + next run tests |
| `internal/db/migrations/004_cron.sql` | **NEW** — cron_jobs + cron_executions tables |
| `internal/tool/builtin/cron.go` | **NEW** — createCron, listCrons, deleteCron tools |
| `internal/tool/tool.go` | Add CronService interface + field on ToolContext |
| `internal/tool/builtin/register.go` | Register 3 cron tools |
| `internal/agent/loop.go` | Add cronStore field, due-job check in tick() |
| `internal/agent/types.go` | Add CronStore to Loop config |
| `internal/server/routes.go` | Add 5 cron API endpoints |
| `cmd/cairn/main.go` | Wire CronStore, register routes, pass to loop |
| `go.mod` | Add `github.com/robfig/cron/v3` |

---

## PR 2: Agent Resilience

### Schema (same migration or separate)

```sql
CREATE TABLE agent_loop_state (
    id                  TEXT PRIMARY KEY DEFAULT 'agent',
    tick_count          INTEGER DEFAULT 0,
    last_reflection_at  TEXT,
    updated_at          TEXT NOT NULL
);
```

### Startup Recovery (`internal/agent/recovery.go`)

On server start, before loop begins:

```go
func RecoverOnStartup(ctx, db, taskEngine, logger) error {
    // 1. Fail stuck tasks — status=running/claimed with expired lease
    stuck := taskEngine.FindExpired(ctx)
    for _, t := range stuck {
        taskEngine.Fail(ctx, t.ID, "stuck_task_recovery: server restarted")
        logger.Info("recovered stuck task", "id", t.ID)
    }

    // 2. Restore loop state from DB
    state := loadAgentLoopState(db)
    return state  // caller sets loop.tickCount and loop.lastReflect
}
```

### State Checkpoint

After each tick, persist to `agent_loop_state`:

```go
func (l *Loop) checkpointState(ctx) {
    upsert agent_loop_state SET
        tick_count = l.tickCount.Load(),
        last_reflection_at = l.lastReflect,
        updated_at = time.Now()
    WHERE id = 'agent'
}
```

### Auto-Start Reaper

Currently `task.Engine` has `StartReaper()` but it's **never called** in main.go. Fix:

```go
// In main.go, after creating task engine:
taskEngine.StartReaper(1 * time.Minute)
defer taskEngine.Close()
```

### Graceful Shutdown

Already partially exists (context cancellation). Ensure:
- `loop.Close()` waits for current tick to finish
- Signal handler calls `loop.Close()` before `server.Shutdown()`
- Journal entry written even on shutdown (current fire-and-forget is fine — goroutine completes)

### Files

| File | Action |
|------|--------|
| `internal/agent/recovery.go` | **NEW** — RecoverOnStartup, loadAgentLoopState |
| `internal/agent/recovery_test.go` | **NEW** — stuck task recovery, state restore tests |
| `internal/agent/loop.go` | Add checkpointState(), restore on init |
| `internal/db/migrations/004_cron.sql` | Add agent_loop_state table (same migration) |
| `cmd/cairn/main.go` | Call RecoverOnStartup, StartReaper, wire shutdown |

---

## Implementation Order

```
PR 1: Cron System
  Step 1: go get robfig/cron/v3
  Step 2: Migration 004 (cron_jobs + cron_executions + agent_loop_state)
  Step 3: internal/cron/ (store + helper + tests)
  Step 4: Tools (createCron, listCrons, deleteCron)
  Step 5: Agent loop integration (due-job check in tick)
  Step 6: REST API endpoints
  Step 7: Wire in main.go

PR 2: Agent Resilience (can be same PR or follow-up)
  Step 1: recovery.go (startup scan + state restore)
  Step 2: Loop state checkpoint (persist after each tick)
  Step 3: Auto-start reaper in main.go
  Step 4: Verify graceful shutdown
```

## Research Sources

| Source | Pattern | Applied To |
|--------|---------|-----------|
| Pub automation engine (`engine.ts`) | Schedule trigger + cooldown + budget cap + execution log | Cron evaluation model |
| Pub `croner` library | Minute-level cron matching | Granularity design |
| `robfig/cron/v3` | 5-field expressions, Go standard | Expression parsing |
| Gollem `RunStateSnapshot` | Agent state capture/restore | Loop state checkpoint |
| Gollem `RecoveryManager` | Sweep for expired leases | Startup recovery |
| Eino `CheckPointStore` | Pluggable checkpoint/resume | State persistence pattern |
| Plandex `queue.go` | Background op queue, single writer | Task queue model |
| Uzi `state.go` | Per-agent file-based state | Simple state persistence |
| Cairn `task/engine.go` | Lease-based claiming + reaper (exists!) | Auto-start reaper |
| Cairn `signal/scheduler.go` | Ticker-based polling | Agent loop model |
