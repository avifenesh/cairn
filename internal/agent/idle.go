package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/llm"
	"github.com/avifenesh/cairn/internal/signal"
	"github.com/avifenesh/cairn/internal/task"
)

const (
	minIdleInterval   = 5 * time.Minute
	briefingMaxAge    = 30 * time.Minute // rebuild briefing every 30min
	briefingMaxTokens = 1024             // cheap model output for briefing
	decisionMaxTokens = 512              // decision model output
)

// FeedItem is a summary of an unread feed event for idle reasoning.
type FeedItem struct {
	Source string
	Kind   string
	Title  string
	Actor  string
}

// JournalSummary is a recent session summary for idle context.
type JournalSummary struct {
	Summary   string
	Mode      string
	CreatedAt time.Time
}

// Observations aggregates signals from the agent's world for idle reasoning.
type Observations struct {
	// Feed
	UnreadFeedCount int            `json:"unreadFeedCount"`
	UnreadBySource  map[string]int `json:"unreadBySource,omitempty"`
	TopUnread       []FeedItem     `json:"-"` // top N unread items with titles

	// Journal
	RecentSessions []JournalSummary `json:"-"` // last few session summaries
	RecentErrors   []string         `json:"recentErrors,omitempty"`

	// Memories
	RelevantMemories []string `json:"-"` // user preferences + active project context

	// System
	PendingTasks    int      `json:"pendingTasks"`
	DigestQueueLen  int      `json:"digestQueueLen"`
	TicksSinceStart int64    `json:"ticksSinceStart"`
	CurrentTime     string   `json:"-"` // human-readable local time
	UpcomingCrons   []string `json:"-"` // cron jobs firing within 2h
}

func (o *Observations) isEmpty() bool {
	return o.UnreadFeedCount == 0 && o.PendingTasks == 0 &&
		len(o.RecentErrors) == 0 && o.DigestQueueLen == 0 &&
		len(o.RecentSessions) == 0 && len(o.RelevantMemories) == 0 &&
		len(o.UpcomingCrons) == 0
}

// IdleDecision represents what the agent decided to do during an idle tick.
type IdleDecision struct {
	Action   string `json:"action"`   // "notify", "task", "learn", "wait"
	Reason   string `json:"reason"`   // Why this action was chosen
	Message  string `json:"message"`  // For notify: notification text
	Priority int    `json:"priority"` // For notify: 0=low, 1=medium, 2=high, 3=critical
}

// idleTick runs when no pending task was claimed and idle mode is enabled.
// Two-phase approach:
//  1. Cheap model (briefingModel) rebuilds context briefing every 30min
//  2. Primary model reads SOUL + briefing + live signals → JSON decision
func (l *Loop) idleTick(ctx context.Context) {
	if !l.config.IdleEnabled || l.provider == nil {
		return
	}
	if time.Since(l.lastIdleTick) < minIdleInterval {
		return
	}

	obs := l.gatherObservations(ctx)
	if obs.isEmpty() {
		l.logger.Debug("idle: no observations, skipping")
		return
	}

	// Add memories only when we have something to reason about (avoids wasted RAG search).
	l.gatherMemories(ctx, obs)

	// Phase 1: Rebuild briefing if stale (cheap model).
	if l.idleBriefing == "" || time.Since(l.briefingBuiltAt) > briefingMaxAge {
		l.rebuildBriefing(ctx, obs)
	}

	// Only update throttle after we have observations worth reasoning about.
	l.lastIdleTick = time.Now()

	// Phase 2: Decision (primary model reads SOUL + briefing + live counts).
	decision := l.reasonAboutAction(ctx, obs)
	l.executeIdleDecision(ctx, decision)
}

const memorySearchQuery = "user preferences goals active projects current work"

// gatherObservations collects rich signals from feed, journal, and crons.
// Memories are gathered separately in gatherMemories (only when observations are non-empty).
func (l *Loop) gatherObservations(ctx context.Context) *Observations {
	now := time.Now()
	obs := &Observations{
		TicksSinceStart: l.tickCount.Load(),
		CurrentTime:     now.Format("2006-01-02 15:04 MST"),
	}

	// Feed: unread items with titles (not just counts).
	if l.events != nil {
		events, err := l.events.List(ctx, signal.EventFilter{
			UnreadOnly:      true,
			ExcludeArchived: true,
			Limit:           50,
		})
		if err == nil {
			obs.UnreadFeedCount = len(events)
			obs.UnreadBySource = make(map[string]int)
			for _, e := range events {
				obs.UnreadBySource[e.Source]++
			}
			limit := min(10, len(events))
			for _, e := range events[:limit] {
				obs.TopUnread = append(obs.TopUnread, FeedItem{
					Source: e.Source,
					Kind:   e.Kind,
					Title:  e.Title,
					Actor:  e.Actor,
				})
			}
		}
	}

	// Journal: recent session summaries + errors (last 6 hours).
	if l.journaler != nil && l.journaler.store != nil {
		entries, err := l.journaler.store.Recent(ctx, 6*time.Hour)
		if err == nil {
			for _, e := range entries {
				obs.RecentErrors = append(obs.RecentErrors, e.Errors...)
				if e.Summary != "" {
					obs.RecentSessions = append(obs.RecentSessions, JournalSummary{
						Summary:   e.Summary,
						Mode:      e.Mode,
						CreatedAt: e.CreatedAt,
					})
				}
			}
			if len(obs.RecentErrors) > 5 {
				obs.RecentErrors = obs.RecentErrors[:5]
			}
			if len(obs.RecentSessions) > 5 {
				obs.RecentSessions = obs.RecentSessions[:5]
			}
		}
	}

	// Crons: upcoming jobs within 2 hours (exclude already-past next_run).
	if l.cronStore != nil {
		jobs, err := l.cronStore.List(ctx)
		if err == nil {
			horizon := now.Add(2 * time.Hour)
			for _, j := range jobs {
				if j.Enabled && j.NextRunAt != nil && j.NextRunAt.After(now) && j.NextRunAt.Before(horizon) {
					obs.UpcomingCrons = append(obs.UpcomingCrons,
						fmt.Sprintf("%s (%s) at %s UTC", j.Name, j.Schedule, j.NextRunAt.Format("15:04")))
				}
			}
		}
	}

	return obs
}

// gatherMemories adds user preference memories to observations.
// Called only when observations are non-empty (avoids wasted RAG searches).
func (l *Loop) gatherMemories(ctx context.Context, obs *Observations) {
	if l.memories == nil {
		return
	}
	results, err := l.memories.Search(ctx, memorySearchQuery, 5)
	if err == nil {
		for _, r := range results {
			obs.RelevantMemories = append(obs.RelevantMemories, r.Memory.Content)
		}
	}
}

// rebuildBriefing calls a cheap model to compress raw observations into
// a focused situation briefing. Cached for briefingMaxAge.
func (l *Loop) rebuildBriefing(ctx context.Context, obs *Observations) {
	model := l.config.BriefingModel
	if model == "" {
		model = l.config.Model // fallback to primary if no cheap model configured
	}

	prompt := buildBriefingPrompt(obs)

	req := &llm.Request{
		Model: model,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock{Text: prompt}}},
		},
		MaxTokens:       briefingMaxTokens,
		DisableThinking: true,
	}

	ch, err := l.provider.Stream(ctx, req)
	if err != nil {
		l.logger.Warn("idle: briefing rebuild failed", "error", err)
		return
	}

	var result strings.Builder
	for ev := range ch {
		if td, ok := ev.(llm.TextDelta); ok {
			result.WriteString(td.Text)
		}
	}

	if result.Len() > 0 {
		l.idleBriefing = result.String()
		l.briefingBuiltAt = time.Now()
		l.logger.Info("idle: briefing rebuilt", "model", model, "chars", result.Len())
	}
}

// reasonAboutAction asks the LLM what to do given SOUL + briefing + live signals.
func (l *Loop) reasonAboutAction(ctx context.Context, obs *Observations) *IdleDecision {
	soulContent := ""
	if l.soul != nil {
		soulContent = l.soul.Content()
	}

	prompt := buildDecisionPrompt(soulContent, l.idleBriefing, obs)

	req := &llm.Request{
		Model: l.config.Model,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock{Text: prompt}}},
		},
		MaxTokens:       decisionMaxTokens,
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
		original := d.Action
		d.Action = "wait"
		d.Reason = "unknown action: " + original
	}

	return &d
}

// executeIdleDecision acts on the agent's idle decision.
func (l *Loop) executeIdleDecision(ctx context.Context, d *IdleDecision) {
	// Store for activity recording (tick() picks this up).
	l.lastIdleDecision = d

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

// buildBriefingPrompt creates a prompt for the cheap model to summarize raw context.
// This runs every ~30min and produces a focused situation briefing.
func buildBriefingPrompt(obs *Observations) string {
	var b strings.Builder

	b.WriteString("Summarize the following observations into a concise situation briefing (max 500 words).\n")
	b.WriteString("Focus on: what needs attention, what changed recently, what the user cares about.\n")
	b.WriteString("Skip noise and routine items. Be specific — include names, numbers, titles.\n\n")

	fmt.Fprintf(&b, "Current time: %s\n\n", obs.CurrentTime)

	if len(obs.RelevantMemories) > 0 {
		b.WriteString("Known user context:\n")
		for _, m := range obs.RelevantMemories {
			fmt.Fprintf(&b, "- %s\n", m)
		}
		b.WriteString("\n")
	}

	if len(obs.RecentSessions) > 0 {
		b.WriteString("Recent agent activity (last 6h):\n")
		for _, s := range obs.RecentSessions {
			fmt.Fprintf(&b, "- [%s] %s (%s)\n", s.Mode, s.Summary, s.CreatedAt.Format("15:04"))
		}
		b.WriteString("\n")
	}

	if obs.UnreadFeedCount > 0 {
		sources := sortedSourceCounts(obs.UnreadBySource)
		fmt.Fprintf(&b, "Unread feed: %d items (%s)\n", obs.UnreadFeedCount, strings.Join(sources, ", "))
		for _, item := range obs.TopUnread {
			actor := ""
			if item.Actor != "" {
				actor = " by " + item.Actor
			}
			fmt.Fprintf(&b, "- [%s/%s] %s%s\n", item.Source, item.Kind, item.Title, actor)
		}
		b.WriteString("\n")
	}

	if len(obs.RecentErrors) > 0 {
		b.WriteString("Recent errors:\n")
		for _, e := range obs.RecentErrors {
			fmt.Fprintf(&b, "- %s\n", e)
		}
		b.WriteString("\n")
	}

	if len(obs.UpcomingCrons) > 0 {
		b.WriteString("Upcoming scheduled tasks:\n")
		for _, c := range obs.UpcomingCrons {
			fmt.Fprintf(&b, "- %s\n", c)
		}
		b.WriteString("\n")
	}

	if obs.PendingTasks > 0 {
		fmt.Fprintf(&b, "Pending tasks: %d\n", obs.PendingTasks)
	}

	return b.String()
}

// buildDecisionPrompt creates a compact prompt for the decision model.
// It uses SOUL + the pre-built briefing + live signal counts.
func buildDecisionPrompt(soulContent, briefing string, obs *Observations) string {
	var b strings.Builder

	if soulContent != "" {
		b.WriteString(soulContent)
		b.WriteString("\n\n---\n\n")
	}

	fmt.Fprintf(&b, "Current time: %s\n\n", obs.CurrentTime)

	if briefing != "" {
		b.WriteString("## Situation Briefing\n")
		b.WriteString(briefing)
		b.WriteString("\n\n")
	}

	// Live signal snapshot (cheap — just counts for freshness).
	b.WriteString("## Live Signals\n")
	fmt.Fprintf(&b, "- Unread feed: %d", obs.UnreadFeedCount)
	if len(obs.UnreadBySource) > 0 {
		fmt.Fprintf(&b, " (%s)", strings.Join(sortedSourceCounts(obs.UnreadBySource), ", "))
	}
	b.WriteString("\n")
	if obs.PendingTasks > 0 {
		fmt.Fprintf(&b, "- Pending tasks: %d\n", obs.PendingTasks)
	}
	if obs.DigestQueueLen > 0 {
		fmt.Fprintf(&b, "- Digest queue: %d\n", obs.DigestQueueLen)
	}
	if len(obs.RecentErrors) > 0 {
		fmt.Fprintf(&b, "- Errors (last 6h): %d\n", len(obs.RecentErrors))
	}

	b.WriteString("\n---\n\n")
	b.WriteString("Based on your personality (SOUL) and the briefing, what should you do?\n\n")
	b.WriteString("Actions: notify | task | learn | wait\n")
	b.WriteString("Rules: \"wait\" is valid and often correct. Only notify for genuine value. Be specific.\n\n")
	b.WriteString("JSON only:\n")
	b.WriteString(`{"action": "wait|notify|task|learn", "reason": "specific explanation", "message": "if notify", "priority": 0}`)

	return b.String()
}

// sortedSourceCounts returns sorted "source: count" strings.
func sortedSourceCounts(m map[string]int) []string {
	sources := make([]string, 0, len(m))
	for src := range m {
		sources = append(sources, src)
	}
	sort.Strings(sources)
	parts := make([]string, 0, len(sources))
	for _, src := range sources {
		parts = append(parts, fmt.Sprintf("%s: %d", src, m[src]))
	}
	return parts
}

// AgentNotification is published to the event bus when the idle loop decides to notify.
type AgentNotification struct {
	eventbus.EventMeta
	Message  string `json:"message"`
	Priority int    `json:"priority"`
	Reason   string `json:"reason"`
}
