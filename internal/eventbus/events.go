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
	Title      string `json:"title"`
	URL        string `json:"url"`
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
	Status     string `json:"status"` // "connected", "disconnected", "error"
	ToolCount  int    `json:"toolCount"`
	Error      string `json:"error,omitempty"`
}

// --- System events ---

// ShutdownInitiated is emitted when the system begins shutting down.
type ShutdownInitiated struct {
	EventMeta
}
