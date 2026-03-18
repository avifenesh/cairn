package llm

import (
	"context"
	"encoding/json"
)

// Role for message authors.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	RoleTool      Role = "tool"
)

// Request to send to an LLM.
type Request struct {
	Model           string
	Messages        []Message
	System          string
	Tools           []ToolDef
	MaxTokens       int
	Temperature     *float64
	Stop            []string
	DisableThinking bool // Skip reasoning/thinking for simple prompts
}

// Message in conversation.
type Message struct {
	Role    Role
	Content []ContentBlock
}

// ContentBlock is a variant type for message content.
type ContentBlock interface {
	contentBlock()
}

// TextBlock holds plain text content.
type TextBlock struct {
	Text string
}

func (TextBlock) contentBlock() {}

// ToolUseBlock represents an LLM requesting a tool call.
type ToolUseBlock struct {
	ID    string
	Name  string
	Input json.RawMessage
}

func (ToolUseBlock) contentBlock() {}

// ToolResultBlock carries the result of a tool call back to the LLM.
type ToolResultBlock struct {
	ToolUseID string
	Content   string
	IsError   bool
}

func (ToolResultBlock) contentBlock() {}

// ReasoningBlock holds chain-of-thought / thinking content.
type ReasoningBlock struct {
	Text string
}

func (ReasoningBlock) contentBlock() {}

// ToolDef describes a tool the LLM can call.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema
}

// Event is a streaming event variant emitted during LLM response generation.
type Event interface {
	eventType() string
}

// TextDelta streams a fragment of text content.
type TextDelta struct {
	Text string
}

func (TextDelta) eventType() string { return "text_delta" }

// ReasoningDelta streams a fragment of reasoning/thinking content.
type ReasoningDelta struct {
	Text string
}

func (ReasoningDelta) eventType() string { return "reasoning_delta" }

// ToolCallDelta signals a tool call (accumulated from stream fragments).
type ToolCallDelta struct {
	ID    string
	Name  string
	Input json.RawMessage
}

func (ToolCallDelta) eventType() string { return "tool_call_delta" }

// MessageEnd signals the end of the LLM response with usage stats.
type MessageEnd struct {
	InputTokens  int
	OutputTokens int
	FinishReason string
	Model        string
}

func (MessageEnd) eventType() string { return "message_end" }

// StreamError signals an error during streaming.
type StreamError struct {
	Err       error
	Retryable bool
}

func (StreamError) eventType() string { return "stream_error" }

// Provider is the interface each LLM provider implements.
type Provider interface {
	// ID returns a unique identifier for this provider (e.g. "glm", "anthropic").
	ID() string
	// Stream sends a request and returns a channel of streaming events.
	// The channel is closed when the stream ends (either successfully or on error).
	Stream(ctx context.Context, req *Request) (<-chan Event, error)
	// Models returns the list of models this provider supports.
	Models() []ModelInfo
}

// ModelInfo describes a model offered by a provider.
type ModelInfo struct {
	ID              string
	DisplayName     string
	MaxTokens       int
	CostPer1MInput  float64
	CostPer1MOutput float64
}
