package agent

import (
	"testing"
)

func TestAllowedToolsFromSkills_NoSkills(t *testing.T) {
	s := &Session{}
	if got := s.AllowedToolsFromSkills(); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestAllowedToolsFromSkills_NoRestriction(t *testing.T) {
	s := &Session{
		ActiveSkills: []ActiveSkill{
			{Name: "web-search", AllowedTools: nil},
		},
	}
	if got := s.AllowedToolsFromSkills(); got != nil {
		t.Fatalf("expected nil (no restriction), got %v", got)
	}
}

func TestAllowedToolsFromSkills_WithRestriction(t *testing.T) {
	s := &Session{
		ActiveSkills: []ActiveSkill{
			{Name: "web-search", AllowedTools: []string{"cairn.webSearch", "cairn.webFetch"}},
		},
	}
	allowed := s.AllowedToolsFromSkills()
	if allowed == nil {
		t.Fatal("expected non-nil allowed tools")
	}

	// Should include the 2 skill tools + cairn.loadSkill + cairn.listSkills.
	allowedMap := make(map[string]bool)
	for _, t := range allowed {
		allowedMap[t] = true
	}
	for _, expected := range []string{"cairn.webSearch", "cairn.webFetch", "cairn.loadSkill", "cairn.listSkills"} {
		if !allowedMap[expected] {
			t.Errorf("expected %q in allowed tools", expected)
		}
	}
}

func TestAllowedToolsFromSkills_MergeMultiple(t *testing.T) {
	s := &Session{
		ActiveSkills: []ActiveSkill{
			{Name: "web-search", AllowedTools: []string{"cairn.webSearch"}},
			{Name: "digest", AllowedTools: []string{"cairn.digest", "cairn.readFeed"}},
		},
	}
	allowed := s.AllowedToolsFromSkills()
	if allowed == nil {
		t.Fatal("expected non-nil")
	}

	allowedMap := make(map[string]bool)
	for _, t := range allowed {
		allowedMap[t] = true
	}
	// Should have union: webSearch + digest + readFeed + loadSkill + listSkills.
	for _, expected := range []string{"cairn.webSearch", "cairn.digest", "cairn.readFeed", "cairn.loadSkill", "cairn.listSkills"} {
		if !allowedMap[expected] {
			t.Errorf("expected %q in merged allowed tools", expected)
		}
	}
}

func TestAllowedToolsFromSkills_MixedRestrictions(t *testing.T) {
	// One skill has restrictions, one doesn't — unrestricted wins.
	s := &Session{
		ActiveSkills: []ActiveSkill{
			{Name: "web-search", AllowedTools: []string{"cairn.webSearch"}},
			{Name: "general", AllowedTools: nil}, // no restriction
		},
	}
	if got := s.AllowedToolsFromSkills(); got != nil {
		t.Fatalf("expected nil when any skill has no restriction, got %v", got)
	}
}
