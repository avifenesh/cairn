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

// AgentConfig holds per-invocation agent configuration.
type AgentConfig struct {
	Model     string
	MaxRounds int
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
func (s *Session) History() []llm.Message {
	var msgs []llm.Message
	for _, ev := range s.Events {
		msg := eventToMessage(ev)
		if msg != nil {
			msgs = append(msgs, *msg)
		}
	}
	return msgs
}

// eventToMessage converts an agent Event to an LLM Message.
func eventToMessage(ev *Event) *llm.Message {
	if len(ev.Parts) == 0 {
		return nil
	}

	// Determine role from author.
	var role llm.Role
	if ev.Author == "user" {
		role = llm.RoleUser
	} else {
		role = llm.RoleAssistant
	}

	var blocks []llm.ContentBlock
	for _, part := range ev.Parts {
		switch p := part.(type) {
		case TextPart:
			blocks = append(blocks, llm.TextBlock{Text: p.Text})
		case ToolPart:
			if role == llm.RoleAssistant && p.Status == ToolCompleted {
				// Assistant requested the tool call.
				blocks = append(blocks, llm.ToolUseBlock{
					ID:    p.CallID,
					Name:  p.ToolName,
					Input: p.Input,
				})
			}
		case ReasoningPart:
			blocks = append(blocks, llm.ReasoningBlock{Text: p.Text})
		}
	}

	if len(blocks) == 0 {
		return nil
	}

	return &llm.Message{Role: role, Content: blocks}
}
