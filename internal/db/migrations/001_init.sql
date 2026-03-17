-- Core tables for Pub v2

-- Feed events (Signal Plane)
CREATE TABLE IF NOT EXISTS events (
    id              TEXT PRIMARY KEY,
    source          TEXT NOT NULL,
    source_item_id  TEXT,
    kind            TEXT NOT NULL,
    title           TEXT NOT NULL DEFAULT '',
    body            TEXT NOT NULL DEFAULT '',
    url             TEXT NOT NULL DEFAULT '',
    actor           TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    read_at         TEXT,
    archived_at     TEXT,
    metadata        TEXT DEFAULT '{}',
    group_key       TEXT,
    UNIQUE(source, source_item_id)
);
CREATE INDEX IF NOT EXISTS idx_events_created ON events(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_source ON events(source);
CREATE INDEX IF NOT EXISTS idx_events_unread ON events(read_at) WHERE read_at IS NULL;

-- Tasks (Action Plane)
CREATE TABLE IF NOT EXISTS tasks (
    id          TEXT PRIMARY KEY,
    type        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    description TEXT NOT NULL DEFAULT '',
    input       TEXT DEFAULT '{}',
    output      TEXT,
    error       TEXT,
    priority    INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    started_at  TEXT,
    completed_at TEXT,
    archived_at TEXT,
    lease_owner TEXT,
    lease_expires_at TEXT,
    metadata    TEXT DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_priority ON tasks(priority DESC, created_at ASC);

-- Approvals (Action Plane)
CREATE TABLE IF NOT EXISTS approvals (
    id          TEXT PRIMARY KEY,
    task_id     TEXT REFERENCES tasks(id),
    type        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    description TEXT NOT NULL DEFAULT '',
    context     TEXT DEFAULT '{}',
    decided_at  TEXT,
    decided_by  TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_approvals_status ON approvals(status);

-- Chat sessions (Assistant Plane)
CREATE TABLE IF NOT EXISTS sessions (
    id          TEXT PRIMARY KEY,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    title       TEXT NOT NULL DEFAULT '',
    mode        TEXT NOT NULL DEFAULT 'talk',
    metadata    TEXT DEFAULT '{}'
);

-- Chat messages
CREATE TABLE IF NOT EXISTS messages (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES sessions(id),
    role        TEXT NOT NULL,
    content     TEXT NOT NULL DEFAULT '',
    mode        TEXT,
    tool_calls  TEXT,
    tool_results TEXT,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    tokens_in   INTEGER DEFAULT 0,
    tokens_out  INTEGER DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, created_at);

-- Memories (Memory System)
CREATE TABLE IF NOT EXISTS memories (
    id          TEXT PRIMARY KEY,
    content     TEXT NOT NULL,
    category    TEXT NOT NULL DEFAULT 'general',
    scope       TEXT NOT NULL DEFAULT 'global',
    status      TEXT NOT NULL DEFAULT 'proposed',
    confidence  REAL NOT NULL DEFAULT 0.5,
    source      TEXT NOT NULL DEFAULT 'agent',
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    embedding   BLOB,
    access_count INTEGER NOT NULL DEFAULT 0,
    last_accessed_at TEXT,
    metadata    TEXT DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_memories_status ON memories(status);
CREATE INDEX IF NOT EXISTS idx_memories_category ON memories(category);

-- Source state (for pollers, checkpoints)
CREATE TABLE IF NOT EXISTS source_state (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL DEFAULT '{}',
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- Migration tracking
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     INTEGER PRIMARY KEY,
    applied_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
