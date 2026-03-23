package agent

import (
	"testing"
)

func TestExtractTopics_PR(t *testing.T) {
	topics := extractTopics("Spawned executor: Verify the current state of PR #219 in the cairn repo")
	if len(topics) == 0 {
		t.Fatal("expected at least one topic, got none")
	}
	found := false
	for _, topic := range topics {
		if topic == "pr:219" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected pr:219 in topics, got %v", topics)
	}
}

func TestExtractTopics_Branch(t *testing.T) {
	topics := extractTopics("worktree fix/reply-context-injection is dirty")
	found := false
	for _, topic := range topics {
		if topic == "branch:fix/reply-context-injection" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected branch:fix/reply-context-injection in topics, got %v", topics)
	}
}

func TestExtractTopics_Multiple(t *testing.T) {
	topics := extractTopics("Verify PR #219 on branch fix/reply-context and also PR #220")
	if len(topics) < 2 {
		t.Errorf("expected at least 2 topics, got %d: %v", len(topics), topics)
	}
}

func TestExtractTopics_NoMatch(t *testing.T) {
	topics := extractTopics("Waiting for something interesting to happen")
	if len(topics) != 0 {
		t.Errorf("expected no topics, got %v", topics)
	}
}

func TestDetectSuppressedTopics(t *testing.T) {
	actions := []ActivityEntry{
		{Summary: "Spawned executor: Verify PR #219 state"},
		{Summary: "Spawned observer: Check PR #219 comments"},
		{Summary: "Spawned explorer: Verify PR #219 worktree"},
		{Summary: "Spawned coder: Fix PR #220 issues"},
	}

	suppressed := detectSuppressedTopics(actions)
	if len(suppressed) == 0 {
		t.Fatal("expected at least one suppressed topic")
	}

	found219 := false
	found220 := false
	for _, s := range suppressed {
		if s == "pr:219" {
			found219 = true
		}
		if s == "pr:220" {
			found220 = true
		}
	}
	if !found219 {
		t.Errorf("expected pr:219 to be suppressed (3 mentions), got %v", suppressed)
	}
	if found220 {
		t.Errorf("pr:220 should NOT be suppressed (only 1 mention), got %v", suppressed)
	}
}

func TestDetectSuppressedTopics_BelowThreshold(t *testing.T) {
	actions := []ActivityEntry{
		{Summary: "Spawned executor: Verify PR #219"},
		{Summary: "Spawned observer: Check PR #219"},
	}

	suppressed := detectSuppressedTopics(actions)
	if len(suppressed) != 0 {
		t.Errorf("expected no suppressed topics (only 2 mentions, threshold is 3), got %v", suppressed)
	}
}

func TestInstructionMentionsTopic(t *testing.T) {
	suppressed := []string{"pr:219", "branch:fix/reply-context"}

	tests := []struct {
		instruction string
		want        string
	}{
		{"Verify the state of PR #219", "pr:219"},
		{"Check PR 219 comments", "pr:219"},
		{"Fix branch fix/reply-context issues", "branch:fix/reply-context"},
		{"Explore the codebase for test gaps", ""},
		{"Work on PR #220", ""},
	}

	for _, tt := range tests {
		got := instructionMentionsTopic(tt.instruction, suppressed)
		if got != tt.want {
			t.Errorf("instructionMentionsTopic(%q) = %q, want %q", tt.instruction, got, tt.want)
		}
	}
}
