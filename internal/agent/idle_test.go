package agent

import (
	"testing"
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

func TestBuildIdlePrompt_WithSoul(t *testing.T) {
	obs := &Observations{
		UnreadFeedCount: 3,
		UnreadBySource:  map[string]int{"github": 2, "gmail": 1},
		RecentErrors:    []string{"build failed"},
		TicksSinceStart: 42,
	}

	prompt := buildIdlePrompt("I am Cairn. Be concise.", obs)

	if !containsString(prompt, "I am Cairn") {
		t.Error("expected SOUL content in prompt")
	}
	if !containsString(prompt, "Unread feed items: 3") {
		t.Error("expected unread count in prompt")
	}
	if !containsString(prompt, "github: 2") {
		t.Error("expected source breakdown in prompt")
	}
	if !containsString(prompt, "build failed") {
		t.Error("expected errors in prompt")
	}
	if !containsString(prompt, "wait") {
		t.Error("expected wait option in prompt")
	}
}

func TestBuildIdlePrompt_NoSoul(t *testing.T) {
	obs := &Observations{UnreadFeedCount: 1, TicksSinceStart: 1}
	prompt := buildIdlePrompt("", obs)

	if containsString(prompt, "personality and values") {
		t.Error("should not include SOUL header when content is empty")
	}
	if !containsString(prompt, "Unread feed items: 1") {
		t.Error("expected observations in prompt")
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
