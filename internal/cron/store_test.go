package cron

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Create tables.
	for _, ddl := range []string{
		`CREATE TABLE cron_jobs (
			id TEXT PRIMARY KEY, enabled INTEGER NOT NULL DEFAULT 1,
			name TEXT NOT NULL UNIQUE, description TEXT DEFAULT '',
			schedule TEXT NOT NULL, instruction TEXT NOT NULL,
			timezone TEXT DEFAULT 'UTC', priority INTEGER DEFAULT 3,
			cooldown_ms INTEGER DEFAULT 3600000,
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
			last_run_at TEXT, next_run_at TEXT)`,
		`CREATE TABLE cron_executions (
			id TEXT PRIMARY KEY, cron_job_id TEXT NOT NULL,
			task_id TEXT, status TEXT NOT NULL, error TEXT,
			created_at TEXT NOT NULL)`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("create table: %v", err)
		}
	}
	return db
}

func TestStore_CreateAndGet(t *testing.T) {
	store := NewStore(testDB(t))
	ctx := context.Background()

	job := &CronJob{
		Enabled:     true,
		Name:        "morning-email",
		Description: "Check email at 9am",
		Schedule:    "0 9 * * *",
		Instruction: "Check my email and summarize the top 5 unread messages",
		Timezone:    "America/New_York",
		Priority:    2,
		CooldownMs:  3600000,
	}

	if err := store.Create(ctx, job); err != nil {
		t.Fatalf("create: %v", err)
	}
	if job.ID == "" {
		t.Fatal("expected ID to be set")
	}
	if job.NextRunAt == nil {
		t.Fatal("expected next_run_at to be computed")
	}

	got, err := store.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "morning-email" {
		t.Errorf("name: got %q, want %q", got.Name, "morning-email")
	}
	if got.Schedule != "0 9 * * *" {
		t.Errorf("schedule: got %q, want %q", got.Schedule, "0 9 * * *")
	}
	if !got.Enabled {
		t.Error("expected enabled")
	}
}

func TestStore_CreateInvalidSchedule(t *testing.T) {
	store := NewStore(testDB(t))
	ctx := context.Background()

	job := &CronJob{
		Enabled:     true,
		Name:        "bad-cron",
		Schedule:    "not valid",
		Instruction: "do something",
	}
	if err := store.Create(ctx, job); err == nil {
		t.Fatal("expected error for invalid schedule")
	}
}

func TestStore_List(t *testing.T) {
	store := NewStore(testDB(t))
	ctx := context.Background()

	for _, name := range []string{"alpha", "beta", "gamma"} {
		if err := store.Create(ctx, &CronJob{
			Enabled: true, Name: name, Schedule: "* * * * *", Instruction: "test",
		}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	jobs, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}
	// Ordered by name.
	if jobs[0].Name != "alpha" || jobs[1].Name != "beta" || jobs[2].Name != "gamma" {
		t.Errorf("unexpected order: %s, %s, %s", jobs[0].Name, jobs[1].Name, jobs[2].Name)
	}
}

func TestStore_Delete(t *testing.T) {
	store := NewStore(testDB(t))
	ctx := context.Background()

	job := &CronJob{Enabled: true, Name: "to-delete", Schedule: "* * * * *", Instruction: "test"}
	if err := store.Create(ctx, job); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := store.Delete(ctx, job.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := store.Get(ctx, job.ID)
	if err != sql.ErrNoRows {
		t.Fatalf("expected ErrNoRows after delete, got: %v", err)
	}
}

func TestStore_GetDueJobs(t *testing.T) {
	store := NewStore(testDB(t))
	ctx := context.Background()

	// Job with next_run in the past → due.
	pastNext := time.Now().UTC().Add(-5 * time.Minute)
	job1 := &CronJob{Enabled: true, Name: "due-job", Schedule: "* * * * *", Instruction: "test"}
	if err := store.Create(ctx, job1); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Override next_run to past.
	store.db.ExecContext(ctx, "UPDATE cron_jobs SET next_run_at = ? WHERE id = ?",
		pastNext.Format(timeFormat), job1.ID)

	// Job with next_run in the future → not due.
	futureNext := time.Now().UTC().Add(1 * time.Hour)
	job2 := &CronJob{Enabled: true, Name: "future-job", Schedule: "0 9 * * *", Instruction: "test"}
	if err := store.Create(ctx, job2); err != nil {
		t.Fatalf("create: %v", err)
	}
	store.db.ExecContext(ctx, "UPDATE cron_jobs SET next_run_at = ? WHERE id = ?",
		futureNext.Format(timeFormat), job2.ID)

	// Disabled job → not due even if past.
	job3 := &CronJob{Enabled: false, Name: "disabled-job", Schedule: "* * * * *", Instruction: "test"}
	store.Create(ctx, job3)
	store.db.ExecContext(ctx, "UPDATE cron_jobs SET next_run_at = ? WHERE id = ?",
		pastNext.Format(timeFormat), job3.ID)

	due, err := store.GetDueJobs(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("getDueJobs: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("expected 1 due job, got %d", len(due))
	}
	if due[0].Name != "due-job" {
		t.Errorf("expected due-job, got %s", due[0].Name)
	}
}

func TestStore_GetDueJobs_CooldownRespected(t *testing.T) {
	store := NewStore(testDB(t))
	ctx := context.Background()

	// Job that ran 30 minutes ago with 1h cooldown → not due.
	job := &CronJob{
		Enabled: true, Name: "cooled", Schedule: "* * * * *",
		Instruction: "test", CooldownMs: 3600000, // 1h
	}
	if err := store.Create(ctx, job); err != nil {
		t.Fatalf("create: %v", err)
	}
	lastRun := time.Now().UTC().Add(-30 * time.Minute)
	pastNext := time.Now().UTC().Add(-5 * time.Minute)
	store.db.ExecContext(ctx, "UPDATE cron_jobs SET last_run_at = ?, next_run_at = ? WHERE id = ?",
		lastRun.Format(timeFormat), pastNext.Format(timeFormat), job.ID)

	due, err := store.GetDueJobs(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("getDueJobs: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("expected 0 due jobs (cooldown active), got %d", len(due))
	}
}

func TestStore_RecordExecution(t *testing.T) {
	store := NewStore(testDB(t))
	ctx := context.Background()

	job := &CronJob{Enabled: true, Name: "exec-test", Schedule: "* * * * *", Instruction: "test"}
	store.Create(ctx, job)

	if err := store.RecordExecution(ctx, job.ID, "task_123", "fired", nil); err != nil {
		t.Fatalf("record: %v", err)
	}

	execs, err := store.ListExecutions(ctx, job.ID, 10)
	if err != nil {
		t.Fatalf("listExecutions: %v", err)
	}
	if len(execs) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(execs))
	}
	if execs[0].Status != "fired" {
		t.Errorf("status: got %q, want %q", execs[0].Status, "fired")
	}
	if execs[0].TaskID != "task_123" {
		t.Errorf("taskID: got %q, want %q", execs[0].TaskID, "task_123")
	}
}

func TestStore_GetByName(t *testing.T) {
	store := NewStore(testDB(t))
	ctx := context.Background()

	job := &CronJob{Enabled: true, Name: "by-name", Schedule: "0 9 * * *", Instruction: "test"}
	store.Create(ctx, job)

	got, err := store.GetByName(ctx, "by-name")
	if err != nil {
		t.Fatalf("getByName: %v", err)
	}
	if got.ID != job.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, job.ID)
	}
}

func TestStore_Update(t *testing.T) {
	store := NewStore(testDB(t))
	ctx := context.Background()

	job := &CronJob{Enabled: true, Name: "updatable", Schedule: "0 9 * * *", Instruction: "old instruction"}
	store.Create(ctx, job)

	newInstruction := "new instruction"
	disabled := false
	if err := store.Update(ctx, job.ID, &disabled, nil, &newInstruction, nil, nil, nil); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := store.Get(ctx, job.ID)
	if got.Instruction != "new instruction" {
		t.Errorf("instruction: got %q, want %q", got.Instruction, "new instruction")
	}
	if got.Enabled {
		t.Error("expected disabled")
	}

	// Update cooldownMs.
	newCooldown := int64(60000)
	if err := store.Update(ctx, job.ID, nil, nil, nil, nil, nil, &newCooldown); err != nil {
		t.Fatalf("update cooldown: %v", err)
	}
	got, _ = store.Get(ctx, job.ID)
	if got.CooldownMs != 60000 {
		t.Errorf("cooldownMs: got %d, want 60000", got.CooldownMs)
	}

	// Nil cooldownMs should leave it unchanged.
	if err := store.Update(ctx, job.ID, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("update nil cooldown: %v", err)
	}
	got, _ = store.Get(ctx, job.ID)
	if got.CooldownMs != 60000 {
		t.Errorf("cooldownMs after nil update: got %d, want 60000", got.CooldownMs)
	}
}
