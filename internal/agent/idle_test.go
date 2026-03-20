package agent

import (
	"testing"
	"time"
)

func TestParseIdleDecision_Valid(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		action string
	}{
		{"wait", `{"action":"wait","reason":"nothing to do"}`, "wait"},
		{"notify", `{"action":"notify","reason":"unread emails","message":"3 unread emails","priority":2}`, "notify"},
		{"task", `{"action":"task","reason":"consolidate memories"}`, "task"},
		{"learn", `{"action":"learn","reason":"pattern detected"}`, "learn"},
		{"with fences", "```json\n{\"action\":\"wait\",\"reason\":\"quiet\"}\n```", "wait"},
		{"with prefix text", "I think I should wait.\n{\"action\":\"wait\",\"reason\":\"nothing urgent\"}", "wait"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := parseIdleDecision(tt.input)
			if d.Action != tt.action {
				t.Errorf("expected action %q, got %q", tt.action, d.Action)
			}
		})
	}
}

func TestParseIdleDecision_Invalid(t *testing.T) {
	tests := []string{
		"",
		"not json at all",
		"{}",                      // missing action → empty string → defaults to wait
		`{"action":"invalid123"}`, // unknown action → defaults to wait
	}

	for _, input := range tests {
		d := parseIdleDecision(input)
		if d.Action != "wait" {
			t.Errorf("expected 'wait' for invalid input %q, got %q", input, d.Action)
		}
	}
}

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

	if !containsString(prompt, "fix: update deps") {
		t.Error("expected feed item title in briefing prompt")
	}
	if !containsString(prompt, "github: 2") {
		t.Error("expected source breakdown in briefing prompt")
	}
	if !containsString(prompt, "User prefers concise") {
		t.Error("expected memories in briefing prompt")
	}
	if !containsString(prompt, "Merged PR #96") {
		t.Error("expected recent sessions in briefing prompt")
	}
	if !containsString(prompt, "build failed") {
		t.Error("expected errors in briefing prompt")
	}
	if !containsString(prompt, "morning-digest") {
		t.Error("expected upcoming crons in briefing prompt")
	}
}

func TestBuildDecisionPrompt_WithBriefing(t *testing.T) {
	obs := &Observations{
		UnreadFeedCount: 3,
		UnreadBySource:  map[string]int{"github": 3},
		CurrentTime:     "2026-03-20 10:00 UTC",
	}

	prompt := buildDecisionPrompt("I am Cairn.", "3 GitHub items: PR merged, star added, CI passed. Nothing urgent.", obs, nil)

	if !containsString(prompt, "I am Cairn") {
		t.Error("expected SOUL in decision prompt")
	}
	if !containsString(prompt, "Situation Briefing") {
		t.Error("expected briefing section in decision prompt")
	}
	if !containsString(prompt, "Nothing urgent") {
		t.Error("expected briefing content in decision prompt")
	}
	if !containsString(prompt, "wait") {
		t.Error("expected wait option in decision prompt")
	}
}

func TestBuildDecisionPrompt_NoBriefing(t *testing.T) {
	obs := &Observations{
		UnreadFeedCount: 1,
		UnreadBySource:  map[string]int{"github": 1},
		CurrentTime:     "2026-03-20 10:00 UTC",
	}

	prompt := buildDecisionPrompt("", "", obs, nil)

	if containsString(prompt, "Situation Briefing") {
		t.Error("should not include briefing section when empty")
	}
	if !containsString(prompt, "Unread feed: 1") {
		t.Error("expected live signal counts")
	}
}

func containsString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
