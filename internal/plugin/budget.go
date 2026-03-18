package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// BudgetPlugin enforces daily/weekly LLM spend limits.
// Checks budget before each LLM call and aborts if exceeded.
// Based on Gollem's budget tracker pattern.
type BudgetPlugin struct {
	dailyCap  float64
	weeklyCap float64
	logger    *slog.Logger

	mu          sync.Mutex
	dailySpend  float64
	weeklySpend float64
	dayStart    time.Time
	weekStart   time.Time

	totalCalls atomic.Int64
	blocked    atomic.Int64
}

// BudgetConfig configures spend limits.
type BudgetConfig struct {
	DailyCap  float64 // Max daily spend in dollars (0 = unlimited)
	WeeklyCap float64 // Max weekly spend in dollars (0 = unlimited)
}

// NewBudgetPlugin creates a budget enforcement plugin.
func NewBudgetPlugin(cfg BudgetConfig, logger *slog.Logger) *BudgetPlugin {
	if logger == nil {
		logger = slog.Default()
	}
	now := time.Now().UTC()
	return &BudgetPlugin{
		dailyCap:  cfg.DailyCap,
		weeklyCap: cfg.WeeklyCap,
		logger:    logger,
		dayStart:  startOfDay(now),
		weekStart: startOfWeek(now),
	}
}

func (p *BudgetPlugin) Name() string { return "budget" }

// Stats returns current budget state for API/dashboard.
func (p *BudgetPlugin) Stats() BudgetStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.maybeReset()
	return BudgetStats{
		DailySpend:  p.dailySpend,
		DailyCap:    p.dailyCap,
		WeeklySpend: p.weeklySpend,
		WeeklyCap:   p.weeklyCap,
		TotalCalls:  p.totalCalls.Load(),
		Blocked:     p.blocked.Load(),
	}
}

// BudgetStats holds current spend state.
type BudgetStats struct {
	DailySpend  float64
	DailyCap    float64
	WeeklySpend float64
	WeeklyCap   float64
	TotalCalls  int64
	Blocked     int64
}

// --- LLMHooks ---

func (p *BudgetPlugin) BeforeLLMCall(ctx context.Context, call *LLMCall) (context.Context, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.maybeReset()

	if p.dailyCap > 0 && p.dailySpend >= p.dailyCap {
		p.blocked.Add(1)
		return ctx, fmt.Errorf("budget: daily cap exceeded (%.2f/%.2f)", p.dailySpend, p.dailyCap)
	}
	if p.weeklyCap > 0 && p.weeklySpend >= p.weeklyCap {
		p.blocked.Add(1)
		return ctx, fmt.Errorf("budget: weekly cap exceeded (%.2f/%.2f)", p.weeklySpend, p.weeklyCap)
	}
	return ctx, nil
}

func (p *BudgetPlugin) AfterLLMCall(ctx context.Context, call *LLMCall, usage *TokenUsage) context.Context {
	cost := estimateCost(call.Model, usage.InputTokens, usage.OutputTokens)
	p.mu.Lock()
	p.dailySpend += cost
	p.weeklySpend += cost
	p.mu.Unlock()
	p.totalCalls.Add(1)

	if cost > 0 {
		p.logger.Debug("budget: llm cost",
			"model", call.Model,
			"cost", cost,
			"dailySpend", p.dailySpend,
			"dailyCap", p.dailyCap)
	}
	return ctx
}

func (p *BudgetPlugin) OnLLMError(ctx context.Context, call *LLMCall, err error) context.Context {
	return ctx
}

// maybeReset resets counters when day/week rolls over. Must be called with mu held.
func (p *BudgetPlugin) maybeReset() {
	now := time.Now().UTC()
	dayStart := startOfDay(now)
	if dayStart.After(p.dayStart) {
		p.dailySpend = 0
		p.dayStart = dayStart
	}
	weekStart := startOfWeek(now)
	if weekStart.After(p.weekStart) {
		p.weeklySpend = 0
		p.weekStart = weekStart
	}
}

// estimateCost returns approximate cost in dollars for a model call.
// Per-million token pricing. GLM is $0 (subscription).
func estimateCost(model string, inputTokens, outputTokens int) float64 {
	type rate struct{ input, output float64 } // per million tokens
	rates := map[string]rate{
		"glm-5-turbo":       {0, 0}, // Z.ai subscription
		"glm-5":             {0, 0},
		"glm-4.7":           {0, 0},
		"gpt-4o":            {2.5, 10},
		"gpt-4o-mini":       {0.15, 0.6},
		"claude-sonnet-4-6": {3, 15},
		"claude-opus-4-6":   {15, 75},
		"claude-haiku-4-5":  {0.8, 4},
	}

	r, ok := rates[model]
	if !ok {
		return 0 // unknown model, assume free
	}
	return float64(inputTokens)/1_000_000*r.input + float64(outputTokens)/1_000_000*r.output
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func startOfWeek(t time.Time) time.Time {
	for t.Weekday() != time.Monday {
		t = t.AddDate(0, 0, -1)
	}
	return startOfDay(t)
}

// Verify interface compliance.
var _ LLMHooks = (*BudgetPlugin)(nil)
