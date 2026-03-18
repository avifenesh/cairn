// Package task provides the task engine for Cairn: lifecycle management,
// priority queue, lease-based claiming, worktree isolation, and dedup.
package task

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"
)

// TaskType identifies the kind of work a task represents.
type TaskType string

const (
	TypeGeneral  TaskType = "general"
	TypeChat     TaskType = "chat"
	TypeCoding   TaskType = "coding"
	TypeDigest   TaskType = "digest"
	TypeTriage   TaskType = "triage"
	TypeWorkflow TaskType = "workflow"
)

// TaskStatus tracks the lifecycle state of a task.
type TaskStatus string

const (
	StatusQueued    TaskStatus = "queued"
	StatusClaimed   TaskStatus = "claimed"
	StatusRunning   TaskStatus = "running"
	StatusCompleted TaskStatus = "completed"
	StatusFailed    TaskStatus = "failed"
	StatusCanceled  TaskStatus = "canceled"
)

// Priority defines task urgency. Lower number = higher priority.
type Priority int

const (
	PriorityCritical Priority = 0
	PriorityHigh     Priority = 1
	PriorityNormal   Priority = 2
	PriorityLow      Priority = 3
	PriorityIdle     Priority = 4
)

// Task represents a unit of work in the system.
type Task struct {
	ID          string
	ParentID    string
	SessionID   string
	Type        TaskType
	Status      TaskStatus
	Description string
	Priority    Priority
	Mode        string // "talk", "work", "coding"
	Input       json.RawMessage
	Output      json.RawMessage
	Error       string
	WorktreeDir string
	LeaseOwner  string
	LeaseExpiry time.Time
	Retries     int
	MaxRetries  int
	CostUSD     float64
	CreatedAt   time.Time
	StartedAt   time.Time
	CompletedAt time.Time
	UpdatedAt   time.Time
}

// SubmitRequest contains the parameters for creating a new task.
type SubmitRequest struct {
	Type        TaskType
	Priority    Priority
	Mode        string
	SessionID   string
	ParentID    string
	Input       json.RawMessage
	Description string
	MaxRetries  int
}

// ListOpts filters task listings.
type ListOpts struct {
	Status   TaskStatus
	Type     TaskType
	Limit    int
	Before   time.Time
	Archived bool
}

// newID generates a random 16-byte hex task ID.
func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("task: crypto/rand failed: %v", err))
	}
	return fmt.Sprintf("%x", b)
}
