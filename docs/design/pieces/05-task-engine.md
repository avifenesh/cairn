# Piece 5: Task Engine

> Task lifecycle, priority queue, worktree isolation, lease-based claiming.

## Interface

```go
type Task struct {
    ID          string
    ParentID    string        // for sub-tasks
    SessionID   string
    Type        TaskType      // chat, coding, digest, triage, workflow
    Status      TaskStatus    // queued, claimed, running, completed, failed, canceled
    Priority    Priority      // critical, high, normal, low, idle
    Mode        tool.Mode
    Input       json.RawMessage
    Output      json.RawMessage
    Error       string
    WorktreeDir string        // set when coding task starts
    LeaseExpiry time.Time     // auto-fail if not heartbeated
    Retries     int
    MaxRetries  int
    CostUSD     float64
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type TaskStatus string
const (
    StatusQueued    TaskStatus = "queued"
    StatusClaimed   TaskStatus = "claimed"   // lease acquired, not yet started
    StatusRunning   TaskStatus = "running"
    StatusCompleted TaskStatus = "completed"
    StatusFailed    TaskStatus = "failed"
    StatusCanceled  TaskStatus = "canceled"
)

type Engine interface {
    Submit(ctx context.Context, req *SubmitRequest) (*Task, error)
    Cancel(ctx context.Context, taskID string) error
    Get(ctx context.Context, taskID string) (*Task, error)
    List(ctx context.Context, opts ListOpts) ([]*Task, error)
    Heartbeat(ctx context.Context, taskID string) error

    // Internal — used by workers
    Claim(ctx context.Context, taskType TaskType) (*Task, error)  // lease-based
    Complete(ctx context.Context, taskID string, output json.RawMessage) error
    Fail(ctx context.Context, taskID string, err error) error
}
```

## Worktree Manager (Uzi-inspired)

```go
type WorktreeManager struct {
    repoDir     string          // main repo path
    worktreeDir string          // base dir for worktrees (e.g., ~/.pub/worktrees/)
    mu          sync.Mutex      // serialize git worktree operations
}

func (m *WorktreeManager) Create(taskID, baseBranch string) (worktreePath, branchName string, err error)
func (m *WorktreeManager) Remove(taskID string) error
func (m *WorktreeManager) Merge(taskID, targetBranch string) error  // rebase
func (m *WorktreeManager) List() ([]WorktreeInfo, error)

type WorktreeInfo struct {
    TaskID     string
    Path       string
    Branch     string
    CreatedAt  time.Time
}
```

## Priority Queue

```go
// In-memory priority queue backed by heap
type Queue struct {
    heap     taskHeap        // min-heap by priority + createdAt
    byID     map[string]*Task
    mu       sync.Mutex
    notEmpty chan struct{}    // signal for workers waiting
}

func (q *Queue) Push(task *Task)
func (q *Queue) Pop(taskType TaskType) *Task  // blocks until available
func (q *Queue) Remove(taskID string)
func (q *Queue) Len() int
```

## Lease-Based Claiming (Gollem pattern)

```
1. Worker calls engine.Claim(taskType)
2. Engine atomically: UPDATE tasks SET status='claimed', lease_expiry=now()+TTL WHERE status='queued' AND type=? LIMIT 1
3. Worker starts heartbeating every 30s
4. If heartbeat stops → lease reaper marks task as queued (retry)
5. After max_retries → mark as failed
```

## Dedup Guard

```go
// Prevents duplicate tasks for same PR/session (the bug we fixed in #700)
func (e *engine) isDuplicate(req *SubmitRequest) bool {
    running := e.List(ctx, ListOpts{Status: StatusRunning, Type: req.Type})
    for _, t := range running {
        if t.Input matches req.Input by PR number or description similarity {
            return true
        }
    }
    return false
}
```

## Subphases

| # | Subphase | Depends On |
|---|----------|------------|
| 5.1 | Task types + store (SQLite) | Nothing |
| 5.2 | Priority queue (in-memory) | 5.1 |
| 5.3 | Worktree manager (git operations) | Nothing |
| 5.4 | Lease-based claiming + reaper | 5.1, 5.2 |
| 5.5 | Dedup guard | 5.1 |
| 5.6 | Worker pool (goroutines per task type) | 5.2, 5.4 |
| 5.7 | Task → Agent wiring (submit coding task → create session → run agent) | 4 (agent), 5.1-5.6 |
| 5.8 | Tests | All |
