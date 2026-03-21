package agent

import (
	"strings"
	"testing"
	"time"
)

func TestObservations_IsEmpty(t *testing.T) {
	empty := &Observations{}
	if !empty.isEmpty() {
		t.Error("expected empty observations")
	}

	withFeed := &Observations{UnreadFeedCount: 5}
	if withFeed.isEmpty() {
		t.Error("expected non-empty with unread feed")
	}

	withErrors := &Observations{RecentErrors: []string{"test error"}}
	if withErrors.isEmpty() {
		t.Error("expected non-empty with errors")
	}
}

func TestBuildBriefingPrompt_RichContent(t *testing.T) {
	obs := &Observations{
		UnreadFeedCount: 3,
		UnreadBySource:  map[string]int{"github": 2, "gmail": 1},
		TopUnread: []FeedItem{
			{Source: "github", Kind: "pr", Title: "fix: update deps", Actor: "avi"},
		},
		RelevantMemories: []string{"User prefers concise responses"},
		RecentErrors:     []string{"build failed"},
		RecentSessions: []JournalSummary{
			{Summary: "Merged PR #96", Mode: "coding", CreatedAt: time.Now()},
		},
		UpcomingCrons:   []string{"morning-digest at 09:00"},
		TicksSinceStart: 42,
		CurrentTime:     "2026-03-20 10:00 UTC",
	}

	prompt := buildBriefingPrompt(obs)

	checks := map[string]string{
		"fix: update deps":     "feed item title",
		"github: 2":            "source breakdown",
		"User prefers concise": "memories",
		"Merged PR #96":        "recent sessions",
		"build failed":         "errors",
		"morning-digest":       "upcoming crons",
	}

	for sub, label := range checks {
		if !strings.Contains(prompt, sub) {
			t.Errorf("expected %s (%q) in briefing prompt", label, sub)
		}
	}
}
