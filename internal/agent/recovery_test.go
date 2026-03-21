package agent

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"
	"time"

	"github.com/avifenesh/cairn/internal/db"
	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/task"

	_ "modernc.org/sqlite"
)

// testRecoveryDB creates a lightweight DB with just the agent_loop_state table.
func testRecoveryDB(t *testing.T) *sql.DB {
	t.Helper()
	rawDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { rawDB.Close() })

	_, err = rawDB.Exec(`CREATE TABLE agent_loop_state (
		id TEXT PRIMARY KEY, tick_count INTEGER DEFAULT 0,
		last_reflection_at TEXT, updated_at TEXT NOT NULL)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	return rawDB
}

// testRecoveryWithEngine creates a full DB with migrations + task engine.
func testRecoveryWithEngine(t *testing.T) (*task.Engine, *db.DB, *eventbus.Bus) {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if err := d.Migrate(); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	store := task.NewStore(d)
	bus := eventbus.New()
	t.Cleanup(func() { bus.Close() })

	engine := task.NewEngine(store, bus, nil)
	t.Cleanup(func() { engine.Close() })

	return engine, d, bus
}

// insertStuckTask creates a task directly in the DB with the given status and retry config.
func insertStuckTask(t *testing.T, rawDB *sql.DB, id, status string, retries, maxRetries int, leaseExpiresAt string) {
	t.Helper()
	insertStuckTaskTyped(t, rawDB, id, "general", status, retries, maxRetries, leaseExpiresAt)
}

// insertStuckTaskTyped creates a task with explicit type.
func insertStuckTaskTyped(t *testing.T, rawDB *sql.DB, id, taskType, status string, retries, maxRetries int, leaseExpiresAt string) {
	t.Helper()
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	metadata := `{"retries":` + itoa(retries) + `,"max_retries":` + itoa(maxRetries) + `}`
	_, err := rawDB.Exec(`
		INSERT INTO tasks (id, type, status, description, input, priority,
			created_at, lease_owner, lease_expires_at, metadata)
		VALUES (?, ?, ?, 'stuck task', '{}', 0, ?, 'engine', ?, ?)`,
		id, taskType, status, now, leaseExpiresAt, metadata)
	if err != nil {
		t.Fatalf("insert stuck task %s: %v", id, err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func getTaskStatus(t *testing.T, rawDB *sql.DB, id string) string {
	t.Helper()
	var status string
	err := rawDB.QueryRow("SELECT status FROM tasks WHERE id = ?", id).Scan(&status)
	if err != nil {
		t.Fatalf("get task status %s: %v", id, err)
	}
	return status
}

// --- Loop state tests (RecoverLoopState) ---

func TestRecoverLoopState_NoState(t *testing.T) {
	rawDB := testRecoveryDB(t)

	state, err := RecoverLoopState(context.Background(), rawDB, slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.TickCount != 0 {
		t.Errorf("expected tick count 0, got %d", state.TickCount)
	}
	if !state.LastReflection.IsZero() {
		t.Errorf("expected zero time, got %v", state.LastReflection)
	}
}

func TestRecoverLoopState_RestoresState(t *testing.T) {
	rawDB := testRecoveryDB(t)

	rawDB.Exec(`INSERT INTO agent_loop_state (id, tick_count, last_reflection_at, updated_at)
		VALUES ('agent', 42, '2026-03-19T10:00:00Z', '2026-03-19T10:01:00Z')`)

	state, _ := RecoverLoopState(context.Background(), rawDB, slog.Default())
	if state.TickCount != 42 {
		t.Errorf("expected tick count 42, got %d", state.TickCount)
	}
	if state.LastReflection.IsZero() {
		t.Error("expected non-zero last reflection time")
	}
}

func TestRecoverLoopState_NilDB(t *testing.T) {
	state, err := RecoverLoopState(context.Background(), nil, slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.TickCount != 0 {
		t.Errorf("expected tick count 0, got %d", state.TickCount)
	}
}

// --- Task recovery tests (RecoverOnStartup) ---

func TestRecoverOnStartup_NoTasks(t *testing.T) {
	engine, _, _ := testRecoveryWithEngine(t)

	stats := RecoverOnStartup(context.Background(), RecoveryDeps{
		TaskEngine: engine,
		Logger:     slog.Default(),
	})
	if stats.Total != 0 {
		t.Errorf("expected 0 recovered, got %d", stats.Total)
	}
}

func TestRecoverOnStartup_NilEngine(t *testing.T) {
	stats := RecoverOnStartup(context.Background(), RecoveryDeps{
		Logger: slog.Default(),
	})
	if stats.Total != 0 {
		t.Errorf("expected 0 recovered, got %d", stats.Total)
	}
}

func TestRecoverOnStartup_RequeuesRetryable(t *testing.T) {
	engine, d, _ := testRecoveryWithEngine(t)

	// Task with retries=0, max_retries=2: should be re-queued (0+1 < 2).
	insertStuckTask(t, d.DB, "retry1", "running", 0, 2, "2099-01-01T00:00:00.000Z")

	stats := RecoverOnStartup(context.Background(), RecoveryDeps{
		TaskEngine: engine,
		Logger:     slog.Default(),
	})

	status := getTaskStatus(t, d.DB, "retry1")
	if status != "queued" {
		t.Errorf("expected 'queued', got %q", status)
	}
	if len(stats.Requeued) != 1 || stats.Requeued[0] != "retry1" {
		t.Errorf("expected requeued=[retry1], got %v", stats.Requeued)
	}
}

func TestRecoverOnStartup_FailsExhaustedRetries(t *testing.T) {
	engine, d, _ := testRecoveryWithEngine(t)

	// Task with retries=1, max_retries=2: should be failed (1+1 >= 2).
	insertStuckTask(t, d.DB, "exhausted1", "running", 1, 2, "2099-01-01T00:00:00.000Z")

	stats := RecoverOnStartup(context.Background(), RecoveryDeps{
		TaskEngine: engine,
		Logger:     slog.Default(),
	})

	status := getTaskStatus(t, d.DB, "exhausted1")
	if status != "failed" {
		t.Errorf("expected 'failed', got %q", status)
	}
	if len(stats.Failed) != 1 || stats.Failed[0] != "exhausted1" {
		t.Errorf("expected failed=[exhausted1], got %v", stats.Failed)
	}
}

func TestRecoverOnStartup_ActiveLeaseRecovered(t *testing.T) {
	engine, d, _ := testRecoveryWithEngine(t)

	// Task with active (future) lease - the zombie case. Should still be recovered.
	future := time.Now().UTC().Add(5 * time.Minute).Format("2006-01-02T15:04:05.000Z")
	insertStuckTask(t, d.DB, "zombie1", "running", 0, 2, future)

	stats := RecoverOnStartup(context.Background(), RecoveryDeps{
		TaskEngine: engine,
		Logger:     slog.Default(),
	})

	status := getTaskStatus(t, d.DB, "zombie1")
	if status != "queued" {
		t.Errorf("expected 'queued' (re-queued despite active lease), got %q", status)
	}
	if stats.Total != 1 {
		t.Errorf("expected 1 total recovered, got %d", stats.Total)
	}
}

func TestRecoverOnStartup_RecoveryStats(t *testing.T) {
	engine, d, _ := testRecoveryWithEngine(t)

	// Mix of retryable and exhausted tasks.
	insertStuckTask(t, d.DB, "r1", "running", 0, 2, "2099-01-01T00:00:00.000Z")
	insertStuckTask(t, d.DB, "r2", "claimed", 0, 3, "2099-01-01T00:00:00.000Z")
	insertStuckTask(t, d.DB, "f1", "running", 1, 2, "2020-01-01T00:00:00.000Z")
	insertStuckTask(t, d.DB, "f2", "running", 2, 2, "2020-01-01T00:00:00.000Z")

	stats := RecoverOnStartup(context.Background(), RecoveryDeps{
		TaskEngine: engine,
		Logger:     slog.Default(),
	})

	if stats.Total != 4 {
		t.Errorf("expected 4 total, got %d", stats.Total)
	}
	if len(stats.Requeued) != 2 {
		t.Errorf("expected 2 requeued, got %d: %v", len(stats.Requeued), stats.Requeued)
	}
	if len(stats.Failed) != 2 {
		t.Errorf("expected 2 failed, got %d: %v", len(stats.Failed), stats.Failed)
	}
}

func TestRecoverOnStartup_RecordsActivity(t *testing.T) {
	engine, d, _ := testRecoveryWithEngine(t)
	activityStore := NewActivityStore(d.DB)

	insertStuckTask(t, d.DB, "act1", "running", 0, 2, "2099-01-01T00:00:00.000Z")

	RecoverOnStartup(context.Background(), RecoveryDeps{
		TaskEngine:    engine,
		ActivityStore: activityStore,
		Logger:        slog.Default(),
	})

	entries, err := activityStore.List(context.Background(), 10, 0, "recovery")
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 activity entry, got %d", len(entries))
	}
	if entries[0].Type != "recovery" {
		t.Errorf("expected type 'recovery', got %q", entries[0].Type)
	}
}

func TestRecoverOnStartup_PublishesTaskFailedEvent(t *testing.T) {
	engine, d, bus := testRecoveryWithEngine(t)

	insertStuckTask(t, d.DB, "ev1", "running", 1, 2, "2099-01-01T00:00:00.000Z")

	received := make(chan eventbus.TaskFailed, 1)
	eventbus.Subscribe(bus, func(e eventbus.TaskFailed) {
		received <- e
	})

	RecoverOnStartup(context.Background(), RecoveryDeps{
		TaskEngine: engine,
		Logger:     slog.Default(),
	})

	select {
	case ev := <-received:
		if ev.TaskID != "ev1" {
			t.Errorf("expected task ID 'ev1', got %q", ev.TaskID)
		}
	case <-time.After(2 * time.Second):
		t.Error("expected TaskFailed event, but none received")
	}
}

func TestRecoverOnStartup_ChatTaskNeverRetried(t *testing.T) {
	engine, d, _ := testRecoveryWithEngine(t)

	// Chat task with retries remaining: should still be failed (not re-queued).
	insertStuckTaskTyped(t, d.DB, "chat1", "chat", "running", 0, 3, "2099-01-01T00:00:00.000Z")
	// General task with same config: should be re-queued.
	insertStuckTask(t, d.DB, "gen1", "running", 0, 3, "2099-01-01T00:00:00.000Z")

	stats := RecoverOnStartup(context.Background(), RecoveryDeps{
		TaskEngine: engine,
		Logger:     slog.Default(),
	})

	chatStatus := getTaskStatus(t, d.DB, "chat1")
	if chatStatus != "failed" {
		t.Errorf("chat task: expected 'failed', got %q (chat tasks must not be retried)", chatStatus)
	}

	genStatus := getTaskStatus(t, d.DB, "gen1")
	if genStatus != "queued" {
		t.Errorf("general task: expected 'queued', got %q", genStatus)
	}

	if len(stats.Requeued) != 1 {
		t.Errorf("expected 1 requeued (general only), got %d: %v", len(stats.Requeued), stats.Requeued)
	}
	if len(stats.Failed) != 1 {
		t.Errorf("expected 1 failed (chat only), got %d: %v", len(stats.Failed), stats.Failed)
	}
}

// --- Checkpoint tests ---

func TestCheckpointState(t *testing.T) {
	rawDB := testRecoveryDB(t)
	ctx := context.Background()

	CheckpointState(ctx, rawDB, 10, time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC))

	var tickCount int64
	rawDB.QueryRow("SELECT tick_count FROM agent_loop_state WHERE id = 'agent'").Scan(&tickCount)
	if tickCount != 10 {
		t.Errorf("expected tick count 10, got %d", tickCount)
	}

	CheckpointState(ctx, rawDB, 20, time.Date(2026, 3, 19, 10, 30, 0, 0, time.UTC))

	rawDB.QueryRow("SELECT tick_count FROM agent_loop_state WHERE id = 'agent'").Scan(&tickCount)
	if tickCount != 20 {
		t.Errorf("expected tick count 20, got %d", tickCount)
	}
}

func TestCheckpointState_NilDB(t *testing.T) {
	CheckpointState(context.Background(), nil, 5, time.Now())
}
