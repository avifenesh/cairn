package task

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/avifenesh/cairn/internal/db"
)

// openTestDB creates an in-memory database with migrations applied.
func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if err := d.Migrate(); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestStore_CreateAndGet(t *testing.T) {
	d := openTestDB(t)
	s := NewStore(d)
	ctx := context.Background()

	task := &Task{
		ID:       newID(),
		Type:     TypeChat,
		Status:   StatusQueued,
		Priority: PriorityNormal,
		Mode:     "talk",
		Input:    json.RawMessage(`{"prompt":"hello"}`),
	}

	if err := s.Create(ctx, task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil")
	}

	if got.ID != task.ID {
		t.Errorf("ID: got %q, want %q", got.ID, task.ID)
	}
	if got.Type != TypeChat {
		t.Errorf("Type: got %q, want %q", got.Type, TypeChat)
	}
	if got.Status != StatusQueued {
		t.Errorf("Status: got %q, want %q", got.Status, StatusQueued)
	}
	if got.Priority != PriorityNormal {
		t.Errorf("Priority: got %d, want %d", got.Priority, PriorityNormal)
	}
	if got.Mode != "talk" {
		t.Errorf("Mode: got %q, want %q", got.Mode, "talk")
	}
	if string(got.Input) != `{"prompt":"hello"}` {
		t.Errorf("Input: got %s, want %s", got.Input, `{"prompt":"hello"}`)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if !got.StartedAt.IsZero() {
		t.Error("StartedAt should be zero for a freshly queued task")
	}
	if !got.CompletedAt.IsZero() {
		t.Error("CompletedAt should be zero for a freshly queued task")
	}
}

func TestStore_List(t *testing.T) {
	d := openTestDB(t)
	s := NewStore(d)
	ctx := context.Background()

	// Create 5 tasks with different types and priorities.
	types := []TaskType{TypeChat, TypeCoding, TypeDigest, TypeChat, TypeTriage}
	priorities := []Priority{PriorityLow, PriorityHigh, PriorityNormal, PriorityCritical, PriorityIdle}

	for i := 0; i < 5; i++ {
		task := &Task{
			ID:       newID(),
			Type:     types[i],
			Status:   StatusQueued,
			Priority: priorities[i],
			Input:    json.RawMessage(`{}`),
		}
		if err := s.Create(ctx, task); err != nil {
			t.Fatalf("Create task %d: %v", i, err)
		}
		// Small delay so created_at ordering is deterministic.
		time.Sleep(5 * time.Millisecond)
	}

	// List all.
	all, err := s.List(ctx, ListOpts{})
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("List all: got %d tasks, want 5", len(all))
	}

	// List by type.
	chats, err := s.List(ctx, ListOpts{Type: TypeChat})
	if err != nil {
		t.Fatalf("List chat: %v", err)
	}
	if len(chats) != 2 {
		t.Errorf("List chat: got %d tasks, want 2", len(chats))
	}

	// List with limit.
	limited, err := s.List(ctx, ListOpts{Limit: 3})
	if err != nil {
		t.Fatalf("List limited: %v", err)
	}
	if len(limited) != 3 {
		t.Errorf("List limited: got %d tasks, want 3", len(limited))
	}

	// Verify ordering: priority ASC, then created_at ASC.
	for i := 1; i < len(all); i++ {
		if all[i-1].Priority > all[i].Priority {
			t.Errorf("List order: task[%d].Priority=%d > task[%d].Priority=%d",
				i-1, all[i-1].Priority, i, all[i].Priority)
		}
	}
}

func TestStore_Claim(t *testing.T) {
	d := openTestDB(t)
	s := NewStore(d)
	ctx := context.Background()

	// Create 3 queued chat tasks with different priorities.
	ids := make([]string, 3)
	priorities := []Priority{PriorityLow, PriorityCritical, PriorityNormal}
	for i := 0; i < 3; i++ {
		ids[i] = newID()
		task := &Task{
			ID:         ids[i],
			Type:       TypeChat,
			Status:     StatusQueued,
			Priority:   priorities[i],
			MaxRetries: 2,
			Input:      json.RawMessage(`{}`),
		}
		if err := s.Create(ctx, task); err != nil {
			t.Fatalf("Create task %d: %v", i, err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Claim should pick the critical priority task first.
	claimed, err := s.Claim(ctx, TypeChat, "worker-1", 5*time.Minute)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if claimed == nil {
		t.Fatal("Claim returned nil")
	}
	if claimed.ID != ids[1] {
		t.Errorf("Claim picked %q, want %q (critical priority)", claimed.ID, ids[1])
	}
	if claimed.Status != StatusClaimed {
		t.Errorf("Claimed status: got %q, want %q", claimed.Status, StatusClaimed)
	}
	if claimed.LeaseOwner != "worker-1" {
		t.Errorf("LeaseOwner: got %q, want %q", claimed.LeaseOwner, "worker-1")
	}

	// Second claim should pick normal priority.
	claimed2, err := s.Claim(ctx, TypeChat, "worker-2", 5*time.Minute)
	if err != nil {
		t.Fatalf("Second Claim: %v", err)
	}
	if claimed2 == nil {
		t.Fatal("Second Claim returned nil")
	}
	if claimed2.ID != ids[2] {
		t.Errorf("Second Claim picked %q, want %q (normal priority)", claimed2.ID, ids[2])
	}

	// Claim a different type should return nil.
	none, err := s.Claim(ctx, TypeCoding, "worker-3", 5*time.Minute)
	if err != nil {
		t.Fatalf("Claim coding: %v", err)
	}
	if none != nil {
		t.Errorf("Claim coding: expected nil, got task %q", none.ID)
	}
}

func TestStore_ClaimAnyType(t *testing.T) {
	d := openTestDB(t)
	s := NewStore(d)
	ctx := context.Background()

	// Create tasks of different types (CreatedAt is set by store, not the struct).
	cronTask := &Task{
		ID:         newID(),
		Type:       "cron",
		Status:     StatusQueued,
		Priority:   PriorityNormal,
		MaxRetries: 2,
		Input:      json.RawMessage(`{"instruction":"test cron"}`),
	}
	generalTask := &Task{
		ID:         newID(),
		Type:       TypeGeneral,
		Status:     StatusQueued,
		Priority:   PriorityHigh,
		MaxRetries: 2,
		Input:      json.RawMessage(`{"description":"test general"}`),
	}
	if err := s.Create(ctx, cronTask); err != nil {
		t.Fatalf("Create cron: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	if err := s.Create(ctx, generalTask); err != nil {
		t.Fatalf("Create general: %v", err)
	}

	// Claim with empty type should pick the highest-priority task (general = PriorityHigh).
	claimed, err := s.Claim(ctx, "", "worker-any", 5*time.Minute)
	if err != nil {
		t.Fatalf("Claim any type: %v", err)
	}
	if claimed == nil {
		t.Fatal("Claim any type returned nil - empty type must match any queued task")
	}
	if claimed.ID != generalTask.ID {
		t.Errorf("Claim any type: got %q, want %q (higher priority)", claimed.ID, generalTask.ID)
	}
	if claimed.Type != TypeGeneral {
		t.Errorf("Claimed type: got %q, want %q", claimed.Type, TypeGeneral)
	}
	if claimed.Status != StatusClaimed {
		t.Errorf("Claimed status: got %q, want %q", claimed.Status, StatusClaimed)
	}

	// Claim again should pick the cron task.
	claimed2, err := s.Claim(ctx, "", "worker-any", 5*time.Minute)
	if err != nil {
		t.Fatalf("Second claim any type: %v", err)
	}
	if claimed2 == nil {
		t.Fatal("Second claim any type returned nil")
	}
	if claimed2.ID != cronTask.ID {
		t.Errorf("Second claim any type: got %q, want %q", claimed2.ID, cronTask.ID)
	}
	if claimed2.Type != "cron" {
		t.Errorf("Second claimed type: got %q, want %q", claimed2.Type, "cron")
	}

	// Third claim should return nil (no more queued tasks).
	claimed3, err := s.Claim(ctx, "", "worker-any", 5*time.Minute)
	if err != nil {
		t.Fatalf("Third claim any type: %v", err)
	}
	if claimed3 != nil {
		t.Errorf("Third claim any type: expected nil, got task %q", claimed3.ID)
	}
}

func TestStore_CreateWithStartedAt(t *testing.T) {
	d := openTestDB(t)
	s := NewStore(d)
	ctx := context.Background()

	now := time.Now()
	claimedTask := &Task{
		ID:          newID(),
		Type:        TypeChat,
		Status:      StatusRunning,
		Priority:    PriorityHigh,
		MaxRetries:  2,
		Input:       json.RawMessage(`{}`),
		StartedAt:   now,
		CompletedAt: now.Add(10 * time.Second),
		LeaseOwner:  "http",
		LeaseExpiry: now.Add(5 * time.Minute),
	}
	if err := s.Create(ctx, claimedTask); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get(ctx, claimedTask.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Verify StartedAt was persisted with correct value (isoTime truncates to ms).
	if got.StartedAt.IsZero() {
		t.Error("StartedAt not persisted - should be set for pre-claimed tasks")
	}
	if got.StartedAt.Sub(claimedTask.StartedAt).Abs() > time.Millisecond {
		t.Errorf("StartedAt value mismatch: got %v, want ~%v", got.StartedAt, claimedTask.StartedAt)
	}

	// Verify CompletedAt was also persisted.
	if got.CompletedAt.IsZero() {
		t.Error("CompletedAt not persisted")
	}
	if got.CompletedAt.Sub(claimedTask.CompletedAt).Abs() > time.Millisecond {
		t.Errorf("CompletedAt value mismatch: got %v, want ~%v", got.CompletedAt, claimedTask.CompletedAt)
	}

	if got.LeaseOwner != "http" {
		t.Errorf("LeaseOwner: got %q, want %q", got.LeaseOwner, "http")
	}
	if got.Status != StatusRunning {
		t.Errorf("Status: got %q, want %q", got.Status, StatusRunning)
	}
}

func TestStore_Heartbeat(t *testing.T) {
	d := openTestDB(t)
	s := NewStore(d)
	ctx := context.Background()

	task := &Task{
		ID:         newID(),
		Type:       TypeCoding,
		Status:     StatusQueued,
		Priority:   PriorityNormal,
		MaxRetries: 2,
		Input:      json.RawMessage(`{}`),
	}
	if err := s.Create(ctx, task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Claim it with a short lease.
	claimed, err := s.Claim(ctx, TypeCoding, "worker-1", 1*time.Second)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if claimed == nil {
		t.Fatal("Claim returned nil")
	}

	originalExpiry := claimed.LeaseExpiry

	// Heartbeat extends the lease.
	time.Sleep(10 * time.Millisecond)
	if err := s.Heartbeat(ctx, claimed.ID, 10*time.Minute); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}

	// Verify lease was extended.
	got, err := s.Get(ctx, claimed.ID)
	if err != nil {
		t.Fatalf("Get after heartbeat: %v", err)
	}
	if !got.LeaseExpiry.After(originalExpiry) {
		t.Errorf("Heartbeat did not extend lease: original=%v, now=%v", originalExpiry, got.LeaseExpiry)
	}
}
