package agent

import (
	"testing"
)

func TestParseOrchestratorDecision_Valid(t *testing.T) {
	raw := `{"actions": [{"type": "approve_memory", "memoryId": "mem_abc"}], "reason": "approved fact"}`
	d := parseOrchestratorDecision(raw)
	if len(d.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(d.Actions))
	}
	if d.Actions[0].Type != "approve_memory" {
		t.Errorf("expected approve_memory, got %s", d.Actions[0].Type)
	}
	if d.Actions[0].MemoryID != "mem_abc" {
		t.Errorf("expected mem_abc, got %s", d.Actions[0].MemoryID)
	}
	if d.Reason != "approved fact" {
		t.Errorf("expected reason 'approved fact', got %q", d.Reason)
	}
}

func TestParseOrchestratorDecision_MultipleActions(t *testing.T) {
	raw := `{"actions": [
		{"type": "approve_memory", "memoryId": "m1"},
		{"type": "spawn", "spawnType": "reviewer", "instruction": "check PR"},
		{"type": "notify", "message": "PR ready", "priority": 1}
	], "reason": "batch"}`
	d := parseOrchestratorDecision(raw)
	if len(d.Actions) != 3 {
		t.Fatalf("expected 3 actions, got %d", len(d.Actions))
	}
	if d.Actions[1].SpawnType != "reviewer" {
		t.Errorf("expected reviewer, got %s", d.Actions[1].SpawnType)
	}
	if d.Actions[2].Priority != 1 {
		t.Errorf("expected priority 1, got %d", d.Actions[2].Priority)
	}
}

func TestParseOrchestratorDecision_MarkdownFences(t *testing.T) {
	raw := "```json\n{\"actions\": [{\"type\": \"wait\"}], \"reason\": \"nothing\"}\n```"
	d := parseOrchestratorDecision(raw)
	if len(d.Actions) != 1 || d.Actions[0].Type != "wait" {
		t.Fatalf("expected wait action, got %v", d.Actions)
	}
}

func TestParseOrchestratorDecision_Invalid(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"not json", "I think we should wait"},
		{"invalid json", "{broken"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := parseOrchestratorDecision(tc.raw)
			if d == nil {
				t.Fatal("expected non-nil decision")
			}
			if len(d.Actions) == 0 || d.Actions[0].Type != "wait" {
				t.Errorf("expected fallback wait action, got %v", d.Actions)
			}
		})
	}
}

func TestParseOrchestratorDecision_UnknownTypeFiltered(t *testing.T) {
	raw := `{"actions": [{"type": "hack_mainframe"}, {"type": "wait"}], "reason": "test"}`
	d := parseOrchestratorDecision(raw)
	if len(d.Actions) != 1 {
		t.Fatalf("expected 1 valid action (unknown filtered), got %d", len(d.Actions))
	}
	if d.Actions[0].Type != "wait" {
		t.Errorf("expected wait, got %s", d.Actions[0].Type)
	}
}

func TestParseOrchestratorDecision_EmptyActions(t *testing.T) {
	raw := `{"actions": [], "reason": "nothing to do"}`
	d := parseOrchestratorDecision(raw)
	if len(d.Actions) != 1 || d.Actions[0].Type != "wait" {
		t.Fatalf("expected fallback wait, got %v", d.Actions)
	}
}

func TestOrchestratorDecisionToIdle_Task(t *testing.T) {
	d := &OrchestratorDecision{
		Actions: []OrchestratorAction{{Type: "spawn", SpawnType: "coder", Instruction: "fix bug"}},
		Reason:  "bug found",
	}
	idle := orchestratorDecisionToIdle(d)
	if idle.Action != "task" {
		t.Errorf("expected 'task', got %q", idle.Action)
	}
	if idle.Reason != "bug found" {
		t.Errorf("expected reason 'bug found', got %q", idle.Reason)
	}
}

func TestOrchestratorDecisionToIdle_Notify(t *testing.T) {
	d := &OrchestratorDecision{
		Actions: []OrchestratorAction{{Type: "notify", Message: "PR ready", Priority: 2}},
		Reason:  "PR complete",
	}
	idle := orchestratorDecisionToIdle(d)
	if idle.Action != "notify" {
		t.Errorf("expected 'notify', got %q", idle.Action)
	}
	if idle.Message != "PR ready" {
		t.Errorf("expected message 'PR ready', got %q", idle.Message)
	}
}

func TestOrchestratorDecisionToIdle_Nil(t *testing.T) {
	idle := orchestratorDecisionToIdle(nil)
	if idle.Action != "wait" {
		t.Errorf("expected 'wait', got %q", idle.Action)
	}
}

func TestOrchestratorState_HasActionableItems(t *testing.T) {
	// Empty state.
	s := &OrchestratorState{}
	if s.hasActionableItems() {
		t.Error("empty state should not have actionable items")
	}

	// With proposed memories.
	s.ProposedMemories = []proposedMemoryInfo{{ID: "m1"}}
	if !s.hasActionableItems() {
		t.Error("state with proposed memories should have actionable items")
	}

	// With pending approvals.
	s2 := &OrchestratorState{}
	s2.PendingApprovals = []approvalInfo{{ID: "a1"}}
	if !s2.hasActionableItems() {
		t.Error("state with pending approvals should have actionable items")
	}
}

func TestTruncateStr(t *testing.T) {
	if truncateStr("hello", 10) != "hello" {
		t.Error("short string should not be truncated")
	}
	if truncateStr("hello world this is long", 10) != "hello w..." {
		t.Errorf("long string truncation: got %q", truncateStr("hello world this is long", 10))
	}
}
