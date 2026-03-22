package rules

import "time"

// Rule is a declarative automation: when trigger fires + condition true → execute actions.
type Rule struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Enabled     bool       `json:"enabled"`
	Trigger     Trigger    `json:"trigger"`
	Condition   string     `json:"condition"` // expr-lang expression, empty = always true
	Actions     []Action   `json:"actions"`
	ThrottleMs  int64      `json:"throttleMs"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	LastFiredAt *time.Time `json:"lastFiredAt,omitempty"`
}

// TriggerType enumerates trigger sources.
const (
	TriggerEvent = "event"
	TriggerCron  = "cron"
)

// Trigger defines what starts rule evaluation.
type Trigger struct {
	Type      string            `json:"type"`                // "event" or "cron"
	EventType string            `json:"eventType,omitempty"` // bus event type name
	Filter    map[string]string `json:"filter,omitempty"`    // simple key=value pre-filter
	Schedule  string            `json:"schedule,omitempty"`  // cron expression
}

// ActionType enumerates available actions.
const (
	ActionNotify = "notify"
	ActionTask   = "task"
)

// Action defines what to do when a rule fires.
type Action struct {
	Type   string            `json:"type"`   // "notify", "task"
	Params map[string]string `json:"params"` // action-specific parameters
}

// Execution records a single rule fire.
type Execution struct {
	ID           string    `json:"id"`
	RuleID       string    `json:"ruleId"`
	TriggerEvent string    `json:"triggerEvent,omitempty"`
	Status       string    `json:"status"` // "success", "error", "throttled", "condition_false"
	Error        string    `json:"error,omitempty"`
	DurationMs   int64     `json:"durationMs"`
	CreatedAt    time.Time `json:"createdAt"`
}

// UpdateOpts holds optional fields for updating a rule.
type UpdateOpts struct {
	Enabled     *bool
	Name        *string
	Description *string
	Trigger     *Trigger
	Condition   *string
	Actions     []Action // nil = no change
	ThrottleMs  *int64
}
