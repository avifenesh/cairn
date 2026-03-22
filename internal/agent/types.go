// Package agent provides the ReAct agent loop for Cairn: LLM reasoning,
// tool execution, session management, and mode-based behavior.
package agent

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/llm"
	"github.com/avifenesh/cairn/internal/memory"
	"github.com/avifenesh/cairn/internal/plugin"
	"github.com/avifenesh/cairn/internal/task"
	"github.com/avifenesh/cairn/internal/tool"
)

// Agent is the core interface for all agents.
type Agent interface {
	Name() string
	Description() string
	Run(ctx *InvocationContext) <-chan RunEvent
}

// RunEvent wraps an Event or error from an agent run.
type RunEvent struct {
	Event *Event
	Err   error
}

// InvocationContext carries all dependencies for an agent invocation.
type InvocationContext struct {
	Context        context.Context
	SessionID      string
	UserMessage    string
	Mode           tool.Mode
	Session        *Session
	Tools          *tool.Registry
	LLM            llm.Provider
	Memory         *memory.Service
	Soul           *memory.Soul
	Tasks          *task.Engine
	Bus            *eventbus.Bus
	Config         *AgentConfig
	ContextBuilder *memory.ContextBuilder      // Token-budgeted context assembly (nil = fallback)
	JournalEntries []memory.JournalDigestEntry // Last 48h journal for context
	Plugins        *plugin.Manager             // Lifecycle hooks (nil = no plugins)

	// Session compaction config.
	CompactionConfig CompactionConfig

	// ActivityStore records tool execution stats (nil = no recording).
	ActivityStore *ActivityStore

	// SteeringCh receives steering messages from the user during execution.
	// Checked between ReAct rounds. Nil = no steering support.
	SteeringCh <-chan SteeringMessage

	// Subagents spawns child agents. Nil = spawning not available (e.g., inside a child agent).
	Subagents tool.SubagentService

	// CheckpointStore persists session checkpoints for crash recovery.
	// Nil = no checkpointing (e.g., subagents).
	CheckpointStore *CheckpointStore

	// Origin tracks where this invocation came from ("chat", "task", "subagent").
	Origin string

	// Tool service adapters — passed through to ToolContext during execution.
	ToolMemories tool.MemoryService
	ToolEvents   tool.EventService
	ToolDigest   tool.DigestService
	ToolJournal  tool.JournalService
	ToolTasks    tool.TaskService
	ToolStatus   tool.StatusService
	ToolSkills   tool.SkillService
	ToolNotifier tool.NotifyService
	ToolCrons    tool.CronService
	ToolConfig   tool.ConfigService
}

// SteeringMessage represents a user intervention injected into an active session.
type SteeringMessage struct {
	Content  string `json:"content"`
	Priority string `json:"priority"` // "normal", "urgent", "stop"
}

// AgentConfig holds per-invocation agent configuration.
type AgentConfig struct {
	Model              string
	MaxRounds          int
	SubagentSystemHint string // Optional system prompt hint for subagent types (prepended to main prompt)
}

// Event represents a single event in the agent's execution.
// Events are append-only and form the session history.
type Event struct {
	ID        string    `json:"id"`
	SessionID string    `json:"sessionId"`
	Timestamp time.Time `json:"timestamp"`
	Author    string    `json:"author"` // agent name or "user"
	Round     int       `json:"round"`
	Parts     []Part    `json:"parts"`
}

// Part is a content variant within an Event.
type Part interface {
	partType() string
}

// TextPart holds streamed or final text content.
type TextPart struct {
	Text string `json:"text"`
}

func (TextPart) partType() string { return "text" }

// ToolPart tracks tool execution state.
type ToolPart struct {
	ToolName string          `json:"toolName"`
	CallID   string          `json:"callId"`
	Status   ToolStatus      `json:"status"`
	Input    json.RawMessage `json:"input,omitempty"`
	Output   string          `json:"output,omitempty"`
	Error    string          `json:"error,omitempty"`
	Duration time.Duration   `json:"duration,omitempty"`
}

func (ToolPart) partType() string { return "tool" }

// ToolStatus tracks the lifecycle of a tool call.
type ToolStatus string

const (
	ToolPending   ToolStatus = "pending"
	ToolRunning   ToolStatus = "running"
	ToolCompleted ToolStatus = "completed"
	ToolFailed    ToolStatus = "failed"
)

// ReasoningPart holds chain-of-thought content.
type ReasoningPart struct {
	Text string `json:"text"`
}

func (ReasoningPart) partType() string { return "reasoning" }

// Session represents a conversation with history.
type Session struct {
	ID           string
	Title        string
	Mode         tool.Mode
	Events       []*Event
	State        map[string]any
	ActiveSkills []ActiveSkill // Skills loaded in this session
	MessageCount int           // Number of messages (populated by List)
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ActiveSkill tracks a skill loaded in a session.
type ActiveSkill struct {
	Name         string
	Content      string   // Full skill body
	AllowedTools []string // Tool scoping from frontmatter (nil or empty = no restriction)
}

// AllowedToolsFromSkills returns the merged allowed-tools from all active skills.
// Returns nil if no skills have tool restrictions (meaning all tools available).
// If ANY active skill has no restriction, all tools are available (nil).
func (s *Session) AllowedToolsFromSkills() []string {
	if len(s.ActiveSkills) == 0 {
		return nil
	}

	merged := make(map[string]bool)
	for _, sk := range s.ActiveSkills {
		if len(sk.AllowedTools) == 0 {
			// This skill has no restriction — all tools available.
			return nil
		}
		for _, t := range sk.AllowedTools {
			merged[t] = true
		}
	}

	// Always include skill tools themselves so agent can manage skills.
	merged["cairn.loadSkill"] = true
	merged["cairn.listSkills"] = true

	result := make([]string, 0, len(merged))
	for t := range merged {
		result = append(result, t)
	}
	sort.Strings(result)
	return result
}

// History converts session events to LLM messages for context.
// Produces the correct interleaved sequence: user → assistant (text + tool_use) → tool results → ...
func (s *Session) History() []llm.Message {
	var msgs []llm.Message
	for _, ev := range s.Events {
		if len(ev.Parts) == 0 {
			continue
		}

		if ev.Author == "user" {
			var blocks []llm.ContentBlock
			for _, p := range ev.Parts {
				if tp, ok := p.(TextPart); ok {
					blocks = append(blocks, llm.TextBlock{Text: tp.Text})
				}
			}
			if len(blocks) > 0 {
				msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: blocks})
			}
			continue
		}

		// Agent events: split into assistant blocks and tool result blocks.
		var assistantBlocks []llm.ContentBlock
		var toolResults []llm.ContentBlock

		for _, p := range ev.Parts {
			switch part := p.(type) {
			case TextPart:
				assistantBlocks = append(assistantBlocks, llm.TextBlock{Text: part.Text})
			case ReasoningPart:
				assistantBlocks = append(assistantBlocks, llm.ReasoningBlock{Text: part.Text})
			case ToolPart:
				if part.Status == ToolRunning || part.Status == ToolPending {
					assistantBlocks = append(assistantBlocks, llm.ToolUseBlock{
						ID:    part.CallID,
						Name:  part.ToolName,
						Input: part.Input,
					})
				} else {
					// ToolCompleted/ToolFailed: produce a tool result message.
					content := part.Output
					isError := part.Status == ToolFailed
					if isError && part.Error != "" {
						content = part.Error
					}
					toolResults = append(toolResults, llm.ToolResultBlock{
						ToolUseID: part.CallID,
						Content:   content,
						IsError:   isError,
					})
				}
			}
		}

		if len(assistantBlocks) > 0 {
			msgs = append(msgs, llm.Message{Role: llm.RoleAssistant, Content: assistantBlocks})
		}
		if len(toolResults) > 0 {
			msgs = append(msgs, llm.Message{Role: llm.RoleTool, Content: toolResults})
		}
	}
	return msgs
}
