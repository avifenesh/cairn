package llm

import (
	"fmt"
	"sync"
	"time"
)

// Budget tracks LLM spending with daily and weekly limits.
// Thread-safe. Auto-resets on day/week boundaries.
type Budget struct {
	DailyLimit  float64
	WeeklyLimit float64

	mu           sync.Mutex
	dailySpent   float64
	weeklySpent  float64
	lastResetDay int // day of year for daily reset
	lastResetW   int // ISO week for weekly reset

	// Cost lookup: model → cost per 1M tokens (input, output).
	costs map[string]ModelCost
}

// ModelCost is the cost per 1M tokens for a model.
type ModelCost struct {
	Per1MInput  float64
	Per1MOutput float64
}

// NewBudget creates a budget tracker with the given limits.
// Pass 0 for a limit to disable that check.
func NewBudget(dailyLimit, weeklyLimit float64) *Budget {
	now := time.Now()
	_, week := now.ISOWeek()
	return &Budget{
		DailyLimit:   dailyLimit,
		WeeklyLimit:  weeklyLimit,
		lastResetDay: now.YearDay(),
		lastResetW:   week,
		costs:        defaultCosts(),
	}
}

// SetModelCost sets or overrides the cost for a model.
func (b *Budget) SetModelCost(model string, cost ModelCost) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.costs[model] = cost
}

// CanAfford checks if a request can be sent within budget.
// estInputTokens is the estimated input token count.
func (b *Budget) CanAfford(model string, estInputTokens int) bool {
	if b.DailyLimit == 0 && b.WeeklyLimit == 0 {
		return true // no limits
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.resetIfNeeded()

	cost := b.estimateCost(model, estInputTokens, 0)

	if b.DailyLimit > 0 && b.dailySpent+cost > b.DailyLimit {
		return false
	}
	if b.WeeklyLimit > 0 && b.weeklySpent+cost > b.WeeklyLimit {
		return false
	}
	return true
}

// Record records actual token usage after a response completes.
func (b *Budget) Record(model string, inputTokens, outputTokens int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.resetIfNeeded()

	cost := b.estimateCost(model, inputTokens, outputTokens)
	b.dailySpent += cost
	b.weeklySpent += cost
}

// MidStreamCheck returns true if the estimated remaining output cost
// would stay within budget. Used to abort long responses.
func (b *Budget) MidStreamCheck(model string, estRemainingOutputTokens int) bool {
	if b.DailyLimit == 0 && b.WeeklyLimit == 0 {
		return true
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	cost := b.estimateCost(model, 0, estRemainingOutputTokens)

	if b.DailyLimit > 0 && b.dailySpent+cost > b.DailyLimit {
		return false
	}
	if b.WeeklyLimit > 0 && b.weeklySpent+cost > b.WeeklyLimit {
		return false
	}
	return true
}

// Spent returns current daily and weekly spend.
func (b *Budget) Spent() (daily, weekly float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.resetIfNeeded()
	return b.dailySpent, b.weeklySpent
}

// String returns a human-readable budget status.
func (b *Budget) String() string {
	daily, weekly := b.Spent()
	return fmt.Sprintf("daily: $%.4f/$%.2f, weekly: $%.4f/$%.2f",
		daily, b.DailyLimit, weekly, b.WeeklyLimit)
}

func (b *Budget) estimateCost(model string, inputTokens, outputTokens int) float64 {
	mc, ok := b.costs[model]
	if !ok {
		return 0 // unknown model = free (subscription models like GLM)
	}
	inputCost := float64(inputTokens) / 1_000_000 * mc.Per1MInput
	outputCost := float64(outputTokens) / 1_000_000 * mc.Per1MOutput
	return inputCost + outputCost
}

func (b *Budget) resetIfNeeded() {
	now := time.Now()

	if now.YearDay() != b.lastResetDay {
		b.dailySpent = 0
		b.lastResetDay = now.YearDay()
	}

	_, week := now.ISOWeek()
	if week != b.lastResetW {
		b.weeklySpent = 0
		b.lastResetW = week
	}
}

func defaultCosts() map[string]ModelCost {
	return map[string]ModelCost{
		// GLM — subscription, no per-token cost
		"glm-5-turbo": {Per1MInput: 0, Per1MOutput: 0},
		"glm-4.7":     {Per1MInput: 0, Per1MOutput: 0},

		// OpenAI
		"gpt-4o":        {Per1MInput: 2.50, Per1MOutput: 10.00},
		"gpt-4o-mini":   {Per1MInput: 0.15, Per1MOutput: 0.60},
		"gpt-4-turbo":   {Per1MInput: 10.00, Per1MOutput: 30.00},
		"gpt-3.5-turbo": {Per1MInput: 0.50, Per1MOutput: 1.50},

		// Anthropic (for future)
		"claude-sonnet-4-6": {Per1MInput: 3.00, Per1MOutput: 15.00},
		"claude-haiku-4-5":  {Per1MInput: 0.80, Per1MOutput: 4.00},
		"claude-opus-4-6":   {Per1MInput: 15.00, Per1MOutput: 75.00},
	}
}
