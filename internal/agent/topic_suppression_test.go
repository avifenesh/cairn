package agent

import (
	"testing"
)

func TestExtractTopics_PR(t *testing.T) {
	topics := extractTopics("Spawned executor: Verify the current state of PR #219 in the cairn repo")
	assertContainsTopic(t, topics, "pr:219")
}

func TestExtractTopics_PullRequest(t *testing.T) {
	topics := extractTopics("Review pull request 42 again")
	assertContainsTopic(t, topics, "pr:42")
}

func TestExtractTopics_Issue(t *testing.T) {
	topics := extractTopics("Checked issue #42 for blockers")
	assertContainsTopic(t, topics, "issue:42")
}

func TestExtractTopics_Branch(t *testing.T) {
	topics := extractTopics("worktree fix/reply-context-injection is dirty")
	assertContainsTopic(t, topics, "branch:fix/reply-context-injection")
}

func TestExtractTopics_BranchCaseInsensitive(t *testing.T) {
	// Branch pattern now has (?i) — should match regardless of case.
	topics := extractTopics("Branch Fix/Reply-Context is stale")
	assertContainsTopic(t, topics, "branch:fix/reply-context")
}

func TestExtractTopics_Task(t *testing.T) {
	topics := extractTopics("retry task 3cf8a86c4f493a21 immediately")
	assertContainsTopic(t, topics, "task:3cf8a86c4f493a21")
}

func TestExtractTopics_Multiple(t *testing.T) {
	topics := extractTopics("Verify PR #219 on branch fix/reply-context and also PR #220")
	assertContainsTopic(t, topics, "pr:219")
	assertContainsTopic(t, topics, "pr:220")
	assertContainsTopic(t, topics, "branch:fix/reply-context")
}

func TestExtractTopics_NoMatch(t *testing.T) {
	topics := extractTopics("Waiting for something interesting to happen")
	if len(topics) != 0 {
		t.Errorf("expected no topics, got %v", topics)
	}
}

func TestExtractTopics_EntityNormalized(t *testing.T) {
	// Entities should be lowercased for consistent matching.
	topics := extractTopics("Branch Fix/UPPER-case is active")
	assertContainsTopic(t, topics, "branch:fix/upper-case")
}

func TestDetectSuppressedTopics(t *testing.T) {
	actions := []ActivityEntry{
		{Summary: "Spawned executor: Verify PR #219 state"},
		{Summary: "Spawned observer: Check PR #219 comments"},
		{Summary: "Spawned explorer: Verify PR #219 worktree"},
		{Summary: "Spawned coder: Fix PR #220 issues"},
	}

	suppressed := detectSuppressedTopics(actions)
	assertContainsTopic(t, suppressed, "pr:219")

	for _, s := range suppressed {
		if s == "pr:220" {
			t.Errorf("pr:220 should NOT be suppressed (only 1 mention), got %v", suppressed)
		}
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

func TestDetectSuppressedTopics_EmptyActions(t *testing.T) {
	suppressed := detectSuppressedTopics(nil)
	if len(suppressed) != 0 {
		t.Errorf("expected no suppressed topics for nil actions, got %v", suppressed)
	}

	suppressed = detectSuppressedTopics([]ActivityEntry{})
	if len(suppressed) != 0 {
		t.Errorf("expected no suppressed topics for empty actions, got %v", suppressed)
	}
}

func TestDetectSuppressedTopics_DetailsField(t *testing.T) {
	// Topic reference in Details (not Summary) should still count.
	actions := []ActivityEntry{
		{Summary: "Idle: task", Details: "Working on PR #300"},
		{Summary: "Idle: task", Details: "Still on PR #300"},
		{Summary: "Idle: task", Details: "Retrying PR #300"},
	}

	suppressed := detectSuppressedTopics(actions)
	assertContainsTopic(t, suppressed, "pr:300")
}

func TestDetectSuppressedTopics_NoCrossFieldStitching(t *testing.T) {
	// Summary ends with "PR" and Details starts with "#219" — should NOT stitch.
	actions := []ActivityEntry{
		{Summary: "Checked PR", Details: "#219 is open"},
		{Summary: "Checked PR", Details: "#219 is open"},
		{Summary: "Checked PR", Details: "#219 is open"},
	}

	suppressed := detectSuppressedTopics(actions)
	// "PR" alone won't match — it needs "PR #NNN" in the same string.
	// "#219" alone won't match the PR pattern either (needs "PR" prefix).
	// So no pr:219 should be detected.
	for _, s := range suppressed {
		if s == "pr:219" {
			t.Errorf("cross-field stitching should not create pr:219, got %v", suppressed)
		}
	}
}

func TestInstructionMentionsTopic(t *testing.T) {
	suppressed := []string{"pr:219", "branch:fix/reply-context", "issue:42", "task:3cf8a86c"}

	tests := []struct {
		instruction string
		want        string
	}{
		// PR matches
		{"Verify the state of PR #219", "pr:219"},
		{"Check PR 219 comments", "pr:219"},
		{"Review pull request 219 again", "pr:219"},
		{"look at issue #219", "pr:219"}, // #219 matches pr:219 (ambiguous — # prefix)
		// PR boundary — must not match #2190
		{"Work on PR #2190", ""},
		{"Check #219.", "pr:219"}, // period is a boundary
		// Branch matches
		{"Fix branch fix/reply-context issues", "branch:fix/reply-context"},
		{"work on fix/reply-context", "branch:fix/reply-context"},
		// Issue matches
		{"look at issue #42", "issue:42"},
		{"fix issue 42 now", "issue:42"},
		{"issue #420 is different", ""}, // boundary: 420 != 42
		// Task matches
		{"retry task 3cf8a86c immediately", "task:3cf8a86c"},
		// No match
		{"Explore the codebase for test gaps", ""},
		{"Work on PR #220", ""},
		// Empty suppressed list
	}

	for _, tt := range tests {
		got := instructionMentionsTopic(tt.instruction, suppressed)
		if got != tt.want {
			t.Errorf("instructionMentionsTopic(%q) = %q, want %q", tt.instruction, got, tt.want)
		}
	}
}

func TestInstructionMentionsTopic_EmptySuppressed(t *testing.T) {
	got := instructionMentionsTopic("anything here", nil)
	if got != "" {
		t.Errorf("expected empty for nil suppressed, got %q", got)
	}
	got = instructionMentionsTopic("anything here", []string{})
	if got != "" {
		t.Errorf("expected empty for empty suppressed, got %q", got)
	}
}

func TestContainsWithBoundary(t *testing.T) {
	tests := []struct {
		haystack string
		needle   string
		want     bool
	}{
		{"check #219 now", "#219", true},
		{"check #2190 now", "#219", false}, // digit boundary
		{"#219", "#219", true},             // start+end of string
		{"(#219)", "#219", true},           // parens are boundaries
		{"x219y", "219", false},            // alpha boundaries
		{"fix/reply-context is stale", "fix/reply-context", true},
		{"prefix-fix/reply-context-suffix", "fix/reply-context", true}, // hyphen is a boundary (not alnum)
	}

	for _, tt := range tests {
		got := containsWithBoundary(tt.haystack, tt.needle)
		if got != tt.want {
			t.Errorf("containsWithBoundary(%q, %q) = %v, want %v", tt.haystack, tt.needle, got, tt.want)
		}
	}
}

func assertContainsTopic(t *testing.T, topics []string, want string) {
	t.Helper()
	if len(topics) == 0 {
		t.Fatalf("expected topic %q but got empty list", want)
	}
	for _, topic := range topics {
		if topic == want {
			return
		}
	}
	t.Errorf("expected %q in topics, got %v", want, topics)
}
