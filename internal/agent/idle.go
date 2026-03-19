package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/llm"
	"github.com/avifenesh/cairn/internal/signal"
	"github.com/avifenesh/cairn/internal/task"
)

const minIdleInterval = 5 * time.Minute

// Observations aggregates signals from the agent's world for idle reasoning.
type Observations struct {
	UnreadFeedCount int            `json:"unreadFeedCount"`
	UnreadBySource  map[string]int `json:"unreadBySource,omitempty"`
	PendingTasks    int            `json:"pendingTasks"`
	RecentErrors    []string       `json:"recentErrors,omitempty"`
	DigestQueueLen  int            `json:"digestQueueLen"`
	TicksSinceStart int64          `json:"ticksSinceStart"`
}

func (o *Observations) isEmpty() bool {
	return o.UnreadFeedCount == 0 && o.PendingTasks == 0 &&
		len(o.RecentErrors) == 0 && o.DigestQueueLen == 0
}

// IdleDecision represents what the agent decided to do during an idle tick.
type IdleDecision struct {
	Action   string `json:"action"`   // "notify", "task", "learn", "wait"
	Reason   string `json:"reason"`   // Why this action was chosen
	Message  string `json:"message"`  // For notify: notification text
	Priority int    `json:"priority"` // For notify: 0=low, 1=medium, 2=high, 3=critical
}

// idleTick runs when no pending task was claimed and idle mode is enabled.
// It gathers observations, asks the LLM what to do, and executes the decision.
func (l *Loop) idleTick(ctx context.Context) {
	if !l.config.IdleEnabled || l.provider == nil {
		return
	}
	if time.Since(l.lastIdleTick) < minIdleInterval {
		return
	}
	l.lastIdleTick = time.Now()

	obs := l.gatherObservations(ctx)
	if obs.isEmpty() {
		l.logger.Debug("idle: no observations, skipping")
		return
	}

	decision := l.reasonAboutAction(ctx, obs)
	l.executeIdleDecision(ctx, decision)
}

// gatherObservations collects signals from feed, tasks, and journal.
func (l *Loop) gatherObservations(ctx context.Context) *Observations {
	obs := &Observations{
		TicksSinceStart: l.tickCount.Load(),
	}

	// Feed: unread count by source.
	if l.events != nil {
		events, err := l.events.List(ctx, signal.EventFilter{
			UnreadOnly: true,
			Limit:      100,
		})
		if err == nil {
			obs.UnreadFeedCount = len(events)
			obs.UnreadBySource = make(map[string]int)
			for _, e := range events {
				obs.UnreadBySource[e.Source]++
			}
		}
	}

	// Journal: recent errors from last 2 hours.
	if l.journaler != nil && l.journaler.store != nil {
		entries, err := l.journaler.store.Recent(ctx, 2*time.Hour)
		if err == nil {
			for _, e := range entries {
				obs.RecentErrors = append(obs.RecentErrors, e.Errors...)
			}
			// Cap errors to avoid bloating the prompt.
			if len(obs.RecentErrors) > 5 {
				obs.RecentErrors = obs.RecentErrors[:5]
			}
		}
	}

	return obs
}

// reasonAboutAction asks the LLM what to do given SOUL + observations.
func (l *Loop) reasonAboutAction(ctx context.Context, obs *Observations) *IdleDecision {
	soulContent := ""
	if l.soul != nil {
		soulContent = l.soul.Content()
	}

	prompt := buildIdlePrompt(soulContent, obs)

	req := &llm.Request{
		Model: l.config.Model,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock{Text: prompt}}},
		},
		MaxTokens:       256,
		DisableThinking: true,
	}

	ch, err := l.provider.Stream(ctx, req)
	if err != nil {
		l.logger.Warn("idle: LLM call failed", "error", err)
		return &IdleDecision{Action: "wait", Reason: "LLM error"}
	}

	var result strings.Builder
	for ev := range ch {
		switch e := ev.(type) {
		case llm.TextDelta:
			result.WriteString(e.Text)
		case llm.StreamError:
			l.logger.Warn("idle: LLM stream error", "error", e.Err)
			return &IdleDecision{Action: "wait", Reason: "LLM stream error"}
		}
	}

	decision := parseIdleDecision(result.String())
	l.logger.Info("idle: decision",
		"action", decision.Action,
		"reason", decision.Reason,
		"unread", obs.UnreadFeedCount,
		"errors", len(obs.RecentErrors),
	)
	return decision
}

// parseIdleDecision extracts a JSON decision from the LLM response.
func parseIdleDecision(raw string) *IdleDecision {
	// Strip markdown fences if present.
	cleaned := strings.TrimSpace(raw)
	if idx := strings.Index(cleaned, "{"); idx >= 0 {
		if end := strings.LastIndex(cleaned, "}"); end > idx {
			cleaned = cleaned[idx : end+1]
		}
	}

	var d IdleDecision
	if err := json.Unmarshal([]byte(cleaned), &d); err != nil {
		return &IdleDecision{Action: "wait", Reason: "failed to parse decision: " + err.Error()}
	}

	// Validate action.
	switch d.Action {
	case "notify", "task", "learn", "wait":
		// valid
	default:
		d.Action = "wait"
		d.Reason = "unknown action: " + d.Action
	}

	return &d
}

// executeIdleDecision acts on the agent's idle decision.
func (l *Loop) executeIdleDecision(ctx context.Context, d *IdleDecision) {
	switch d.Action {
	case "notify":
		if d.Message != "" && l.bus != nil {
			// Publish notification event — channel handler or SSE will pick it up.
			eventbus.Publish(l.bus, AgentNotification{
				EventMeta: eventbus.NewMeta("agent"),
				Message:   d.Message,
				Priority:  d.Priority,
				Reason:    d.Reason,
			})
			l.logger.Info("idle: notification sent", "message", d.Message[:min(len(d.Message), 80)])
		}

	case "task":
		if l.tasks != nil {
			input, _ := json.Marshal(map[string]string{"instruction": d.Reason})
			_, err := l.tasks.Submit(ctx, &task.SubmitRequest{
				Type:        "idle",
				Priority:    task.PriorityLow,
				Description: d.Reason,
				Input:       input,
			})
			if err != nil {
				l.logger.Warn("idle: task submission failed", "error", err)
			}
		}

	case "learn":
		if l.reflector != nil {
			l.runReflection(ctx)
			l.lastReflect = time.Now()
			l.logger.Info("idle: triggered early reflection", "reason", d.Reason)
		}

	case "wait":
		// Nothing to do — valid choice.
	}
}

// buildIdlePrompt constructs the prompt for idle reasoning.
func buildIdlePrompt(soulContent string, obs *Observations) string {
	var b strings.Builder

	if soulContent != "" {
		b.WriteString("Here is your personality and values:\n\n")
		b.WriteString(soulContent)
		b.WriteString("\n\n---\n\n")
	}

	b.WriteString("Current observations:\n")
	fmt.Fprintf(&b, "- Unread feed items: %d", obs.UnreadFeedCount)
	if len(obs.UnreadBySource) > 0 {
		parts := make([]string, 0, len(obs.UnreadBySource))
		for src, count := range obs.UnreadBySource {
			parts = append(parts, fmt.Sprintf("%s: %d", src, count))
		}
		fmt.Fprintf(&b, " (%s)", strings.Join(parts, ", "))
	}
	b.WriteString("\n")

	if obs.PendingTasks > 0 {
		fmt.Fprintf(&b, "- Pending tasks: %d\n", obs.PendingTasks)
	}

	if len(obs.RecentErrors) > 0 {
		fmt.Fprintf(&b, "- Recent errors (last 2h): %s\n", strings.Join(obs.RecentErrors, "; "))
	}

	if obs.DigestQueueLen > 0 {
		fmt.Fprintf(&b, "- Digest queue: %d queued notifications\n", obs.DigestQueueLen)
	}

	fmt.Fprintf(&b, "- Ticks since start: %d\n", obs.TicksSinceStart)

	b.WriteString("\nBased on your personality and these observations, what should you do right now?\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("- \"wait\" is always valid and often correct. Don't act without clear value.\n")
	b.WriteString("- Only notify for things genuinely worth the user's attention right now.\n")
	b.WriteString("- Never perform external actions without approval.\n")
	b.WriteString("- Be specific about what and why.\n\n")
	b.WriteString("Respond with JSON only:\n")
	b.WriteString(`{"action": "wait|notify|task|learn", "reason": "brief explanation", "message": "notification text if action=notify", "priority": 0}`)

	return b.String()
}

// AgentNotification is published to the event bus when the idle loop decides to notify.
type AgentNotification struct {
	eventbus.EventMeta
	Message  string `json:"message"`
	Priority int    `json:"priority"`
	Reason   string `json:"reason"`
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
