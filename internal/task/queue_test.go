package task

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestQueue_PriorityOrder(t *testing.T) {
	q := NewQueue()
	ctx := context.Background()

	// Push 3 tasks with different priorities.
	tasks := []*Task{
		{ID: "low", Priority: PriorityLow, Type: TypeChat, CreatedAt: time.Now()},
		{ID: "critical", Priority: PriorityCritical, Type: TypeChat, CreatedAt: time.Now()},
		{ID: "normal", Priority: PriorityNormal, Type: TypeChat, CreatedAt: time.Now()},
	}
	for _, task := range tasks {
		q.Push(task)
	}

	// Pop should return in priority order: critical, normal, low.
	expected := []string{"critical", "normal", "low"}
	for _, want := range expected {
		got, err := q.Pop(ctx, "")
		if err != nil {
			t.Fatalf("Pop: %v", err)
		}
		if got.ID != want {
			t.Errorf("Pop: got %q, want %q", got.ID, want)
		}
	}

	if q.Len() != 0 {
		t.Errorf("Len: got %d, want 0", q.Len())
	}
}

func TestQueue_PopBlocks(t *testing.T) {
	q := NewQueue()

	// Pop on empty queue should block until push or cancel.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})

	go func() {
		defer close(done)
		got, err := q.Pop(ctx, TypeChat)
		if err != nil {
			return // expected — context cancelled
		}
		if got.ID != "unblocked" {
			t.Errorf("Pop got %q, want %q", got.ID, "unblocked")
		}
	}()

	// Push after a short delay to unblock.
	time.Sleep(30 * time.Millisecond)
	q.Push(&Task{ID: "unblocked", Priority: PriorityNormal, Type: TypeChat, CreatedAt: time.Now()})

	<-done
}

func TestQueue_PopBlocksContextCancel(t *testing.T) {
	q := NewQueue()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := q.Pop(ctx, TypeChat)
		done <- err
	}()

	// Cancel should unblock Pop.
	time.Sleep(20 * time.Millisecond)
	cancel()

	err := <-done
	if err == nil {
		t.Error("Pop should return error when context is cancelled")
	}
}

func TestQueue_Remove(t *testing.T) {
	q := NewQueue()
	ctx := context.Background()

	q.Push(&Task{ID: "keep", Priority: PriorityNormal, Type: TypeChat, CreatedAt: time.Now()})
	q.Push(&Task{ID: "remove", Priority: PriorityCritical, Type: TypeChat, CreatedAt: time.Now()})

	if q.Len() != 2 {
		t.Fatalf("Len before remove: got %d, want 2", q.Len())
	}

	q.Remove("remove")

	if q.Len() != 1 {
		t.Fatalf("Len after remove: got %d, want 1", q.Len())
	}

	// The remaining item should be "keep".
	got, err := q.Pop(ctx, "")
	if err != nil {
		t.Fatalf("Pop: %v", err)
	}
	if got.ID != "keep" {
		t.Errorf("Pop after remove: got %q, want %q", got.ID, "keep")
	}
}

func TestQueue_ConcurrentPushPop(t *testing.T) {
	q := NewQueue()
	const n = 100

	var wg sync.WaitGroup
	ctx := context.Background()

	// Push n tasks concurrently.
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			q.Push(&Task{
				ID:        newID(),
				Priority:  Priority(i % 5),
				Type:      TypeChat,
				CreatedAt: time.Now(),
			})
		}(i)
	}
	wg.Wait()

	if q.Len() != n {
		t.Fatalf("Len after push: got %d, want %d", q.Len(), n)
	}

	// Pop n tasks concurrently.
	results := make(chan *Task, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			task, err := q.Pop(ctx, "")
			if err != nil {
				t.Errorf("Pop: %v", err)
				return
			}
			results <- task
		}()
	}
	wg.Wait()
	close(results)

	// Verify all tasks were popped.
	count := 0
	seen := make(map[string]bool)
	for task := range results {
		if seen[task.ID] {
			t.Errorf("duplicate task popped: %s", task.ID)
		}
		seen[task.ID] = true
		count++
	}
	if count != n {
		t.Errorf("popped %d tasks, want %d", count, n)
	}

	if q.Len() != 0 {
		t.Errorf("Len after pop all: got %d, want 0", q.Len())
	}
}
