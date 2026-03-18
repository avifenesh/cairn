// Package plugin provides lifecycle hooks for extending the agent loop,
// tool execution, and LLM calls. Based on ADK-Go's plugin system with
// Eino's context-returning handler pattern.
//
// Plugins implement optional hook interfaces. The Manager runs hooks in
// registration order. Each hook receives and returns context.Context for
// state propagation (Eino pattern). First non-nil error stops the chain
// (ADK-Go pattern).
package plugin

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/avifenesh/cairn/internal/tool"
)

// Plugin is the base interface. Implement only the hook interfaces you need.
type Plugin interface {
	Name() string
}

// --- Agent-level hooks ---

// AgentHooks intercepts the agent run lifecycle.
type AgentHooks interface {
	// BeforeAgentRun fires before the ReAct loop starts. Return error to abort.
	BeforeAgentRun(ctx context.Context, inv *Invocation) (context.Context, error)
	// AfterAgentRun fires after the ReAct loop completes successfully.
	AfterAgentRun(ctx context.Context, inv *Invocation, result *RunResult) context.Context
	// OnAgentError fires when the agent run fails.
	OnAgentError(ctx context.Context, inv *Invocation, err error) context.Context
}

// --- Tool-level hooks ---

// ToolHooks intercepts tool execution.
type ToolHooks interface {
	// BeforeToolCall fires before a tool executes. Return error to block.
	BeforeToolCall(ctx context.Context, call *ToolCall) (context.Context, error)
	// AfterToolCall fires after a tool completes successfully.
	AfterToolCall(ctx context.Context, call *ToolCall, result *ToolResult) context.Context
	// OnToolError fires when a tool fails.
	OnToolError(ctx context.Context, call *ToolCall, err error) context.Context
}

// --- LLM-level hooks ---

// LLMHooks intercepts LLM calls.
type LLMHooks interface {
	// BeforeLLMCall fires before an LLM request. Return error to abort.
	BeforeLLMCall(ctx context.Context, call *LLMCall) (context.Context, error)
	// AfterLLMCall fires after an LLM response is received.
	AfterLLMCall(ctx context.Context, call *LLMCall, usage *TokenUsage) context.Context
	// OnLLMError fires when an LLM call fails.
	OnLLMError(ctx context.Context, call *LLMCall, err error) context.Context
}

// --- Hook data types ---

// Invocation carries per-run metadata.
type Invocation struct {
	SessionID   string
	UserMessage string
	Mode        tool.Mode
	Model       string
	StartedAt   time.Time
}

// RunResult carries agent run outcome.
type RunResult struct {
	Rounds     int
	ToolCalls  int
	DurationMs int64
}

// ToolCall carries tool execution context.
type ToolCall struct {
	Name  string
	Input json.RawMessage
}

// ToolResult carries tool output.
type ToolResult struct {
	Output   string
	Duration time.Duration
}

// LLMCall carries LLM request context.
type LLMCall struct {
	Model string
	Round int
}

// TokenUsage from an LLM response.
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
	Model        string
}

// --- Manager ---

// Manager holds registered plugins and runs hooks in order.
// Thread-safe for concurrent hook execution (plugins are read-only after registration).
type Manager struct {
	plugins []Plugin
	logger  *slog.Logger
}

// NewManager creates a plugin manager.
func NewManager(logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{logger: logger}
}

// Register adds a plugin. Call before starting the agent loop.
func (m *Manager) Register(p Plugin) {
	m.plugins = append(m.plugins, p)
	m.logger.Info("plugin registered", "name", p.Name())
}

// Plugins returns the registered plugins (for inspection/testing).
func (m *Manager) Plugins() []Plugin {
	return m.plugins
}

// --- Agent hook runners ---

// RunBeforeAgentRun executes BeforeAgentRun on all plugins with AgentHooks.
// First error stops the chain and returns it.
func (m *Manager) RunBeforeAgentRun(ctx context.Context, inv *Invocation) (context.Context, error) {
	for _, p := range m.plugins {
		if h, ok := p.(AgentHooks); ok {
			var err error
			ctx, err = h.BeforeAgentRun(ctx, inv)
			if err != nil {
				m.logger.Warn("plugin: BeforeAgentRun aborted", "plugin", p.Name(), "error", err)
				return ctx, err
			}
		}
	}
	return ctx, nil
}

// RunAfterAgentRun executes AfterAgentRun on all plugins with AgentHooks.
func (m *Manager) RunAfterAgentRun(ctx context.Context, inv *Invocation, result *RunResult) context.Context {
	for _, p := range m.plugins {
		if h, ok := p.(AgentHooks); ok {
			ctx = h.AfterAgentRun(ctx, inv, result)
		}
	}
	return ctx
}

// RunOnAgentError executes OnAgentError on all plugins with AgentHooks.
func (m *Manager) RunOnAgentError(ctx context.Context, inv *Invocation, err error) context.Context {
	for _, p := range m.plugins {
		if h, ok := p.(AgentHooks); ok {
			ctx = h.OnAgentError(ctx, inv, err)
		}
	}
	return ctx
}

// --- Tool hook runners ---

// RunBeforeToolCall executes BeforeToolCall on all plugins with ToolHooks.
func (m *Manager) RunBeforeToolCall(ctx context.Context, call *ToolCall) (context.Context, error) {
	for _, p := range m.plugins {
		if h, ok := p.(ToolHooks); ok {
			var err error
			ctx, err = h.BeforeToolCall(ctx, call)
			if err != nil {
				m.logger.Warn("plugin: BeforeToolCall blocked", "plugin", p.Name(), "tool", call.Name, "error", err)
				return ctx, err
			}
		}
	}
	return ctx, nil
}

// RunAfterToolCall executes AfterToolCall on all plugins with ToolHooks.
func (m *Manager) RunAfterToolCall(ctx context.Context, call *ToolCall, result *ToolResult) context.Context {
	for _, p := range m.plugins {
		if h, ok := p.(ToolHooks); ok {
			ctx = h.AfterToolCall(ctx, call, result)
		}
	}
	return ctx
}

// RunOnToolError executes OnToolError on all plugins with ToolHooks.
func (m *Manager) RunOnToolError(ctx context.Context, call *ToolCall, err error) context.Context {
	for _, p := range m.plugins {
		if h, ok := p.(ToolHooks); ok {
			ctx = h.OnToolError(ctx, call, err)
		}
	}
	return ctx
}

// --- LLM hook runners ---

// RunBeforeLLMCall executes BeforeLLMCall on all plugins with LLMHooks.
func (m *Manager) RunBeforeLLMCall(ctx context.Context, call *LLMCall) (context.Context, error) {
	for _, p := range m.plugins {
		if h, ok := p.(LLMHooks); ok {
			var err error
			ctx, err = h.BeforeLLMCall(ctx, call)
			if err != nil {
				m.logger.Warn("plugin: BeforeLLMCall aborted", "plugin", p.Name(), "model", call.Model, "error", err)
				return ctx, err
			}
		}
	}
	return ctx, nil
}

// RunAfterLLMCall executes AfterLLMCall on all plugins with LLMHooks.
func (m *Manager) RunAfterLLMCall(ctx context.Context, call *LLMCall, usage *TokenUsage) context.Context {
	for _, p := range m.plugins {
		if h, ok := p.(LLMHooks); ok {
			ctx = h.AfterLLMCall(ctx, call, usage)
		}
	}
	return ctx
}

// RunOnLLMError executes OnLLMError on all plugins with LLMHooks.
func (m *Manager) RunOnLLMError(ctx context.Context, call *LLMCall, err error) context.Context {
	for _, p := range m.plugins {
		if h, ok := p.(LLMHooks); ok {
			ctx = h.OnLLMError(ctx, call, err)
		}
	}
	return ctx
}
