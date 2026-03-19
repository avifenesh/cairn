package agent

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func testRecoveryDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	for _, ddl := range []string{
		`CREATE TABLE agent_loop_state (
			id TEXT PRIMARY KEY, tick_count INTEGER DEFAULT 0,
			last_reflection_at TEXT, updated_at TEXT NOT NULL)`,
		`CREATE TABLE tasks (
			id TEXT PRIMARY KEY, type TEXT, status TEXT NOT NULL,
			description TEXT, error TEXT, priority INTEGER DEFAULT 0,
			lease_owner TEXT, lease_expires_at TEXT,
			created_at TEXT, updated_at TEXT, completed_at TEXT)`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("create table: %v", err)
		}
	}
	return db
}

func TestRecoverOnStartup_NoState(t *testing.T) {
	db := testRecoveryDB(t)
	logger := slog.Default()

	state := RecoverOnStartup(context.Background(), db, logger)
	if state.TickCount != 0 {
		t.Errorf("expected tick count 0, got %d", state.TickCount)
	}
	if !state.LastReflection.IsZero() {
		t.Errorf("expected zero time, got %v", state.LastReflection)
	}
}

func TestRecoverOnStartup_RestoresState(t *testing.T) {
	db := testRecoveryDB(t)
	logger := slog.Default()

	// Insert prior state.
	db.Exec(`INSERT INTO agent_loop_state (id, tick_count, last_reflection_at, updated_at)
		VALUES ('agent', 42, '2026-03-19T10:00:00Z', '2026-03-19T10:01:00Z')`)

	state := RecoverOnStartup(context.Background(), db, logger)
	if state.TickCount != 42 {
		t.Errorf("expected tick count 42, got %d", state.TickCount)
	}
	if state.LastReflection.IsZero() {
		t.Error("expected non-zero last reflection time")
	}
}

func TestRecoverOnStartup_FailsStuckTasks(t *testing.T) {
	db := testRecoveryDB(t)
	logger := slog.Default()

	// Insert stuck task (claimed, lease expired 10 min ago).
	expired := time.Now().UTC().Add(-10 * time.Minute).Format("2006-01-02T15:04:05Z")
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	db.Exec(`INSERT INTO tasks (id, type, status, description, lease_owner, lease_expires_at, created_at, updated_at)
		VALUES ('stuck1', 'general', 'running', 'stuck task', 'engine', ?, ?, ?)`,
		expired, now, now)

	// Insert active task (claimed, lease not expired).
	future := time.Now().UTC().Add(5 * time.Minute).Format("2006-01-02T15:04:05Z")
	db.Exec(`INSERT INTO tasks (id, type, status, description, lease_owner, lease_expires_at, created_at, updated_at)
		VALUES ('active1', 'general', 'running', 'active task', 'engine', ?, ?, ?)`,
		future, now, now)

	RecoverOnStartup(context.Background(), db, logger)

	// Stuck task should be failed.
	var status, taskErr string
	db.QueryRow("SELECT status, error FROM tasks WHERE id = 'stuck1'").Scan(&status, &taskErr)
	if status != "failed" {
		t.Errorf("stuck task: expected 'failed', got %q", status)
	}
	if taskErr == "" {
		t.Error("stuck task: expected error message")
	}

	// Active task should still be running.
	db.QueryRow("SELECT status FROM tasks WHERE id = 'active1'").Scan(&status)
	if status != "running" {
		t.Errorf("active task: expected 'running', got %q", status)
	}
}

func TestCheckpointState(t *testing.T) {
	db := testRecoveryDB(t)
	ctx := context.Background()

	// First checkpoint.
	CheckpointState(ctx, db, 10, time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC))

	var tickCount int64
	db.QueryRow("SELECT tick_count FROM agent_loop_state WHERE id = 'agent'").Scan(&tickCount)
	if tickCount != 10 {
		t.Errorf("expected tick count 10, got %d", tickCount)
	}

	// Update checkpoint.
	CheckpointState(ctx, db, 20, time.Date(2026, 3, 19, 10, 30, 0, 0, time.UTC))

	db.QueryRow("SELECT tick_count FROM agent_loop_state WHERE id = 'agent'").Scan(&tickCount)
	if tickCount != 20 {
		t.Errorf("expected tick count 20, got %d", tickCount)
	}
}

func TestCheckpointState_NilDB(t *testing.T) {
	// Should not panic.
	CheckpointState(context.Background(), nil, 5, time.Now())
}
