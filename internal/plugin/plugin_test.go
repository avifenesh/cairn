package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/avifenesh/cairn/internal/tool"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// --- Manager tests ---

func TestManager_RegisterAndList(t *testing.T) {
	m := NewManager(discardLogger())
	m.Register(NewLoggingPlugin(discardLogger()))

	if len(m.Plugins()) != 1 {
		t.Errorf("plugins = %d, want 1", len(m.Plugins()))
	}
	if m.Plugins()[0].Name() != "logging" {
		t.Errorf("name = %q, want 'logging'", m.Plugins()[0].Name())
	}
}

func TestManager_BeforeAgentRun_Order(t *testing.T) {
	m := NewManager(discardLogger())

	var order []string
	m.Register(&orderPlugin{name: "first", onBefore: func() { order = append(order, "first") }})
	m.Register(&orderPlugin{name: "second", onBefore: func() { order = append(order, "second") }})

	inv := &Invocation{SessionID: "s1", Mode: tool.ModeTalk}
	_, err := m.RunBeforeAgentRun(context.Background(), inv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Errorf("execution order = %v, want [first, second]", order)
	}
}

func TestManager_BeforeAgentRun_EarlyExit(t *testing.T) {
	m := NewManager(discardLogger())

	var order []string
	m.Register(&orderPlugin{name: "blocker", onBefore: func() { order = append(order, "blocker") }, err: errors.New("blocked")})
	m.Register(&orderPlugin{name: "skipped", onBefore: func() { order = append(order, "skipped") }})

	inv := &Invocation{SessionID: "s1", Mode: tool.ModeTalk}
	_, err := m.RunBeforeAgentRun(context.Background(), inv)

	if err == nil {
		t.Fatal("expected error from blocker")
	}
	if len(order) != 1 || order[0] != "blocker" {
		t.Errorf("expected only blocker to run, got %v", order)
	}
}

func TestManager_ContextPropagation(t *testing.T) {
	m := NewManager(discardLogger())

	type ctxKey struct{}
	m.Register(&ctxPlugin{
		name: "setter",
		beforeAgent: func(ctx context.Context, _ *Invocation) (context.Context, error) {
			return context.WithValue(ctx, ctxKey{}, "hello"), nil
		},
	})
	m.Register(&ctxPlugin{
		name: "reader",
		beforeAgent: func(ctx context.Context, _ *Invocation) (context.Context, error) {
			val, ok := ctx.Value(ctxKey{}).(string)
			if !ok || val != "hello" {
				t.Error("expected context value 'hello' from previous plugin")
			}
			return ctx, nil
		},
	})

	inv := &Invocation{SessionID: "s1"}
	m.RunBeforeAgentRun(context.Background(), inv)
}

func TestManager_ToolHooks(t *testing.T) {
	m := NewManager(discardLogger())
	m.Register(NewLoggingPlugin(discardLogger()))

	call := &ToolCall{Name: "readFile", Input: json.RawMessage(`{"path":"main.go"}`)}
	ctx, err := m.RunBeforeToolCall(context.Background(), call)
	if err != nil {
		t.Fatalf("before tool: %v", err)
	}

	result := &ToolResult{Output: "file contents", Duration: 10 * time.Millisecond}
	ctx = m.RunAfterToolCall(ctx, call, result)
	_ = ctx
}

func TestManager_LLMHooks(t *testing.T) {
	m := NewManager(discardLogger())
	m.Register(NewLoggingPlugin(discardLogger()))

	call := &LLMCall{Model: "glm-5-turbo", Round: 1}
	ctx, err := m.RunBeforeLLMCall(context.Background(), call)
	if err != nil {
		t.Fatalf("before llm: %v", err)
	}

	usage := &TokenUsage{InputTokens: 100, OutputTokens: 50, Model: "glm-5-turbo"}
	ctx = m.RunAfterLLMCall(ctx, call, usage)
	_ = ctx
}

// --- Budget plugin tests ---

func TestBudgetPlugin_AllowsUnderCap(t *testing.T) {
	bp := NewBudgetPlugin(BudgetConfig{DailyCap: 10.0}, discardLogger())
	call := &LLMCall{Model: "gpt-4o", Round: 1}

	_, err := bp.BeforeLLMCall(context.Background(), call)
	if err != nil {
		t.Fatalf("expected no error under cap: %v", err)
	}
}

func TestBudgetPlugin_BlocksOverCap(t *testing.T) {
	bp := NewBudgetPlugin(BudgetConfig{DailyCap: 0.001}, discardLogger())

	// Simulate a call that costs money.
	call := &LLMCall{Model: "gpt-4o", Round: 1}
	bp.AfterLLMCall(context.Background(), call, &TokenUsage{InputTokens: 1000000, OutputTokens: 500000, Model: "gpt-4o"})

	// Next call should be blocked.
	_, err := bp.BeforeLLMCall(context.Background(), call)
	if err == nil {
		t.Error("expected budget error after exceeding cap")
	}
}

func TestBudgetPlugin_GLMFree(t *testing.T) {
	bp := NewBudgetPlugin(BudgetConfig{DailyCap: 0.001}, discardLogger())
	call := &LLMCall{Model: "glm-5-turbo", Round: 1}

	// Even huge GLM calls should be free.
	bp.AfterLLMCall(context.Background(), call, &TokenUsage{InputTokens: 10000000, OutputTokens: 5000000, Model: "glm-5-turbo"})

	_, err := bp.BeforeLLMCall(context.Background(), call)
	if err != nil {
		t.Errorf("GLM should be free: %v", err)
	}
}

func TestBudgetPlugin_Stats(t *testing.T) {
	bp := NewBudgetPlugin(BudgetConfig{DailyCap: 100, WeeklyCap: 500}, discardLogger())
	call := &LLMCall{Model: "gpt-4o", Round: 1}
	bp.AfterLLMCall(context.Background(), call, &TokenUsage{InputTokens: 1000, OutputTokens: 500, Model: "gpt-4o"})

	stats := bp.Stats()
	if stats.DailyCap != 100 {
		t.Errorf("dailyCap = %f, want 100", stats.DailyCap)
	}
	if stats.TotalCalls != 1 {
		t.Errorf("totalCalls = %d, want 1", stats.TotalCalls)
	}
	if stats.DailySpend <= 0 {
		t.Error("expected non-zero daily spend")
	}
}

func TestBudgetPlugin_UnlimitedWhenZero(t *testing.T) {
	bp := NewBudgetPlugin(BudgetConfig{DailyCap: 0, WeeklyCap: 0}, discardLogger())
	call := &LLMCall{Model: "claude-opus-4-6", Round: 1}

	// Simulate expensive call.
	bp.AfterLLMCall(context.Background(), call, &TokenUsage{InputTokens: 10000000, OutputTokens: 5000000, Model: "claude-opus-4-6"})

	_, err := bp.BeforeLLMCall(context.Background(), call)
	if err != nil {
		t.Errorf("expected no limit with zero cap: %v", err)
	}
}

func TestEstimateCost(t *testing.T) {
	// GPT-4o: $2.5 input, $10 output per million
	cost := estimateCost("gpt-4o", 1_000_000, 1_000_000)
	if cost != 12.5 {
		t.Errorf("gpt-4o 1M/1M = %f, want 12.5", cost)
	}

	// GLM: free
	cost = estimateCost("glm-5-turbo", 1_000_000, 1_000_000)
	if cost != 0 {
		t.Errorf("glm-5-turbo = %f, want 0", cost)
	}

	// Unknown: free
	cost = estimateCost("unknown-model", 1_000_000, 1_000_000)
	if cost != 0 {
		t.Errorf("unknown = %f, want 0", cost)
	}
}

// --- Test helpers ---

type orderPlugin struct {
	name     string
	onBefore func()
	err      error
}

func (p *orderPlugin) Name() string { return p.name }

func (p *orderPlugin) BeforeAgentRun(ctx context.Context, _ *Invocation) (context.Context, error) {
	if p.onBefore != nil {
		p.onBefore()
	}
	return ctx, p.err
}
func (p *orderPlugin) AfterAgentRun(ctx context.Context, _ *Invocation, _ *RunResult) context.Context {
	return ctx
}
func (p *orderPlugin) OnAgentError(ctx context.Context, _ *Invocation, _ error) context.Context {
	return ctx
}

type ctxPlugin struct {
	name        string
	beforeAgent func(context.Context, *Invocation) (context.Context, error)
}

func (p *ctxPlugin) Name() string { return p.name }
func (p *ctxPlugin) BeforeAgentRun(ctx context.Context, inv *Invocation) (context.Context, error) {
	if p.beforeAgent != nil {
		return p.beforeAgent(ctx, inv)
	}
	return ctx, nil
}
func (p *ctxPlugin) AfterAgentRun(ctx context.Context, _ *Invocation, _ *RunResult) context.Context {
	return ctx
}
func (p *ctxPlugin) OnAgentError(ctx context.Context, _ *Invocation, _ error) context.Context {
	return ctx
}
