package eventbus

import (
	"crypto/rand"
	"fmt"
	"time"
)

// EventMeta contains base fields every event should carry.
type EventMeta struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"`
}

// NewMeta creates an EventMeta with a random ID and current timestamp.
func NewMeta(source string) EventMeta {
	b := make([]byte, 8)
	rand.Read(b)
	return EventMeta{
		ID:        fmt.Sprintf("%x", b),
		Timestamp: time.Now(),
		Source:    source,
	}
}

// --- Signal events ---

// EventIngested is emitted when a new event is ingested from a source.
type EventIngested struct {
	EventMeta
	SourceType string `json:"sourceType"`
	Kind       string `json:"kind"` // pr, issue, story, email, etc.
	Title      string `json:"title"`
	URL        string `json:"url"`
	Actor      string `json:"actor"` // who created the event
	Repo       string `json:"repo"`  // repo name (GitHub events)
}

// EventRead is emitted when a user marks an event as read.
type EventRead struct {
	EventMeta
	EventID string `json:"eventId"`
}

// EventArchived is emitted when an event is archived.
type EventArchived struct {
	EventMeta
	EventID string `json:"eventId"`
}

// --- LLM events ---

// StreamStarted is emitted when an LLM stream begins.
type StreamStarted struct {
	EventMeta
	TaskID string `json:"taskId"`
	Model  string `json:"model"`
}

// TextDelta is emitted for each text chunk during LLM streaming.
type TextDelta struct {
	EventMeta
	TaskID string `json:"taskId"`
	Text   string `json:"text"`
}

// ReasoningDelta is emitted for reasoning/thinking content during LLM streaming.
type ReasoningDelta struct {
	EventMeta
	TaskID string `json:"taskId"`
	Text   string `json:"text"`
	Round  int    `json:"round"`
}

// ToolCallEvent is emitted when a tool is invoked during LLM processing.
type ToolCallEvent struct {
	EventMeta
	TaskID   string `json:"taskId"`
	ToolName string `json:"toolName"`
	Phase    string `json:"phase"`
}

// ToolExecuted is emitted after a tool completes execution with its result.
type ToolExecuted struct {
	EventMeta
	TaskID     string `json:"taskId"`
	ToolName   string `json:"toolName"`
	DurationMs int64  `json:"durationMs"`
	IsError    bool   `json:"isError"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
}

// StreamEnded is emitted when an LLM stream completes.
type StreamEnded struct {
	EventMeta
	TaskID       string `json:"taskId"`
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
	FinishReason string `json:"finishReason"`
}

// --- Task events ---

// TaskCreated is emitted when a new task is created.
type TaskCreated struct {
	EventMeta
	TaskID      string `json:"taskId"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// TaskRunning is emitted when a task begins execution.
type TaskRunning struct {
	EventMeta
	TaskID string `json:"taskId"`
}

// TaskCompleted is emitted when a task finishes successfully.
type TaskCompleted struct {
	EventMeta
	TaskID string `json:"taskId"`
	Result string `json:"result"`
}

// TaskFailed is emitted when a task fails.
type TaskFailed struct {
	EventMeta
	TaskID string `json:"taskId"`
	Error  string `json:"error"`
}

// --- Memory events ---

// MemoryProposed is emitted when a new memory is proposed.
type MemoryProposed struct {
	EventMeta
	MemoryID string `json:"memoryId"`
	Content  string `json:"content"`
}

// --- Soul events ---

// SoulPatchProposed is emitted when a soul patch is proposed for review.
type SoulPatchProposed struct {
	EventMeta
	PatchID string `json:"patchId"`
}

// MemoryAccepted is emitted when a proposed memory is accepted.
type MemoryAccepted struct {
	EventMeta
	MemoryID string `json:"memoryId"`
}

// MemoryRejected is emitted when a proposed memory is rejected.
type MemoryRejected struct {
	EventMeta
	MemoryID string `json:"memoryId"`
}

// --- MCP events ---

// MCPConnectionChanged is emitted when an MCP client connection status changes.
type MCPConnectionChanged struct {
	EventMeta
	ServerName string `json:"serverName"`
	Status     string `json:"status"` // "connected", "connecting", "disconnected", "error"
	ToolCount  int    `json:"toolCount"`
	Error      string `json:"error,omitempty"`
}

// --- Session observability events ---

// SessionEvent is emitted for every observable action in a coding session.
// The frontend session panel subscribes to these to show real-time activity.
type SessionEvent struct {
	EventMeta
	SessionID string `json:"sessionId"`
	EventType string `json:"eventType"` // tool_call, tool_result, file_change, text_delta, thinking, state_change, round_complete, user_steer
	Payload   any    `json:"payload"`
}

// --- Subagent events ---

// SubagentStarted is emitted when a child agent is spawned.
type SubagentStarted struct {
	EventMeta
	ParentTaskID string `json:"parentTaskId"`
	SubagentID   string `json:"subagentId"`
	AgentType    string `json:"agentType"`
	ExecMode     string `json:"execMode"`
	Instruction  string `json:"instruction"`
}

// SubagentProgress is emitted on each ReAct round of a running subagent.
type SubagentProgress struct {
	EventMeta
	SubagentID string `json:"subagentId"`
	Round      int    `json:"round"`
	MaxRounds  int    `json:"maxRounds"`
	ToolName   string `json:"toolName,omitempty"`
}

// SubagentCompleted is emitted when a child agent finishes or fails.
type SubagentCompleted struct {
	EventMeta
	SubagentID string `json:"subagentId"`
	Status     string `json:"status"` // "completed", "failed", "canceled"
	Summary    string `json:"summary"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"durationMs"`
	ToolCalls  int    `json:"toolCalls"`
	Rounds     int    `json:"rounds"`
}

// --- Rules events ---

// RuleExecuted is emitted when an automation rule fires.
type RuleExecuted struct {
	EventMeta
	RuleID     string `json:"ruleId"`
	RuleName   string `json:"ruleName"`
	Status     string `json:"status"` // "success", "error", "throttled", "condition_false"
	DurationMs int64  `json:"durationMs"`
	Error      string `json:"error,omitempty"`
}

// --- System events ---

// ShutdownInitiated is emitted when the system begins shutting down.
type ShutdownInitiated struct {
	EventMeta
}
