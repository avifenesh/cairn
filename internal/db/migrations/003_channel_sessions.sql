CREATE TABLE IF NOT EXISTS channel_sessions (
    channel    TEXT NOT NULL,
    chat_id    TEXT NOT NULL,
    session_id TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (channel, chat_id)
);
