-- Agent activity log for observability.
CREATE TABLE IF NOT EXISTS agent_activity (
    id          TEXT PRIMARY KEY,
    type        TEXT NOT NULL,       -- task, idle, reflection, cron, error
    summary     TEXT NOT NULL,
    details     TEXT DEFAULT '',
    errors      TEXT DEFAULT '[]',   -- JSON array of error strings
    tool_count  INTEGER DEFAULT 0,
    duration_ms INTEGER DEFAULT 0,
    created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_activity_created ON agent_activity(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_activity_type ON agent_activity(type);

-- Per-tool execution stats (upserted on each tool call).
CREATE TABLE IF NOT EXISTS tool_stats (
    tool_name   TEXT PRIMARY KEY,
    calls       INTEGER DEFAULT 0,
    errors      INTEGER DEFAULT 0,
    total_ms    INTEGER DEFAULT 0,
    last_error  TEXT,
    updated_at  TEXT NOT NULL
);
