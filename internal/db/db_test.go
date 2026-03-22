package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// helper opens an in-memory DB and runs migrations.
func openTestDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:): %v", err)
	}
	if err := d.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestOpenAndMigrate(t *testing.T) {
	d := openTestDB(t)

	// Verify all expected tables exist.
	tables := []string{
		"events", "tasks", "approvals", "sessions",
		"messages", "memories", "source_state", "schema_migrations",
	}
	for _, tbl := range tables {
		var name string
		err := d.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", tbl, err)
		}
	}
}

func TestMigrateIdempotent(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	if err := d.Migrate(); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := d.Migrate(); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}

	// Should still have exactly ten migration records (001-010).
	var count int
	if err := d.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 10 {
		t.Errorf("expected 10 migration records, got %d", count)
	}
}

func TestInsertEvent(t *testing.T) {
	d := openTestDB(t)

	_, err := d.Exec(`
		INSERT INTO events (id, source, source_item_id, kind, title, body, url, actor)
		VALUES ('evt-001', 'github', 'gh-123', 'pr', 'Fix bug', 'Fixes #42', 'https://github.com/test', 'alice')
	`)
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}

	var (
		id, source, kind, title, actor string
		createdAt                      string
	)
	err = d.QueryRow("SELECT id, source, kind, title, actor, created_at FROM events WHERE id = 'evt-001'").
		Scan(&id, &source, &kind, &title, &actor, &createdAt)
	if err != nil {
		t.Fatalf("query event: %v", err)
	}

	if id != "evt-001" || source != "github" || kind != "pr" || title != "Fix bug" || actor != "alice" {
		t.Errorf("unexpected values: id=%s source=%s kind=%s title=%s actor=%s", id, source, kind, title, actor)
	}

	// Verify created_at was auto-populated (ISO-8601 format).
	_, err = time.Parse("2006-01-02T15:04:05.000Z", createdAt)
	if err != nil {
		t.Errorf("created_at not valid ISO-8601: %q: %v", createdAt, err)
	}
}

func TestInsertTask(t *testing.T) {
	d := openTestDB(t)

	_, err := d.Exec(`
		INSERT INTO tasks (id, type, description, priority)
		VALUES ('task-001', 'review', 'Review PR #42', 5)
	`)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}

	var (
		id, typ, status, desc string
		priority              int
	)
	err = d.QueryRow("SELECT id, type, status, description, priority FROM tasks WHERE id = 'task-001'").
		Scan(&id, &typ, &status, &desc, &priority)
	if err != nil {
		t.Fatalf("query task: %v", err)
	}

	if status != "pending" {
		t.Errorf("expected default status 'pending', got %q", status)
	}
	if priority != 5 {
		t.Errorf("expected priority 5, got %d", priority)
	}
}

func TestInsertSession(t *testing.T) {
	d := openTestDB(t)

	// Insert a session.
	_, err := d.Exec(`
		INSERT INTO sessions (id, title, mode) VALUES ('sess-001', 'Test Chat', 'talk')
	`)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}

	// Insert messages into the session.
	_, err = d.Exec(`
		INSERT INTO messages (id, session_id, role, content) VALUES
			('msg-001', 'sess-001', 'user', 'Hello'),
			('msg-002', 'sess-001', 'assistant', 'Hi there!')
	`)
	if err != nil {
		t.Fatalf("insert messages: %v", err)
	}

	// Query messages by session.
	rows, err := d.Query("SELECT id, role, content FROM messages WHERE session_id = 'sess-001' ORDER BY created_at")
	if err != nil {
		t.Fatalf("query messages: %v", err)
	}
	defer rows.Close()

	type msg struct {
		id, role, content string
	}
	var msgs []msg
	for rows.Next() {
		var m msg
		if err := rows.Scan(&m.id, &m.role, &m.content); err != nil {
			t.Fatalf("scan message: %v", err)
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].role != "user" || msgs[1].role != "assistant" {
		t.Errorf("unexpected roles: %v", msgs)
	}
}

func TestInsertMemory(t *testing.T) {
	d := openTestDB(t)

	_, err := d.Exec(`
		INSERT INTO memories (id, content, category, status)
		VALUES ('mem-001', 'User prefers dark mode', 'preference', 'accepted')
	`)
	if err != nil {
		t.Fatalf("insert memory: %v", err)
	}

	// Query by status.
	var count int
	err = d.QueryRow("SELECT COUNT(*) FROM memories WHERE status = 'accepted'").Scan(&count)
	if err != nil {
		t.Fatalf("count memories: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 accepted memory, got %d", count)
	}

	// Query by category.
	var content string
	err = d.QueryRow("SELECT content FROM memories WHERE category = 'preference'").Scan(&content)
	if err != nil {
		t.Fatalf("query by category: %v", err)
	}
	if content != "User prefers dark mode" {
		t.Errorf("unexpected content: %q", content)
	}
}

func TestWALMode(t *testing.T) {
	// WAL mode can only be verified on a real file, not :memory:.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test-wal.db")

	d, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q): %v", dbPath, err)
	}
	defer d.Close()

	var journalMode string
	if err := d.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("expected journal_mode=wal, got %q", journalMode)
	}

	// Verify WAL file exists after a write.
	if err := d.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	_, err = os.Stat(dbPath + "-wal")
	if err != nil {
		t.Logf("WAL file check: %v (may not exist until write is flushed)", err)
	}
}

func TestUniqueConstraint(t *testing.T) {
	d := openTestDB(t)

	// Insert first event.
	_, err := d.Exec(`
		INSERT INTO events (id, source, source_item_id, kind, title)
		VALUES ('evt-001', 'github', 'gh-123', 'pr', 'First')
	`)
	if err != nil {
		t.Fatalf("insert first event: %v", err)
	}

	// Insert duplicate source+source_item_id should fail.
	_, err = d.Exec(`
		INSERT INTO events (id, source, source_item_id, kind, title)
		VALUES ('evt-002', 'github', 'gh-123', 'pr', 'Duplicate')
	`)
	if err == nil {
		t.Fatal("expected unique constraint violation, got nil error")
	}

	// Verify only one event exists.
	var count int
	if err := d.QueryRow("SELECT COUNT(*) FROM events").Scan(&count); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 event after duplicate rejection, got %d", count)
	}
}

func TestForeignKeyEnforcement(t *testing.T) {
	d := openTestDB(t)

	// Inserting a message with a non-existent session_id should fail
	// because foreign_keys is ON.
	_, err := d.Exec(`
		INSERT INTO messages (id, session_id, role, content)
		VALUES ('msg-orphan', 'nonexistent-session', 'user', 'Hello')
	`)
	if err == nil {
		t.Fatal("expected foreign key violation, got nil error")
	}
}

func TestSourceState(t *testing.T) {
	d := openTestDB(t)

	_, err := d.Exec(`
		INSERT INTO source_state (key, value) VALUES ('github_cursor', '{"etag":"abc"}')
	`)
	if err != nil {
		t.Fatalf("insert source_state: %v", err)
	}

	var value string
	err = d.QueryRow("SELECT value FROM source_state WHERE key = 'github_cursor'").Scan(&value)
	if err != nil {
		t.Fatalf("query source_state: %v", err)
	}
	if value != `{"etag":"abc"}` {
		t.Errorf("unexpected value: %q", value)
	}

	// Upsert via REPLACE.
	_, err = d.Exec(`
		INSERT OR REPLACE INTO source_state (key, value) VALUES ('github_cursor', '{"etag":"def"}')
	`)
	if err != nil {
		t.Fatalf("upsert source_state: %v", err)
	}

	err = d.QueryRow("SELECT value FROM source_state WHERE key = 'github_cursor'").Scan(&value)
	if err != nil {
		t.Fatalf("query after upsert: %v", err)
	}
	if value != `{"etag":"def"}` {
		t.Errorf("unexpected value after upsert: %q", value)
	}
}

func TestApprovalForeignKey(t *testing.T) {
	d := openTestDB(t)

	// Insert a task first.
	_, err := d.Exec(`
		INSERT INTO tasks (id, type, description) VALUES ('task-001', 'deploy', 'Deploy v2')
	`)
	if err != nil {
		t.Fatalf("insert task: %v", err)
	}

	// Insert an approval referencing that task.
	_, err = d.Exec(`
		INSERT INTO approvals (id, task_id, type, description)
		VALUES ('appr-001', 'task-001', 'manual', 'Approve deploy')
	`)
	if err != nil {
		t.Fatalf("insert approval: %v", err)
	}

	var status string
	err = d.QueryRow("SELECT status FROM approvals WHERE id = 'appr-001'").Scan(&status)
	if err != nil {
		t.Fatalf("query approval: %v", err)
	}
	if status != "pending" {
		t.Errorf("expected default status 'pending', got %q", status)
	}
}

func TestNullableFields(t *testing.T) {
	d := openTestDB(t)

	// Insert event without optional fields.
	_, err := d.Exec(`
		INSERT INTO events (id, source, kind) VALUES ('evt-minimal', 'test', 'ping')
	`)
	if err != nil {
		t.Fatalf("insert minimal event: %v", err)
	}

	var readAt, archivedAt sql.NullString
	err = d.QueryRow("SELECT read_at, archived_at FROM events WHERE id = 'evt-minimal'").
		Scan(&readAt, &archivedAt)
	if err != nil {
		t.Fatalf("query nullable fields: %v", err)
	}
	if readAt.Valid {
		t.Errorf("expected read_at to be NULL, got %q", readAt.String)
	}
	if archivedAt.Valid {
		t.Errorf("expected archived_at to be NULL, got %q", archivedAt.String)
	}
}
