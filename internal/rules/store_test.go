package rules

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	// Enable foreign keys for cascade delete.
	db.Exec("PRAGMA foreign_keys = ON")

	// Apply schema.
	for _, stmt := range []string{
		`CREATE TABLE rules (
			id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, description TEXT DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1, trigger TEXT NOT NULL,
			condition TEXT DEFAULT '', actions TEXT NOT NULL DEFAULT '[]',
			throttle_ms INTEGER DEFAULT 0, created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL, last_fired_at TEXT)`,
		`CREATE TABLE rule_executions (
			id TEXT PRIMARY KEY, rule_id TEXT NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
			trigger_event TEXT, status TEXT NOT NULL, error TEXT,
			duration_ms INTEGER, created_at TEXT NOT NULL)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestStore_CRUD(t *testing.T) {
	store := NewStore(testDB(t))
	ctx := context.Background()

	// Create.
	rule := &Rule{
		Name:        "test-rule",
		Description: "A test rule",
		Enabled:     true,
		Trigger: Trigger{
			Type:      TriggerEvent,
			EventType: "EventIngested",
			Filter:    map[string]string{"sourceType": "github"},
		},
		Condition: `sourceType == "github"`,
		Actions: []Action{
			{Type: ActionNotify, Params: map[string]string{"message": "test", "priority": "1"}},
		},
		ThrottleMs: 5000,
	}
	if err := store.Create(ctx, rule); err != nil {
		t.Fatalf("create: %v", err)
	}
	if rule.ID == "" {
		t.Fatal("expected generated ID")
	}

	// Get.
	got, err := store.Get(ctx, rule.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "test-rule" {
		t.Errorf("expected name 'test-rule', got %q", got.Name)
	}
	if got.Trigger.EventType != "EventIngested" {
		t.Errorf("expected trigger EventIngested, got %q", got.Trigger.EventType)
	}
	if len(got.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(got.Actions))
	}
	if got.Actions[0].Type != ActionNotify {
		t.Errorf("expected notify action, got %q", got.Actions[0].Type)
	}

	// GetByName.
	got, err = store.GetByName(ctx, "test-rule")
	if err != nil {
		t.Fatalf("getByName: %v", err)
	}
	if got.ID != rule.ID {
		t.Errorf("expected ID %q, got %q", rule.ID, got.ID)
	}

	// List.
	rules, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	// Update.
	newName := "renamed-rule"
	enabled := false
	if err := store.Update(ctx, rule.ID, UpdateOpts{Name: &newName, Enabled: &enabled}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = store.Get(ctx, rule.ID)
	if got.Name != "renamed-rule" {
		t.Errorf("expected 'renamed-rule', got %q", got.Name)
	}
	if got.Enabled {
		t.Error("expected disabled")
	}

	// ListEnabled should be empty.
	enabled_rules, _ := store.ListEnabled(ctx)
	if len(enabled_rules) != 0 {
		t.Errorf("expected 0 enabled rules, got %d", len(enabled_rules))
	}

	// Delete.
	if err := store.Delete(ctx, rule.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	rules, _ = store.List(ctx)
	if len(rules) != 0 {
		t.Errorf("expected 0 rules after delete, got %d", len(rules))
	}
}

func TestStore_Executions(t *testing.T) {
	store := NewStore(testDB(t))
	ctx := context.Background()

	rule := &Rule{
		Name:    "exec-test",
		Enabled: true,
		Trigger: Trigger{Type: TriggerEvent, EventType: "EventIngested"},
		Actions: []Action{{Type: ActionNotify, Params: map[string]string{"message": "hi"}}},
	}
	store.Create(ctx, rule)

	// Record executions.
	for _, status := range []ExecutionStatus{ExecSuccess, ExecError, ExecThrottled} {
		exec := &Execution{
			RuleID: rule.ID,
			Status: status,
		}
		if status == ExecError {
			exec.Error = "something went wrong"
		}
		if err := store.RecordExecution(ctx, exec); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	// List by rule.
	execs, err := store.ListExecutions(ctx, rule.ID, 10)
	if err != nil {
		t.Fatalf("listExecutions: %v", err)
	}
	if len(execs) != 3 {
		t.Fatalf("expected 3 executions, got %d", len(execs))
	}

	// List recent.
	recent, err := store.ListRecentExecutions(ctx, 10)
	if err != nil {
		t.Fatalf("listRecent: %v", err)
	}
	if len(recent) != 3 {
		t.Fatalf("expected 3 recent, got %d", len(recent))
	}

	// Delete rule cascades.
	store.Delete(ctx, rule.ID)
	execs, _ = store.ListExecutions(ctx, rule.ID, 10)
	if len(execs) != 0 {
		t.Errorf("expected cascade delete, got %d executions", len(execs))
	}
}

func TestStore_DuplicateName(t *testing.T) {
	store := NewStore(testDB(t))
	ctx := context.Background()

	rule1 := &Rule{
		Name:    "unique-name",
		Enabled: true,
		Trigger: Trigger{Type: TriggerEvent, EventType: "EventIngested"},
		Actions: []Action{{Type: ActionNotify, Params: map[string]string{"message": "1"}}},
	}
	store.Create(ctx, rule1)

	rule2 := &Rule{
		Name:    "unique-name",
		Enabled: true,
		Trigger: Trigger{Type: TriggerEvent, EventType: "EventIngested"},
		Actions: []Action{{Type: ActionNotify, Params: map[string]string{"message": "2"}}},
	}
	if err := store.Create(ctx, rule2); err == nil {
		t.Fatal("expected error on duplicate name")
	}
}

func TestStore_UpdateAllFields(t *testing.T) {
	store := NewStore(testDB(t))
	ctx := context.Background()

	rule := &Rule{
		Name:    "original",
		Enabled: true,
		Trigger: Trigger{Type: TriggerEvent, EventType: "EventIngested"},
		Actions: []Action{{Type: ActionNotify, Params: map[string]string{"message": "old"}}},
	}
	store.Create(ctx, rule)

	newName := "updated-name"
	newDesc := "updated description"
	newEnabled := false
	newCondition := `title contains "important"`
	newTrigger := Trigger{Type: TriggerEvent, EventType: "TaskFailed", Filter: map[string]string{"source": "agent"}}
	newActions := []Action{
		{Type: ActionTask, Params: map[string]string{"description": "follow up"}},
		{Type: ActionNotify, Params: map[string]string{"message": "alert"}},
	}
	var newThrottle int64 = 30000

	err := store.Update(ctx, rule.ID, UpdateOpts{
		Name:        &newName,
		Description: &newDesc,
		Enabled:     &newEnabled,
		Condition:   &newCondition,
		Trigger:     &newTrigger,
		Actions:     newActions,
		ThrottleMs:  &newThrottle,
	})
	if err != nil {
		t.Fatalf("update all fields: %v", err)
	}

	got, err := store.Get(ctx, rule.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.Name != newName {
		t.Errorf("name: want %q, got %q", newName, got.Name)
	}
	if got.Description != newDesc {
		t.Errorf("description: want %q, got %q", newDesc, got.Description)
	}
	if got.Enabled != newEnabled {
		t.Errorf("enabled: want %v, got %v", newEnabled, got.Enabled)
	}
	if got.Condition != newCondition {
		t.Errorf("condition: want %q, got %q", newCondition, got.Condition)
	}
	if got.Trigger.EventType != "TaskFailed" {
		t.Errorf("trigger.eventType: want %q, got %q", "TaskFailed", got.Trigger.EventType)
	}
	if got.Trigger.Filter["source"] != "agent" {
		t.Errorf("trigger.filter[source]: want %q, got %q", "agent", got.Trigger.Filter["source"])
	}
	if len(got.Actions) != 2 {
		t.Fatalf("actions: want 2, got %d", len(got.Actions))
	}
	if got.Actions[0].Type != ActionTask {
		t.Errorf("actions[0].type: want %q, got %q", ActionTask, got.Actions[0].Type)
	}
	if got.Actions[1].Type != ActionNotify {
		t.Errorf("actions[1].type: want %q, got %q", ActionNotify, got.Actions[1].Type)
	}
	if got.ThrottleMs != newThrottle {
		t.Errorf("throttleMs: want %d, got %d", newThrottle, got.ThrottleMs)
	}
}

func TestStore_UpdateLastFired(t *testing.T) {
	store := NewStore(testDB(t))
	ctx := context.Background()

	rule := &Rule{
		Name:    "fire-test",
		Enabled: true,
		Trigger: Trigger{Type: TriggerEvent, EventType: "EventIngested"},
		Actions: []Action{{Type: ActionNotify, Params: map[string]string{"message": "x"}}},
	}
	store.Create(ctx, rule)

	firedAt := time.Now().UTC().Truncate(time.Millisecond)
	if err := store.UpdateLastFired(ctx, rule.ID, firedAt); err != nil {
		t.Fatalf("updateLastFired: %v", err)
	}

	got, err := store.Get(ctx, rule.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.LastFiredAt == nil {
		t.Fatal("expected LastFiredAt to be set")
	}
	// Compare formatted strings since SQLite stores with millisecond precision.
	if got.LastFiredAt.Format(timeFormat) != firedAt.Format(timeFormat) {
		t.Errorf("lastFiredAt: want %s, got %s", firedAt.Format(timeFormat), got.LastFiredAt.Format(timeFormat))
	}
}

func TestStore_GetNotFound(t *testing.T) {
	store := NewStore(testDB(t))
	ctx := context.Background()

	_, err := store.Get(ctx, "bogus-id-does-not-exist")
	if err == nil {
		t.Fatal("expected error for missing rule")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got %q", err.Error())
	}
}

func TestStore_GetByNameNotFound(t *testing.T) {
	store := NewStore(testDB(t))
	ctx := context.Background()

	_, err := store.GetByName(ctx, "nonexistent-rule-name")
	if err == nil {
		t.Fatal("expected error for missing rule name")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got %q", err.Error())
	}
}

func TestStore_DeleteNotFound(t *testing.T) {
	store := NewStore(testDB(t))
	ctx := context.Background()

	err := store.Delete(ctx, "bogus-id-does-not-exist")
	if err == nil {
		t.Fatal("expected error deleting non-existent rule")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got %q", err.Error())
	}
}

func TestStore_ListRecentLimit(t *testing.T) {
	store := NewStore(testDB(t))
	ctx := context.Background()

	rule := &Rule{
		Name:    "limit-test",
		Enabled: true,
		Trigger: Trigger{Type: TriggerEvent, EventType: "EventIngested"},
		Actions: []Action{{Type: ActionNotify, Params: map[string]string{"message": "x"}}},
	}
	store.Create(ctx, rule)

	// Create 5 executions with staggered timestamps so ordering is deterministic.
	for i := 0; i < 5; i++ {
		exec := &Execution{
			RuleID:    rule.ID,
			Status:    ExecSuccess,
			CreatedAt: time.Now().UTC().Add(time.Duration(i) * time.Second),
		}
		if err := store.RecordExecution(ctx, exec); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}

	recent, err := store.ListRecentExecutions(ctx, 2)
	if err != nil {
		t.Fatalf("listRecent: %v", err)
	}
	if len(recent) != 2 {
		t.Errorf("expected 2 executions with limit=2, got %d", len(recent))
	}
}
