package task

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/avifenesh/cairn/internal/eventbus"
)

const defaultLeaseDuration = 5 * time.Minute

// ErrDuplicate is returned when a duplicate task is submitted.
var ErrDuplicate = errors.New("task: duplicate task already running")

// Engine orchestrates the task lifecycle: submit, claim, complete, fail, cancel.
type Engine struct {
	store    *Store
	queue    *Queue
	worktree *WorktreeManager
	bus      *eventbus.Bus
	reaper   *time.Ticker
	done     chan struct{}
}

// NewEngine creates a task engine wired to the given store, event bus, and worktree manager.
// worktree may be nil if worktree management is not needed.
func NewEngine(store *Store, bus *eventbus.Bus, worktree *WorktreeManager) *Engine {
	return &Engine{
		store:    store,
		queue:    NewQueue(),
		worktree: worktree,
		bus:      bus,
		done:     make(chan struct{}),
	}
}

// Submit creates a new task, persists it, enqueues it, and emits a TaskCreated event.
// Returns ErrDuplicate if a running/queued/claimed task with the same Type+Input exists.
func (e *Engine) Submit(ctx context.Context, req *SubmitRequest) (*Task, error) {
	// Dedup check: look for running/queued/claimed tasks with same type + input.
	if err := e.checkDuplicate(ctx, req); err != nil {
		return nil, err
	}

	now := time.Now()
	maxRetries := req.MaxRetries
	if maxRetries == 0 {
		maxRetries = 2
	}

	t := &Task{
		ID:          newID(),
		ParentID:    req.ParentID,
		SessionID:   req.SessionID,
		Type:        req.Type,
		Status:      StatusQueued,
		Priority:    req.Priority,
		Mode:        req.Mode,
		Input:       req.Input,
		Description: req.Description,
		MaxRetries:  maxRetries,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := e.store.Create(ctx, t); err != nil {
		return nil, fmt.Errorf("task engine: submit: %w", err)
	}

	e.queue.Push(t)

	eventbus.Publish(e.bus, eventbus.TaskCreated{
		EventMeta:   eventbus.NewMeta("task-engine"),
		TaskID:      t.ID,
		Type:        string(t.Type),
		Description: req.Description,
	})

	slog.Info("task submitted", "id", t.ID, "type", t.Type, "priority", t.Priority)
	return t, nil
}

// Cancel marks a task as canceled. If queued, removes it from the queue.
func (e *Engine) Cancel(ctx context.Context, taskID string) error {
	t, err := e.store.Get(ctx, taskID)
	if err != nil {
		return fmt.Errorf("task engine: cancel: %w", err)
	}
	if t == nil {
		return fmt.Errorf("task engine: cancel: task %s not found", taskID)
	}

	if t.Status == StatusCompleted || t.Status == StatusFailed || t.Status == StatusCanceled {
		return fmt.Errorf("task engine: cancel: task %s already in terminal state %s", taskID, t.Status)
	}

	t.Status = StatusCanceled
	t.UpdatedAt = time.Now()

	if err := e.store.Update(ctx, t); err != nil {
		return fmt.Errorf("task engine: cancel update: %w", err)
	}

	e.queue.Remove(taskID)

	eventbus.Publish(e.bus, eventbus.TaskFailed{
		EventMeta: eventbus.NewMeta("task-engine"),
		TaskID:    taskID,
		Error:     "canceled",
	})

	slog.Info("task canceled", "id", taskID)
	return nil
}

// Delete permanently removes a task from the store.
func (e *Engine) Delete(ctx context.Context, taskID string) error {
	e.queue.Remove(taskID)
	return e.store.Delete(ctx, taskID)
}

// Get retrieves a task by ID.
func (e *Engine) Get(ctx context.Context, taskID string) (*Task, error) {
	t, err := e.store.Get(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("task engine: get: %w", err)
	}
	return t, nil
}

// List retrieves tasks matching the given filters.
func (e *Engine) List(ctx context.Context, opts ListOpts) ([]*Task, error) {
	return e.store.List(ctx, opts)
}

// Claim pops the highest-priority task of the given type from the queue,
// marks it as claimed in the store with a lease, and returns it.
// If no task is available, it falls back to the store-level atomic claim.
func (e *Engine) Claim(ctx context.Context, taskType TaskType) (*Task, error) {
	// Try in-memory queue first (non-blocking attempt with short timeout).
	claimCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	t, err := e.queue.Pop(claimCtx, taskType)
	if err == nil && t != nil {
		// Persist the claim.
		t.Status = StatusClaimed
		t.LeaseOwner = "engine"
		t.LeaseExpiry = time.Now().Add(defaultLeaseDuration)
		t.UpdatedAt = time.Now()
		if updateErr := e.store.Update(ctx, t); updateErr != nil {
			slog.Error("task engine: failed to persist claim from queue", "id", t.ID, "err", updateErr)
			// Put it back.
			t.Status = StatusQueued
			e.queue.Push(t)
			return nil, updateErr
		}
		return t, nil
	}

	// Fallback: try store-level atomic claim (handles tasks that were
	// persisted but not in memory, e.g. after restart).
	t, err = e.store.Claim(ctx, taskType, "engine", defaultLeaseDuration)
	if err != nil {
		return nil, fmt.Errorf("task engine: claim: %w", err)
	}
	return t, nil
}

// MarkRunning sets a task to running status so the agent loop won't claim it.
// Used by the HTTP handler for chat tasks that are handled inline.
func (e *Engine) MarkRunning(ctx context.Context, taskID string) {
	t, err := e.store.Get(ctx, taskID)
	if err != nil || t == nil {
		return
	}
	t.Status = StatusRunning
	t.LeaseOwner = "http"
	t.LeaseExpiry = time.Now().Add(defaultLeaseDuration)
	t.UpdatedAt = time.Now()
	e.store.Update(ctx, t)
	// Remove from queue so the loop doesn't see it.
	e.queue.Remove(taskID)
}

// Complete marks a task as completed with the given output.
// Idempotent: re-completing updates the output if non-empty (loop's
// final output wins over early tool calls with empty output).
func (e *Engine) Complete(ctx context.Context, taskID string, output json.RawMessage) error {
	t, err := e.store.Get(ctx, taskID)
	if err != nil {
		return fmt.Errorf("task engine: complete: %w", err)
	}
	if t == nil {
		return fmt.Errorf("task engine: complete: task %s not found", taskID)
	}
	// Already completed: update output if caller provides non-empty content and notify subscribers.
	if t.Status == StatusCompleted {
		if len(output) > 0 && string(output) != `""` {
			t.Output = output
			t.UpdatedAt = time.Now()
			if err := e.store.Update(ctx, t); err != nil {
				return fmt.Errorf("task engine: complete update: %w", err)
			}

			eventbus.Publish(e.bus, eventbus.TaskCompleted{
				EventMeta: eventbus.NewMeta("task-engine"),
				TaskID:    taskID,
				Result:    string(output),
			})

			slog.Info("task output updated for completed task", "id", taskID)
		}
		return nil
	}
	// Don't allow completing failed/canceled tasks.
	if t.Status == StatusFailed || t.Status == StatusCanceled {
		return fmt.Errorf("task engine: complete: task %s already in terminal state %s", taskID, t.Status)
	}

	t.Status = StatusCompleted
	t.Output = output
	t.UpdatedAt = time.Now()
	t.LeaseOwner = ""
	t.LeaseExpiry = time.Time{}

	if err := e.store.Update(ctx, t); err != nil {
		return fmt.Errorf("task engine: complete update: %w", err)
	}

	eventbus.Publish(e.bus, eventbus.TaskCompleted{
		EventMeta: eventbus.NewMeta("task-engine"),
		TaskID:    taskID,
		Result:    string(output),
	})

	slog.Info("task completed", "id", taskID)
	return nil
}

// Fail marks a task as failed. If retries remain, re-queues it instead.
func (e *Engine) Fail(ctx context.Context, taskID string, taskErr error) error {
	t, err := e.store.Get(ctx, taskID)
	if err != nil {
		return fmt.Errorf("task engine: fail: %w", err)
	}
	if t == nil {
		return fmt.Errorf("task engine: fail: task %s not found", taskID)
	}

	t.Retries++
	t.LeaseOwner = ""
	t.LeaseExpiry = time.Time{}
	t.UpdatedAt = time.Now()

	if t.Retries < t.MaxRetries {
		// Re-queue for retry.
		t.Status = StatusQueued
		t.Error = taskErr.Error()
		if err := e.store.Update(ctx, t); err != nil {
			return fmt.Errorf("task engine: fail re-queue: %w", err)
		}
		e.queue.Push(t)
		slog.Info("task re-queued for retry", "id", taskID, "retries", t.Retries, "maxRetries", t.MaxRetries)
		return nil
	}

	t.Status = StatusFailed
	t.Error = taskErr.Error()

	if err := e.store.Update(ctx, t); err != nil {
		return fmt.Errorf("task engine: fail update: %w", err)
	}

	eventbus.Publish(e.bus, eventbus.TaskFailed{
		EventMeta: eventbus.NewMeta("task-engine"),
		TaskID:    taskID,
		Error:     taskErr.Error(),
	})

	slog.Info("task failed", "id", taskID, "error", taskErr)
	return nil
}

// Heartbeat extends the lease on a running task.
func (e *Engine) Heartbeat(ctx context.Context, taskID string) error {
	return e.store.Heartbeat(ctx, taskID, defaultLeaseDuration)
}

// StartReaper launches a background goroutine that periodically finds tasks
// with expired leases and either re-queues them (if retries remain) or
// marks them as failed.
func (e *Engine) StartReaper(interval time.Duration) {
	e.reaper = time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-e.done:
				return
			case <-e.reaper.C:
				e.reap()
			}
		}
	}()
}

// Close stops the reaper and cleans up.
func (e *Engine) Close() {
	close(e.done)
	if e.reaper != nil {
		e.reaper.Stop()
	}
}

// reap finds expired leases and handles them.
func (e *Engine) reap() {
	ctx := context.Background()
	expired, err := e.store.FindExpiredLeases(ctx)
	if err != nil {
		slog.Error("reaper: find expired leases", "err", err)
		return
	}

	for _, t := range expired {
		t.Retries++
		t.LeaseOwner = ""
		t.LeaseExpiry = time.Time{}
		t.UpdatedAt = time.Now()

		if t.Retries < t.MaxRetries {
			t.Status = StatusQueued
			if err := e.store.Update(ctx, t); err != nil {
				slog.Error("reaper: re-queue task", "id", t.ID, "err", err)
				continue
			}
			e.queue.Push(t)
			slog.Info("reaper: task re-queued", "id", t.ID, "retries", t.Retries)
		} else {
			t.Status = StatusFailed
			t.Error = "lease expired after max retries"
			if err := e.store.Update(ctx, t); err != nil {
				slog.Error("reaper: fail task", "id", t.ID, "err", err)
				continue
			}
			eventbus.Publish(e.bus, eventbus.TaskFailed{
				EventMeta: eventbus.NewMeta("task-reaper"),
				TaskID:    t.ID,
				Error:     t.Error,
			})
			slog.Info("reaper: task failed", "id", t.ID)
		}
	}
}

// checkDuplicate looks for queued/claimed/running tasks with the same Type and Input.
func (e *Engine) checkDuplicate(ctx context.Context, req *SubmitRequest) error {
	for _, status := range []TaskStatus{StatusQueued, StatusClaimed, StatusRunning} {
		tasks, err := e.store.List(ctx, ListOpts{
			Status: status,
			Type:   req.Type,
		})
		if err != nil {
			return fmt.Errorf("dedup check: %w", err)
		}
		for _, t := range tasks {
			if bytes.Equal(t.Input, req.Input) {
				return ErrDuplicate
			}
		}
	}
	return nil
}
