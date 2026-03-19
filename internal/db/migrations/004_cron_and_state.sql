-- Cron jobs: user-defined recurring tasks with natural language instructions.
CREATE TABLE IF NOT EXISTS cron_jobs (
    id          TEXT PRIMARY KEY,
    enabled     INTEGER NOT NULL DEFAULT 1,
    name        TEXT NOT NULL UNIQUE,
    description TEXT DEFAULT '',
    schedule    TEXT NOT NULL,
    instruction TEXT NOT NULL,
    timezone    TEXT DEFAULT 'UTC',
    priority    INTEGER DEFAULT 3,
    cooldown_ms INTEGER DEFAULT 3600000,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    last_run_at TEXT,
    next_run_at TEXT
);

-- Execution log: audit trail of cron fires.
CREATE TABLE IF NOT EXISTS cron_executions (
    id          TEXT PRIMARY KEY,
    cron_job_id TEXT NOT NULL REFERENCES cron_jobs(id) ON DELETE CASCADE,
    task_id     TEXT,
    status      TEXT NOT NULL,
    error       TEXT,
    created_at  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_cron_enabled ON cron_jobs(enabled, next_run_at);
CREATE INDEX IF NOT EXISTS idx_cron_exec_job ON cron_executions(cron_job_id, created_at DESC);

-- Agent loop state: survives restarts.
CREATE TABLE IF NOT EXISTS agent_loop_state (
    id                  TEXT PRIMARY KEY,
    tick_count          INTEGER DEFAULT 0,
    last_reflection_at  TEXT,
    updated_at          TEXT NOT NULL
);
