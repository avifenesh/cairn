package skill

import (
	"path/filepath"
	"strings"
	"testing"
)

// knownTools is a shared list of tool names for validation tests.
var testKnownTools = []string{
	"pub.readFile", "pub.writeFile", "pub.editFile", "pub.deleteFile",
	"pub.listFiles", "pub.searchFiles", "pub.shell", "pub.gitRun",
	"cairn.loadSkill", "cairn.listSkills",
}

func TestValidate_Clean(t *testing.T) {
	sk := &Skill{
		Name:         "deploy",
		Description:  "Use when user asks to deploy the application",
		AllowedTools: []string{"pub.readFile", "pub.shell"},
		DisableModel: true,
		Location:     "/skills/deploy/SKILL.md",
	}

	issues := Validate(sk, testKnownTools)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues for clean skill, got %d:", len(issues))
		for _, iss := range issues {
			t.Errorf("  [%s] %s", iss.Severity, iss.Message)
		}
	}
}

func TestValidate_UnknownTool(t *testing.T) {
	sk := &Skill{
		Name:         "test-skill",
		Description:  "Use when running tests on the project",
		AllowedTools: []string{"pub.readFile", "pub.nonExistent"},
		DisableModel: true,
		Location:     "/skills/test-skill/SKILL.md",
	}

	issues := Validate(sk, testKnownTools)

	found := false
	for _, iss := range issues {
		if iss.Severity == SeverityWarning && strings.Contains(iss.Message, "pub.nonExistent") {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about unknown tool pub.nonExistent")
	}
}

func TestValidate_ShellWithoutDisableModel(t *testing.T) {
	sk := &Skill{
		Name:         "risky",
		Description:  "Use when user needs shell access for debugging",
		AllowedTools: []string{"pub.shell"},
		DisableModel: false, // should trigger warning
		Location:     "/skills/risky/SKILL.md",
	}

	issues := Validate(sk, testKnownTools)

	found := false
	for _, iss := range issues {
		if iss.Severity == SeverityWarning && strings.Contains(iss.Message, "pub.shell") && strings.Contains(iss.Message, "disable-model-invocation") && strings.Contains(iss.Message, "security risk") {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about pub.shell without disable-model-invocation")
	}
}

func TestValidate_ShortDescription(t *testing.T) {
	sk := &Skill{
		Name:        "tiny",
		Description: "Short",
		Location:    "/skills/tiny/SKILL.md",
	}

	issues := Validate(sk, testKnownTools)

	found := false
	for _, iss := range issues {
		if iss.Severity == SeverityWarning && strings.Contains(iss.Message, "too short") {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about short description")
	}
}

func TestValidate_NameMismatch(t *testing.T) {
	// Skill name is "my-skill" but it lives in /skills/wrong-name/SKILL.md.
	sk := &Skill{
		Name:        "my-skill",
		Description: "Use when user asks for help with something",
		Location:    filepath.Join("/skills", "wrong-name", "SKILL.md"),
	}

	issues := Validate(sk, testKnownTools)

	found := false
	for _, iss := range issues {
		if iss.Severity == SeverityWarning && strings.Contains(iss.Message, "does not match directory") {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about name/directory mismatch")
	}
}

func TestValidate_NoIssuesWhenNameMatchesDir(t *testing.T) {
	sk := &Skill{
		Name:        "helper",
		Description: "Use when user asks for help with coding tasks",
		Location:    filepath.Join("/skills", "helper", "SKILL.md"),
	}

	issues := Validate(sk, testKnownTools)

	for _, iss := range issues {
		if strings.Contains(iss.Message, "does not match directory") {
			t.Error("should not warn about name mismatch when name equals directory basename")
		}
	}
}

func TestValidate_ShellWithDisableModel(t *testing.T) {
	// pub.shell with disable-model-invocation=true should NOT warn.
	sk := &Skill{
		Name:         "safe-shell",
		Description:  "Use when user asks to run safe shell commands",
		AllowedTools: []string{"pub.shell"},
		DisableModel: true,
		Location:     "/skills/safe-shell/SKILL.md",
	}

	issues := Validate(sk, testKnownTools)

	for _, iss := range issues {
		if strings.Contains(iss.Message, "pub.shell") && strings.Contains(iss.Message, "disable-model-invocation") && strings.Contains(iss.Message, "security risk") {
			t.Error("should not warn about pub.shell when disable-model-invocation is true")
		}
	}
}

func TestValidate_EmptyKnownTools(t *testing.T) {
	sk := &Skill{
		Name:         "any-tool",
		Description:  "Use when user asks for anything involving tools",
		AllowedTools: []string{"pub.readFile", "pub.writeFile"},
		DisableModel: true,
		Location:     "/skills/any-tool/SKILL.md",
	}

	// With empty knownTools, all allowed-tools should warn.
	issues := Validate(sk, nil)

	warnCount := 0
	for _, iss := range issues {
		if strings.Contains(iss.Message, "unknown tool") {
			warnCount++
		}
	}
	if warnCount != 2 {
		t.Errorf("expected 2 unknown tool warnings with empty knownTools, got %d", warnCount)
	}
}
