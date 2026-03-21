package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/signal"
)

const (
	minIdleInterval   = 5 * time.Minute
	briefingMaxAge    = 30 * time.Minute // rebuild briefing every 30min
	briefingMaxTokens = 4096             // cheap model briefing output
	decisionMaxTokens = 4096             // decision — thinking + JSON (unlimited sub)
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
	Action   string `json:"action"`   // "notify", "task", "code", "learn", "wait"
	Reason   string `json:"reason"`   // Why this action was chosen
	Message  string `json:"message"`  // For notify: notification text
	Priority int    `json:"priority"` // For notify: 0=low, 1=medium, 2=high, 3=critical
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
