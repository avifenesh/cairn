package task

import (
	"container/heap"
	"context"
	"sync"
)

// Queue is an in-memory priority queue for tasks, backed by a min-heap.
// Pop blocks until a matching task is available or the context is cancelled.
type Queue struct {
	mu       sync.Mutex
	heap     taskHeap
	byID     map[string]*queueItem
	notEmpty chan struct{}
}

// queueItem wraps a Task with its heap index.
type queueItem struct {
	task  *Task
	index int
}

// NewQueue creates an empty priority queue.
func NewQueue() *Queue {
	return &Queue{
		byID:     make(map[string]*queueItem),
		notEmpty: make(chan struct{}, 1),
	}
}

// Push adds a task to the queue.
func (q *Queue) Push(t *Task) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, exists := q.byID[t.ID]; exists {
		return // already in queue
	}

	item := &queueItem{task: t}
	heap.Push(&q.heap, item)
	q.byID[t.ID] = item

	// Signal that the queue is non-empty.
	select {
	case q.notEmpty <- struct{}{}:
	default:
	}
}

// Pop blocks until a task matching taskType is available, then removes and
// returns it. If taskType is empty, any task type matches. Returns an error
// only if the context is cancelled.
func (q *Queue) Pop(ctx context.Context, taskType TaskType) (*Task, error) {
	for {
		q.mu.Lock()
		t := q.popMatching(taskType)
		q.mu.Unlock()

		if t != nil {
			return t, nil
		}

		// Wait for a signal or context cancellation.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-q.notEmpty:
			// A new item was pushed; loop back and try again.
		}
	}
}

// popMatching finds and removes the highest-priority task matching taskType.
// Caller must hold q.mu.
func (q *Queue) popMatching(taskType TaskType) *Task {
	if q.heap.Len() == 0 {
		return nil
	}

	if taskType == "" {
		// Any type: just pop the top.
		item := heap.Pop(&q.heap).(*queueItem)
		delete(q.byID, item.task.ID)
		return item.task
	}

	// Find the highest-priority item matching the type.
	for i := 0; i < q.heap.Len(); i++ {
		item := q.heap[i]
		if item.task.Type == taskType {
			heap.Remove(&q.heap, item.index)
			delete(q.byID, item.task.ID)
			return item.task
		}
	}
	return nil
}

// Remove deletes a task from the queue by ID. No-op if not found.
func (q *Queue) Remove(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, ok := q.byID[id]
	if !ok {
		return
	}
	heap.Remove(&q.heap, item.index)
	delete(q.byID, id)
}

// Len returns the number of tasks in the queue.
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.heap.Len()
}

// --- heap.Interface implementation ---

// taskHeap implements heap.Interface for priority-ordered tasks.
// Lower Priority number = higher urgency. Ties broken by earlier CreatedAt.
type taskHeap []*queueItem

func (h taskHeap) Len() int { return len(h) }

func (h taskHeap) Less(i, j int) bool {
	if h[i].task.Priority != h[j].task.Priority {
		return h[i].task.Priority < h[j].task.Priority
	}
	return h[i].task.CreatedAt.Before(h[j].task.CreatedAt)
}

func (h taskHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *taskHeap) Push(x any) {
	item := x.(*queueItem)
	item.index = len(*h)
	*h = append(*h, item)
}

func (h *taskHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // avoid memory leak
	item.index = -1
	*h = old[:n-1]
	return item
}
