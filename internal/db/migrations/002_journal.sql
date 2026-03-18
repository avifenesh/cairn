-- Session journal for episodic memory (Phase 5)
CREATE TABLE IF NOT EXISTS session_journal (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL,
    summary     TEXT NOT NULL DEFAULT '',
    decisions   TEXT NOT NULL DEFAULT '[]',
    errors      TEXT NOT NULL DEFAULT '[]',
    learnings   TEXT NOT NULL DEFAULT '[]',
    entities    TEXT NOT NULL DEFAULT '[]',
    tool_count  INTEGER NOT NULL DEFAULT 0,
    round_count INTEGER NOT NULL DEFAULT 0,
    mode        TEXT NOT NULL DEFAULT 'talk',
    duration_ms INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_journal_created ON session_journal(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_journal_session ON session_journal(session_id);
