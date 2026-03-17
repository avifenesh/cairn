package skill

import (
	"strings"
	"testing"
)

func TestInject_AlwaysSkills(t *testing.T) {
	skills := []*Skill{
		{
			Name:      "core",
			Inclusion: Always,
			Content:   "Core instructions here.",
		},
		{
			Name:      "safety",
			Inclusion: Always,
			Content:   "Safety rules here.",
		},
	}

	result := InjectSkills(skills, "talk", 0)

	if !strings.Contains(result, "## Active Skills") {
		t.Error("missing '## Active Skills' header")
	}
	if !strings.Contains(result, "### core") {
		t.Error("missing '### core' section")
	}
	if !strings.Contains(result, "Core instructions here.") {
		t.Error("missing core skill body")
	}
	if !strings.Contains(result, "### safety") {
		t.Error("missing '### safety' section")
	}
	if !strings.Contains(result, "Safety rules here.") {
		t.Error("missing safety skill body")
	}
	// On-demand section should not appear.
	if strings.Contains(result, "Available Skills") {
		t.Error("should not contain 'Available Skills' section when there are no on-demand skills")
	}
}

func TestInject_OnDemandSkills(t *testing.T) {
	skills := []*Skill{
		{
			Name:        "deploy",
			Description: "Use when deploying",
			Inclusion:   OnDemand,
			Content:     "Deploy instructions that should NOT appear.",
		},
		{
			Name:        "test",
			Description: "Use when running tests",
			Inclusion:   OnDemand,
			Content:     "Test instructions that should NOT appear.",
		},
	}

	result := InjectSkills(skills, "talk", 0)

	if !strings.Contains(result, "Available Skills (ask to activate)") {
		t.Error("missing on-demand header")
	}
	if !strings.Contains(result, "- deploy: Use when deploying") {
		t.Error("missing deploy description line")
	}
	if !strings.Contains(result, "- test: Use when running tests") {
		t.Error("missing test description line")
	}
	// Body should NOT be included for on-demand skills.
	if strings.Contains(result, "Deploy instructions that should NOT appear") {
		t.Error("on-demand skill body should not be included")
	}
	if strings.Contains(result, "Test instructions that should NOT appear") {
		t.Error("on-demand skill body should not be included")
	}
}

func TestInject_MixedSkills(t *testing.T) {
	skills := []*Skill{
		{
			Name:      "always-skill",
			Inclusion: Always,
			Content:   "Always body.",
		},
		{
			Name:        "optional-skill",
			Description: "Optional desc",
			Inclusion:   OnDemand,
			Content:     "Should not appear.",
		},
	}

	result := InjectSkills(skills, "work", 0)

	if !strings.Contains(result, "### always-skill") {
		t.Error("missing always skill section")
	}
	if !strings.Contains(result, "Always body.") {
		t.Error("missing always skill body")
	}
	if !strings.Contains(result, "- optional-skill: Optional desc") {
		t.Error("missing on-demand skill listing")
	}
	if strings.Contains(result, "Should not appear.") {
		t.Error("on-demand skill body should not be included")
	}
}

func TestInject_TokenBudget(t *testing.T) {
	// Create a skill with a large body.
	longBody := strings.Repeat("x", 10000)
	skills := []*Skill{
		{
			Name:      "big-skill",
			Inclusion: Always,
			Content:   longBody,
		},
	}

	// Budget of 100 tokens = 400 chars.
	result := InjectSkills(skills, "talk", 100)

	if len(result) > 400 {
		t.Errorf("result length %d exceeds budget of 400 chars", len(result))
	}
	if len(result) == 0 {
		t.Error("result should not be empty even with small budget")
	}
}

func TestInject_EmptySkills(t *testing.T) {
	result := InjectSkills(nil, "talk", 0)
	if result != "" {
		t.Errorf("expected empty string for nil skills, got %q", result)
	}

	result = InjectSkills([]*Skill{}, "talk", 0)
	if result != "" {
		t.Errorf("expected empty string for empty skills, got %q", result)
	}
}

func TestInject_DeterministicOrder(t *testing.T) {
	skills := []*Skill{
		{Name: "zebra", Inclusion: Always, Content: "Z body."},
		{Name: "alpha", Inclusion: Always, Content: "A body."},
		{Name: "mid", Inclusion: Always, Content: "M body."},
	}

	result1 := InjectSkills(skills, "talk", 0)
	result2 := InjectSkills(skills, "talk", 0)

	if result1 != result2 {
		t.Error("output should be deterministic across calls")
	}

	// Verify alpha comes before mid comes before zebra.
	aIdx := strings.Index(result1, "### alpha")
	mIdx := strings.Index(result1, "### mid")
	zIdx := strings.Index(result1, "### zebra")

	if aIdx >= mIdx || mIdx >= zIdx {
		t.Errorf("skills not in alphabetical order: alpha@%d, mid@%d, zebra@%d", aIdx, mIdx, zIdx)
	}
}
