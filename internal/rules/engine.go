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

// Engine subscribes to bus events, evaluates rules, and dispatches actions.
type Engine struct {
	store    *Store
	bus      *eventbus.Bus
	notifier NotifyService
	tasks    TaskSubmitter
	logger   *slog.Logger
	unsubs   []func()
	cache    []*Rule
	cacheMu  sync.RWMutex
}

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
				"title":      ev.Title,
				"url":        ev.URL,
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

// RefreshCache reloads enabled rules from the store. Call after CRUD operations.
func (e *Engine) RefreshCache() {
	e.refreshCache()
}

func (e *Engine) refreshCache() {
	rules, err := e.store.ListEnabled(context.Background())
	if err != nil {
		e.logger.Warn("rules: failed to refresh cache", "error", err)
		return
	}
	e.cacheMu.Lock()
	e.cache = rules
	e.cacheMu.Unlock()
}

// Stats returns summary statistics for observations.
func (e *Engine) Stats() (total, enabled, recentFailures int) {
	e.cacheMu.RLock()
	enabled = len(e.cache)
	e.cacheMu.RUnlock()

	all, err := e.store.List(context.Background())
	if err == nil {
		total = len(all)
	}

	recent, err := e.store.ListRecentExecutions(context.Background(), 50)
	if err == nil {
		for _, ex := range recent {
			if ex.Status == "error" {
				recentFailures++
			}
		}
	}
	return
}

// handleEvent processes a bus event against all cached rules.
func (e *Engine) handleEvent(eventType string, data map[string]any) {
	e.cacheMu.RLock()
	rules := e.cache
	e.cacheMu.RUnlock()

	for _, rule := range rules {
		if !rule.Enabled || rule.Trigger.Type != TriggerEvent {
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
				go e.recordExec(rule.ID, data, "throttled", nil, 0)
				continue
			}
		}
		go e.evaluateAndExecute(rule, data)
	}
}

func (e *Engine) evaluateAndExecute(rule *Rule, data map[string]any) {
	start := time.Now()

	// Evaluate condition.
	if rule.Condition != "" {
		result, err := expr.Eval(rule.Condition, data)
		if err != nil {
			e.logger.Warn("rules: condition eval error", "rule", rule.Name, "error", err)
			e.recordExec(rule.ID, data, "error", err, time.Since(start).Milliseconds())
			return
		}
		b, ok := result.(bool)
		if !ok || !b {
			e.recordExec(rule.ID, data, "condition_false", nil, time.Since(start).Milliseconds())
			return
		}
	}

	// Execute actions sequentially.
	for _, action := range rule.Actions {
		if err := e.dispatchAction(action, data); err != nil {
			e.logger.Warn("rules: action failed", "rule", rule.Name, "action", action.Type, "error", err)
			e.recordExec(rule.ID, data, "error", err, time.Since(start).Milliseconds())
			return
		}
	}

	// Success.
	dur := time.Since(start).Milliseconds()
	e.store.UpdateLastFired(context.Background(), rule.ID, time.Now())
	e.recordExec(rule.ID, data, "success", nil, dur)

	// Publish SSE event.
	if e.bus != nil {
		eventbus.Publish(e.bus, eventbus.RuleExecuted{
			EventMeta: eventbus.NewMeta("rules"),
			RuleID:    rule.ID,
			RuleName:  rule.Name,
			Status:    "success",
		})
	}

	// Refresh cache (lastFiredAt changed).
	e.refreshCache()

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

func (e *Engine) recordExec(ruleID string, data map[string]any, status string, err error, durationMs int64) {
	triggerJSON, _ := json.Marshal(data)
	exec := &Execution{
		RuleID:       ruleID,
		TriggerEvent: string(triggerJSON),
		Status:       status,
		DurationMs:   durationMs,
	}
	if err != nil {
		exec.Error = err.Error()
	}
	if storeErr := e.store.RecordExecution(context.Background(), exec); storeErr != nil {
		e.logger.Warn("rules: failed to record execution", "rule", ruleID, "error", storeErr)
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
