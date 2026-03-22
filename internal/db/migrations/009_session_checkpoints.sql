-- Session checkpoints: track interrupted sessions for crash recovery.
-- One row per active session; deleted on successful completion.
CREATE TABLE IF NOT EXISTS session_checkpoints (
    session_id   TEXT PRIMARY KEY,
    task_id      TEXT DEFAULT '',
    round        INTEGER NOT NULL DEFAULT 0,
    mode         TEXT NOT NULL DEFAULT 'talk',
    max_rounds   INTEGER NOT NULL DEFAULT 40,
    user_message TEXT NOT NULL DEFAULT '',
    origin       TEXT NOT NULL DEFAULT 'chat',
    state        TEXT DEFAULT '{}',
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
