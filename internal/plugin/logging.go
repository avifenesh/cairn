package plugin

import (
	"context"
	"log/slog"
	"time"
)

// LoggingPlugin logs agent, tool, and LLM lifecycle events.
// Based on ADK-Go's loggingplugin.
type LoggingPlugin struct {
	logger *slog.Logger
}

// NewLoggingPlugin creates a logging plugin.
func NewLoggingPlugin(logger *slog.Logger) *LoggingPlugin {
	if logger == nil {
		logger = slog.Default()
	}
	return &LoggingPlugin{logger: logger}
}

func (p *LoggingPlugin) Name() string { return "logging" }

// --- AgentHooks ---

func (p *LoggingPlugin) BeforeAgentRun(ctx context.Context, inv *Invocation) (context.Context, error) {
	p.logger.Info("agent run started",
		"session", inv.SessionID,
		"mode", inv.Mode,
		"model", inv.Model,
		"messageLen", len(inv.UserMessage))
	return context.WithValue(ctx, agentStartKey{}, time.Now()), nil
}

func (p *LoggingPlugin) AfterAgentRun(ctx context.Context, inv *Invocation, result *RunResult) context.Context {
	args := []any{
		"session", inv.SessionID,
		"rounds", result.Rounds,
		"toolCalls", result.ToolCalls,
	}
	if start, ok := ctx.Value(agentStartKey{}).(time.Time); ok {
		args = append(args, "duration", time.Since(start))
	}
	p.logger.Info("agent run completed", args...)
	return ctx
}

func (p *LoggingPlugin) OnAgentError(ctx context.Context, inv *Invocation, err error) context.Context {
	p.logger.Error("agent run failed",
		"session", inv.SessionID,
		"error", err)
	return ctx
}

// --- ToolHooks ---

func (p *LoggingPlugin) BeforeToolCall(ctx context.Context, call *ToolCall) (context.Context, error) {
	p.logger.Debug("tool call started", "tool", call.Name)
	return context.WithValue(ctx, toolStartKey{}, time.Now()), nil
}

func (p *LoggingPlugin) AfterToolCall(ctx context.Context, call *ToolCall, result *ToolResult) context.Context {
	p.logger.Info("tool call completed",
		"tool", call.Name,
		"duration", result.Duration,
		"outputLen", len(result.Output))
	return ctx
}

func (p *LoggingPlugin) OnToolError(ctx context.Context, call *ToolCall, err error) context.Context {
	p.logger.Warn("tool call failed", "tool", call.Name, "error", err)
	return ctx
}

// --- LLMHooks ---

func (p *LoggingPlugin) BeforeLLMCall(ctx context.Context, call *LLMCall) (context.Context, error) {
	p.logger.Debug("llm call started", "model", call.Model, "round", call.Round)
	return context.WithValue(ctx, llmStartKey{}, time.Now()), nil
}

func (p *LoggingPlugin) AfterLLMCall(ctx context.Context, call *LLMCall, usage *TokenUsage) context.Context {
	args := []any{
		"model", call.Model,
		"round", call.Round,
		"inputTokens", usage.InputTokens,
		"outputTokens", usage.OutputTokens,
	}
	if start, ok := ctx.Value(llmStartKey{}).(time.Time); ok {
		args = append(args, "duration", time.Since(start))
	}
	p.logger.Info("llm call completed", args...)
	return ctx
}

func (p *LoggingPlugin) OnLLMError(ctx context.Context, call *LLMCall, err error) context.Context {
	p.logger.Error("llm call failed", "model", call.Model, "round", call.Round, "error", err)
	return ctx
}

// Context keys for timing state (Eino pattern: state flows via context).
type agentStartKey struct{}
type toolStartKey struct{}
type llmStartKey struct{}

// Verify interface compliance at compile time.
var (
	_ AgentHooks = (*LoggingPlugin)(nil)
	_ ToolHooks  = (*LoggingPlugin)(nil)
	_ LLMHooks   = (*LoggingPlugin)(nil)
)
