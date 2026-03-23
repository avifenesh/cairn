package task

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/avifenesh/cairn/internal/db"
	"github.com/avifenesh/cairn/internal/eventbus"
)

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if err := d.Migrate(); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	store := NewStore(d)
	bus := eventbus.New()
	t.Cleanup(func() { bus.Close() })

	e := NewEngine(store, bus, nil)
	t.Cleanup(func() { e.Close() })

	return e
}

func TestEngine_SubmitAndClaim(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	task, err := e.Submit(ctx, &SubmitRequest{
		Type:        TypeChat,
		Priority:    PriorityNormal,
		Mode:        "talk",
		Input:       json.RawMessage(`{"prompt":"hello"}`),
		Description: "test chat",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if task == nil {
		t.Fatal("Submit returned nil task")
	}
	if task.Status != StatusQueued {
		t.Errorf("Submit status: got %q, want %q", task.Status, StatusQueued)
	}

	// Claim the task.
	claimed, err := e.Claim(ctx, TypeChat)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if claimed == nil {
		t.Fatal("Claim returned nil")
	}
	if claimed.ID != task.ID {
		t.Errorf("Claimed ID: got %q, want %q", claimed.ID, task.ID)
	}
	if claimed.Status != StatusClaimed {
		t.Errorf("Claimed status: got %q, want %q", claimed.Status, StatusClaimed)
	}

	// Verify via Get.
	got, err := e.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusClaimed {
		t.Errorf("Get after claim status: got %q, want %q", got.Status, StatusClaimed)
	}
}

func TestEngine_SubmitPreClaimed(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	task, err := e.Submit(ctx, &SubmitRequest{
		Type:        TypeChat,
		Priority:    PriorityNormal,
		Input:       json.RawMessage(`{"message":"hi"}`),
		Description: "test pre-claimed submit",
		ClaimOwner:  "http",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Task should be created directly in running status.
	if task.Status != StatusRunning {
		t.Errorf("expected status 'running', got %q", task.Status)
	}
	if task.LeaseOwner != "http" {
		t.Errorf("expected lease owner 'http', got %q", task.LeaseOwner)
	}

	// Verify status and StartedAt are persisted in the DB.
	got, err := e.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusRunning {
		t.Errorf("expected status 'running', got %q", got.Status)
	}
	if got.StartedAt.IsZero() {
		t.Error("StartedAt should be set for pre-claimed running task")
	}

	// Trying to claim should return nil (task was never queued).
	claimed, err := e.Claim(ctx, TypeChat)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if claimed != nil {
		t.Errorf("expected nil claim (task already running), got %s", claimed.ID)
	}
}

func TestEngine_FailTerminal(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	// Submit with retries remaining.
	task, err := e.Submit(ctx, &SubmitRequest{
		Type:        TypeGeneral,
		Priority:    PriorityNormal,
		Input:       json.RawMessage(`{}`),
		Description: "test fail terminal",
		MaxRetries:  5,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Claim it first.
	e.Claim(ctx, TypeGeneral)

	// FailTerminal should mark failed even with retries remaining.
	if err := e.FailTerminal(ctx, task.ID, errors.New("panic")); err != nil {
		t.Fatalf("FailTerminal: %v", err)
	}

	got, _ := e.Get(ctx, task.ID)
	if got.Status != StatusFailed {
		t.Errorf("expected 'failed', got %q (FailTerminal should bypass retries)", got.Status)
	}
}

func TestEngine_CompleteTask(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	task, err := e.Submit(ctx, &SubmitRequest{
		Type:     TypeCoding,
		Priority: PriorityHigh,
		Mode:     "coding",
		Input:    json.RawMessage(`{"repo":"test"}`),
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	_, err = e.Claim(ctx, TypeCoding)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}

	output := json.RawMessage(`{"result":"done","pr":42}`)
	if err := e.Complete(ctx, task.ID, output); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	got, err := e.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusCompleted {
		t.Errorf("Status: got %q, want %q", got.Status, StatusCompleted)
	}
	if string(got.Output) != string(output) {
		t.Errorf("Output: got %s, want %s", got.Output, output)
	}
}

func TestEngine_FailTask(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	task, err := e.Submit(ctx, &SubmitRequest{
		Type:       TypeDigest,
		Priority:   PriorityNormal,
		Input:      json.RawMessage(`{}`),
		MaxRetries: 1, // fail immediately, no retry
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	_, err = e.Claim(ctx, TypeDigest)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}

	taskErr := errors.New("something went wrong")
	if err := e.Fail(ctx, task.ID, taskErr); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	got, err := e.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusFailed {
		t.Errorf("Status: got %q, want %q", got.Status, StatusFailed)
	}
	if got.Error != "something went wrong" {
		t.Errorf("Error: got %q, want %q", got.Error, "something went wrong")
	}
}

func TestEngine_FailTask_Retry(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	task, err := e.Submit(ctx, &SubmitRequest{
		Type:       TypeDigest,
		Priority:   PriorityNormal,
		Input:      json.RawMessage(`{"key":"retry-test"}`),
		MaxRetries: 3,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	_, err = e.Claim(ctx, TypeDigest)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}

	// First failure — should re-queue, not mark as failed.
	if err := e.Fail(ctx, task.ID, errors.New("transient error")); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	got, err := e.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusQueued {
		t.Errorf("Status after first failure: got %q, want %q", got.Status, StatusQueued)
	}
	if got.Retries != 1 {
		t.Errorf("Retries: got %d, want 1", got.Retries)
	}
}

func TestEngine_CancelTask(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	task, err := e.Submit(ctx, &SubmitRequest{
		Type:     TypeWorkflow,
		Priority: PriorityLow,
		Input:    json.RawMessage(`{"workflow":"build"}`),
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	if err := e.Cancel(ctx, task.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	got, err := e.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusCanceled {
		t.Errorf("Status: got %q, want %q", got.Status, StatusCanceled)
	}
}

func TestEngine_Dedup(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	input := json.RawMessage(`{"prompt":"same input"}`)

	_, err := e.Submit(ctx, &SubmitRequest{
		Type:     TypeChat,
		Priority: PriorityNormal,
		Input:    input,
	})
	if err != nil {
		t.Fatalf("First submit: %v", err)
	}

	// Second submit with same type+input should fail.
	_, err = e.Submit(ctx, &SubmitRequest{
		Type:     TypeChat,
		Priority: PriorityNormal,
		Input:    input,
	})
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("Second submit: got %v, want ErrDuplicate", err)
	}
}

func TestEngine_Reaper(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	// Submit a task with MaxRetries=1 so the reaper will fail it.
	task, err := e.Submit(ctx, &SubmitRequest{
		Type:       TypeCoding,
		Priority:   PriorityNormal,
		Input:      json.RawMessage(`{"key":"reaper-test"}`),
		MaxRetries: 1,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Claim it with a very short lease via the store directly.
	claimed, err := e.store.Claim(ctx, TypeCoding, "worker", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if claimed == nil {
		t.Fatal("Claim returned nil")
	}

	// Remove from the in-memory queue so the engine doesn't try to pop it.
	e.queue.Remove(task.ID)

	// Wait for the lease to expire.
	time.Sleep(200 * time.Millisecond)

	// Run reaper manually.
	e.reap()

	// Task should now be failed (MaxRetries=1, so 1 retry attempt exhausts it).
	got, err := e.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusFailed {
		t.Errorf("Status after reap: got %q, want %q", got.Status, StatusFailed)
	}
}

func TestEngine_Reaper_Requeue(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	// Submit a task with MaxRetries=3 so the reaper will re-queue it.
	task, err := e.Submit(ctx, &SubmitRequest{
		Type:       TypeCoding,
		Priority:   PriorityNormal,
		Input:      json.RawMessage(`{"key":"reaper-requeue"}`),
		MaxRetries: 3,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Claim with short lease.
	claimed, err := e.store.Claim(ctx, TypeCoding, "worker", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if claimed == nil {
		t.Fatal("Claim returned nil")
	}
	e.queue.Remove(task.ID)

	// Wait for lease to expire, then reap.
	time.Sleep(200 * time.Millisecond)
	e.reap()

	got, err := e.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusQueued {
		t.Errorf("Status after reap: got %q, want %q (should re-queue)", got.Status, StatusQueued)
	}
	if got.Retries != 1 {
		t.Errorf("Retries: got %d, want 1", got.Retries)
	}
}

// TestEngine_ClaimFromStore_AfterRestart simulates the post-restart scenario
// where tasks exist in the DB but not in the in-memory queue. The engine's
// Claim method must fall back to the store-level claim with empty type.
func TestEngine_ClaimFromStore_AfterRestart(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	// Submit a task normally (goes to both DB and queue).
	task, err := e.Submit(ctx, &SubmitRequest{
		Type:        "cron",
		Priority:    PriorityNormal,
		Input:       json.RawMessage(`{"instruction":"daily digest"}`),
		Description: "generate daily digest",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Simulate restart: drain the in-memory queue so only DB has the task.
	e.queue.Remove(task.ID)

	// Verify queue is empty.
	if e.queue.Len() != 0 {
		t.Fatalf("Queue should be empty after remove, got %d", e.queue.Len())
	}

	// Claim with empty type (how the loop calls it). Before the fix, this
	// would fail because Store.Claim used WHERE type = '' which matches nothing.
	claimed, err := e.Claim(ctx, "")
	if err != nil {
		t.Fatalf("Claim after simulated restart: %v", err)
	}
	if claimed == nil {
		t.Fatal("Claim returned nil - store fallback must handle empty type as 'any'")
	}
	if claimed.ID != task.ID {
		t.Errorf("Claimed ID: got %q, want %q", claimed.ID, task.ID)
	}
	if claimed.Status != StatusClaimed {
		t.Errorf("Claimed status: got %q, want %q", claimed.Status, StatusClaimed)
	}
}

func TestEngine_List(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := e.Submit(ctx, &SubmitRequest{
			Type:     TypeChat,
			Priority: Priority(i),
			Input:    json.RawMessage(`{"i":` + string(rune('0'+i)) + `}`),
		})
		if err != nil {
			t.Fatalf("Submit %d: %v", i, err)
		}
	}

	tasks, err := e.List(ctx, ListOpts{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tasks) != 3 {
		t.Errorf("List: got %d tasks, want 3", len(tasks))
	}
}
