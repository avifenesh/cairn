package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"

	"github.com/avifenesh/cairn/internal/eventbus"
)

// NotifyService sends notifications to configured channels.
type NotifyService interface {
	Notify(ctx context.Context, message string, priority int) error
}

// TaskSubmitter submits tasks to the agent task queue.
type TaskSubmitter interface {
	Submit(ctx context.Context, description, taskType string, priority int) (string, error)
}

// EngineDeps holds dependencies for the rules engine.
type EngineDeps struct {
	Store    *Store
	Bus      *eventbus.Bus
	Notifier NotifyService
	Tasks    TaskSubmitter
	Logger   *slog.Logger
}

// compiledRule is a rule with its pre-compiled expr condition.
type compiledRule struct {
	Rule
	program *vm.Program // nil if condition is empty
}

// Engine subscribes to bus events, evaluates rules, and dispatches actions.
type Engine struct {
	store    *Store
	bus      *eventbus.Bus
	notifier NotifyService
	tasks    TaskSubmitter
	logger   *slog.Logger
	unsubs   []func()
	cache    []compiledRule
	cacheMu  sync.RWMutex

	// Goroutine limiter: max concurrent rule executions.
	sem chan struct{}
}

const maxConcurrentExecs = 10

// NewEngine creates a rules engine.
func NewEngine(deps EngineDeps) *Engine {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{
		store:    deps.Store,
		bus:      deps.Bus,
		notifier: deps.Notifier,
		tasks:    deps.Tasks,
		logger:   logger,
		sem:      make(chan struct{}, maxConcurrentExecs),
	}
}

// Start subscribes to bus events and begins rule evaluation.
func (e *Engine) Start() {
	e.refreshCache()

	e.unsubs = append(e.unsubs,
		eventbus.Subscribe(e.bus, func(ev eventbus.EventIngested) {
			e.handleEvent("EventIngested", map[string]any{
				"source":     ev.Source,
				"sourceType": ev.SourceType,
				"kind":       ev.Kind,
				"title":      ev.Title,
				"url":        ev.URL,
				"actor":      ev.Actor,
				"repo":       ev.Repo,
			})
		}),
		eventbus.Subscribe(e.bus, func(ev eventbus.TaskCreated) {
			e.handleEvent("TaskCreated", map[string]any{
				"taskId":      ev.TaskID,
				"type":        ev.Type,
				"description": ev.Description,
			})
		}),
		eventbus.Subscribe(e.bus, func(ev eventbus.TaskCompleted) {
			e.handleEvent("TaskCompleted", map[string]any{
				"taskId": ev.TaskID,
				"result": ev.Result,
			})
		}),
		eventbus.Subscribe(e.bus, func(ev eventbus.TaskFailed) {
			e.handleEvent("TaskFailed", map[string]any{
				"taskId": ev.TaskID,
				"error":  ev.Error,
			})
		}),
		eventbus.Subscribe(e.bus, func(ev eventbus.MemoryProposed) {
			e.handleEvent("MemoryProposed", map[string]any{
				"memoryId": ev.MemoryID,
				"content":  ev.Content,
			})
		}),
	)

	e.logger.Info("rules engine started", "rules", len(e.cache))
}

// Close unsubscribes from all bus events.
func (e *Engine) Close() {
	for _, unsub := range e.unsubs {
		unsub()
	}
	e.unsubs = nil
	e.logger.Info("rules engine closed")
}

// RefreshCache reloads enabled rules from the store and pre-compiles conditions.
func (e *Engine) RefreshCache() {
	e.refreshCache()
}

func (e *Engine) refreshCache() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rules, err := e.store.ListEnabled(ctx)
	if err != nil {
		e.logger.Warn("rules: failed to refresh cache", "error", err)
		return
	}

	compiled := make([]compiledRule, 0, len(rules))
	for _, r := range rules {
		cr := compiledRule{Rule: *r}
		if r.Condition != "" {
			program, err := expr.Compile(r.Condition, expr.AsBool())
			if err != nil {
				e.logger.Warn("rules: failed to compile condition", "rule", r.Name, "error", err)
				continue // skip rules with invalid conditions
			}
			cr.program = program
		}
		compiled = append(compiled, cr)
	}

	e.cacheMu.Lock()
	e.cache = compiled
	e.cacheMu.Unlock()
}

// Stats returns summary statistics for observations.
func (e *Engine) Stats() (total, enabled, recentFailures int) {
	e.cacheMu.RLock()
	enabled = len(e.cache)
	e.cacheMu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if n, err := e.store.Count(ctx); err == nil {
		total = n
	}

	// Count failures from the last ~50 execution window (approximate with 1h lookback).
	if n, err := e.store.CountFailedExecutions(ctx, time.Now().Add(-1*time.Hour)); err == nil {
		recentFailures = n
	}
	return
}

// handleEvent processes a bus event against all cached rules.
func (e *Engine) handleEvent(eventType string, data map[string]any) {
	e.cacheMu.RLock()
	rules := make([]compiledRule, len(e.cache))
	copy(rules, e.cache) // snapshot to avoid data race on slice elements
	e.cacheMu.RUnlock()

	for _, rule := range rules {
		if rule.Trigger.Type != TriggerEvent {
			continue
		}
		if rule.Trigger.EventType != eventType {
			continue
		}
		if !matchFilter(rule.Trigger.Filter, data) {
			continue
		}
		// Throttle check.
		if rule.ThrottleMs > 0 && rule.LastFiredAt != nil {
			if time.Since(*rule.LastFiredAt).Milliseconds() < rule.ThrottleMs {
				e.recordExec(rule.ID, rule.Name, data, ExecThrottled, nil, 0)
				continue
			}
		}

		// Rate-limited goroutine spawning.
		r := rule // capture for goroutine
		select {
		case e.sem <- struct{}{}:
			go func() {
				defer func() { <-e.sem }()
				e.evaluateAndExecute(r, data)
			}()
		default:
			e.logger.Warn("rules: execution semaphore full, skipping", "rule", rule.Name)
			e.recordExec(rule.ID, rule.Name, data, ExecBackpressure, nil, 0)
		}
	}
}

func (e *Engine) evaluateAndExecute(rule compiledRule, data map[string]any) {
	start := time.Now()

	// Evaluate pre-compiled condition.
	if rule.program != nil {
		result, err := expr.Run(rule.program, data)
		if err != nil {
			e.logger.Warn("rules: condition eval error", "rule", rule.Name, "error", err)
			e.recordExec(rule.ID, rule.Name, data, ExecError, err, time.Since(start).Milliseconds())
			return
		}
		b, ok := result.(bool)
		if !ok || !b {
			e.recordExec(rule.ID, rule.Name, data, ExecConditionFalse, nil, time.Since(start).Milliseconds())
			return
		}
	}

	// Execute actions sequentially.
	for _, action := range rule.Actions {
		if err := e.dispatchAction(action, data); err != nil {
			e.logger.Warn("rules: action failed", "rule", rule.Name, "action", action.Type, "error", err)
			e.recordExec(rule.ID, rule.Name, data, ExecError, err, time.Since(start).Milliseconds())
			return
		}
	}

	// Success.
	dur := time.Since(start).Milliseconds()
	if err := e.store.UpdateLastFired(context.Background(), rule.ID, time.Now()); err != nil {
		e.logger.Warn("rules: update last_fired failed", "rule", rule.Name, "error", err)
	}
	e.recordExec(rule.ID, rule.Name, data, ExecSuccess, nil, dur)

	// Refresh cache asynchronously (lastFiredAt changed) — don't block hot path.
	go e.refreshCache()

	e.logger.Info("rules: fired", "rule", rule.Name, "duration_ms", dur)
}

func (e *Engine) dispatchAction(action Action, data map[string]any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	switch action.Type {
	case ActionNotify:
		if e.notifier == nil {
			return fmt.Errorf("notify service not available")
		}
		msg := expandTemplate(action.Params["message"], data)
		priority := 1
		if p, ok := action.Params["priority"]; ok {
			if v, err := strconv.Atoi(p); err == nil {
				priority = v
			}
		}
		return e.notifier.Notify(ctx, msg, priority)

	case ActionTask:
		if e.tasks == nil {
			return fmt.Errorf("task service not available")
		}
		desc := expandTemplate(action.Params["description"], data)
		taskType := action.Params["type"]
		if taskType == "" {
			taskType = "general"
		}
		priority := 2
		if p, ok := action.Params["priority"]; ok {
			if v, err := strconv.Atoi(p); err == nil {
				priority = v
			}
		}
		_, err := e.tasks.Submit(ctx, desc, taskType, priority)
		return err

	default:
		return fmt.Errorf("unknown action type: %s", action.Type)
	}
}

func (e *Engine) recordExec(ruleID, ruleName string, data map[string]any, status ExecutionStatus, execErr error, durationMs int64) {
	triggerJSON, _ := json.Marshal(data)
	// Truncate trigger event to prevent large payloads in execution log.
	triggerStr := string(triggerJSON)
	if len(triggerStr) > 4096 {
		triggerStr = triggerStr[:4096] + "..."
	}

	exec := &Execution{
		RuleID:       ruleID,
		TriggerEvent: triggerStr,
		Status:       status,
		DurationMs:   durationMs,
	}
	if execErr != nil {
		exec.Error = execErr.Error()
	}
	if storeErr := e.store.RecordExecution(context.Background(), exec); storeErr != nil {
		e.logger.Warn("rules: failed to record execution", "rule", ruleID, "error", storeErr)
	}

	// Publish SSE event for all statuses so frontend stays in sync.
	if e.bus != nil {
		errStr := ""
		if execErr != nil {
			errStr = execErr.Error()
		}
		eventbus.Publish(e.bus, eventbus.RuleExecuted{
			EventMeta:  eventbus.NewMeta("rules"),
			RuleID:     ruleID,
			RuleName:   ruleName,
			Status:     string(status),
			DurationMs: durationMs,
			Error:      errStr,
		})
	}
}

// matchFilter checks that all key=value pairs in filter match the event data.
func matchFilter(filter map[string]string, data map[string]any) bool {
	for k, v := range filter {
		dv, ok := data[k]
		if !ok {
			return false
		}
		if fmt.Sprint(dv) != v {
			return false
		}
	}
	return true
}

// expandTemplate does simple {{.key}} substitution from event data.
func expandTemplate(tmpl string, data map[string]any) string {
	result := tmpl
	for k, v := range data {
		result = strings.ReplaceAll(result, "{{."+k+"}}", fmt.Sprint(v))
	}
	return result
}

// PruneExecutions removes old execution records. Call periodically.
func (e *Engine) PruneExecutions(ctx context.Context, maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge).Format(timeFormat)
	res, err := e.store.db.ExecContext(ctx, `DELETE FROM rule_executions WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
