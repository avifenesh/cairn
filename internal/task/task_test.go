package task

import "testing"

func TestTaskTypes(t *testing.T) {
	types := []TaskType{TypeChat, TypeCoding, TypeDigest, TypeTriage, TypeWorkflow}
	expected := []string{"chat", "coding", "digest", "triage", "workflow"}

	for i, tt := range types {
		if string(tt) != expected[i] {
			t.Errorf("TaskType %d: got %q, want %q", i, tt, expected[i])
		}
	}
}

func TestTaskStatuses(t *testing.T) {
	statuses := []TaskStatus{StatusQueued, StatusClaimed, StatusRunning, StatusCompleted, StatusFailed, StatusCanceled}
	expected := []string{"queued", "claimed", "running", "completed", "failed", "canceled"}

	for i, s := range statuses {
		if string(s) != expected[i] {
			t.Errorf("TaskStatus %d: got %q, want %q", i, s, expected[i])
		}
	}
}

func TestPriorities(t *testing.T) {
	if PriorityCritical != 0 {
		t.Errorf("PriorityCritical: got %d, want 0", PriorityCritical)
	}
	if PriorityHigh != 1 {
		t.Errorf("PriorityHigh: got %d, want 1", PriorityHigh)
	}
	if PriorityNormal != 2 {
		t.Errorf("PriorityNormal: got %d, want 2", PriorityNormal)
	}
	if PriorityLow != 3 {
		t.Errorf("PriorityLow: got %d, want 3", PriorityLow)
	}
	if PriorityIdle != 4 {
		t.Errorf("PriorityIdle: got %d, want 4", PriorityIdle)
	}
}

func TestNewID(t *testing.T) {
	id1 := newID()
	id2 := newID()

	if len(id1) != 32 { // 16 bytes = 32 hex chars
		t.Errorf("newID length: got %d, want 32", len(id1))
	}
	if id1 == id2 {
		t.Errorf("newID should be unique, got %s twice", id1)
	}
}
