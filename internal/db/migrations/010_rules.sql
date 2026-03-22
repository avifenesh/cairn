-- Automation rules: declarative "when X happens, do Y"
CREATE TABLE IF NOT EXISTS rules (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL UNIQUE,
    description   TEXT DEFAULT '',
    enabled       INTEGER NOT NULL DEFAULT 1,
    trigger       TEXT NOT NULL,
    condition     TEXT DEFAULT '',
    actions       TEXT NOT NULL DEFAULT '[]',
    throttle_ms   INTEGER DEFAULT 0,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    last_fired_at TEXT
);

-- Execution log: audit trail of rule fires
CREATE TABLE IF NOT EXISTS rule_executions (
    id            TEXT PRIMARY KEY,
    rule_id       TEXT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
    trigger_event TEXT,
    status        TEXT NOT NULL,
    error         TEXT,
    duration_ms   INTEGER,
    created_at    TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_rules_enabled ON rules(enabled);
CREATE INDEX IF NOT EXISTS idx_rule_exec_rule ON rule_executions(rule_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_rule_exec_created ON rule_executions(created_at DESC);
