package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse_BasicSkill(t *testing.T) {
	content := `---
name: my-skill
description: "Use when user asks to deploy"
inclusion: always
allowed-tools: "cairn.shell,cairn.readFile"
disable-model-invocation: true
---

# Deploy Instructions

Run the deploy script.
`
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sk, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if sk.Name != "my-skill" {
		t.Errorf("Name: got %q, want %q", sk.Name, "my-skill")
	}
	if sk.Description != "Use when user asks to deploy" {
		t.Errorf("Description: got %q, want %q", sk.Description, "Use when user asks to deploy")
	}
	if sk.Inclusion != Always {
		t.Errorf("Inclusion: got %q, want %q", sk.Inclusion, Always)
	}
	if len(sk.AllowedTools) != 2 {
		t.Fatalf("AllowedTools length: got %d, want 2", len(sk.AllowedTools))
	}
	if sk.AllowedTools[0] != "cairn.shell" {
		t.Errorf("AllowedTools[0]: got %q, want %q", sk.AllowedTools[0], "cairn.shell")
	}
	if sk.AllowedTools[1] != "cairn.readFile" {
		t.Errorf("AllowedTools[1]: got %q, want %q", sk.AllowedTools[1], "cairn.readFile")
	}
	if !sk.DisableModel {
		t.Error("DisableModel: got false, want true")
	}
	if sk.Location != path {
		t.Errorf("Location: got %q, want %q", sk.Location, path)
	}
	if sk.Content == "" {
		t.Error("Content: got empty, want body text")
	}
	if !contains(sk.Content, "Deploy Instructions") {
		t.Errorf("Content should contain 'Deploy Instructions', got %q", sk.Content)
	}
}

func TestParse_MinimalSkill(t *testing.T) {
	content := `---
name: helper
description: A simple helper skill
---

Do helpful things.
`
	sk, err := ParseContent([]byte(content), "test.md")
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}

	if sk.Name != "helper" {
		t.Errorf("Name: got %q, want %q", sk.Name, "helper")
	}
	if sk.Description != "A simple helper skill" {
		t.Errorf("Description: got %q, want %q", sk.Description, "A simple helper skill")
	}
	// Defaults.
	if sk.Inclusion != OnDemand {
		t.Errorf("Inclusion: got %q, want %q", sk.Inclusion, OnDemand)
	}
	if len(sk.AllowedTools) != 0 {
		t.Errorf("AllowedTools: got %v, want empty", sk.AllowedTools)
	}
	if sk.DisableModel {
		t.Error("DisableModel: got true, want false")
	}
}

func TestParse_NoFrontmatter(t *testing.T) {
	content := `# No frontmatter here

Just a regular markdown file.
`
	_, err := ParseContent([]byte(content), "bad.md")
	if err == nil {
		t.Fatal("expected error for file without frontmatter")
	}
}

func TestParse_QuotedValues(t *testing.T) {
	content := `---
name: quoted-skill
description: "Use when user asks to build something"
---

Build instructions.
`
	sk, err := ParseContent([]byte(content), "test.md")
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}

	if sk.Description != "Use when user asks to build something" {
		t.Errorf("Description: got %q, want unquoted value", sk.Description)
	}
}

func TestParse_SingleQuotedValues(t *testing.T) {
	content := `---
name: single-quoted
description: 'Use when user asks for help'
---

Help text.
`
	sk, err := ParseContent([]byte(content), "test.md")
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}

	if sk.Description != "Use when user asks for help" {
		t.Errorf("Description: got %q, want unquoted value", sk.Description)
	}
}

func TestParse_AllowedTools(t *testing.T) {
	content := `---
name: tooled
description: Has tools
allowed-tools: "cairn.shell, cairn.readFile, cairn.writeFile"
---

Body.
`
	sk, err := ParseContent([]byte(content), "test.md")
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}

	if len(sk.AllowedTools) != 3 {
		t.Fatalf("AllowedTools length: got %d, want 3", len(sk.AllowedTools))
	}

	expected := []string{"cairn.shell", "cairn.readFile", "cairn.writeFile"}
	for i, want := range expected {
		if sk.AllowedTools[i] != want {
			t.Errorf("AllowedTools[%d]: got %q, want %q", i, sk.AllowedTools[i], want)
		}
	}
}

func TestParse_BooleanField(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"true", true},
		{"false", false},
		{"yes", true},
		{"no", false},
		{"1", true},
		{"0", false},
		{"TRUE", true},
		{"FALSE", false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			content := "---\nname: bool-test\ndescription: test\ndisable-model-invocation: " + tt.value + "\n---\n\nBody.\n"
			sk, err := ParseContent([]byte(content), "test.md")
			if err != nil {
				t.Fatalf("ParseContent: %v", err)
			}
			if sk.DisableModel != tt.want {
				t.Errorf("DisableModel for %q: got %v, want %v", tt.value, sk.DisableModel, tt.want)
			}
		})
	}
}

func TestParse_ExtraMetadata(t *testing.T) {
	content := `---
name: meta-skill
description: Has extra metadata
version: 2.1
author: avi
experimental: true
---

Body.
`
	sk, err := ParseContent([]byte(content), "test.md")
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}

	if sk.Metadata["version"] != "2.1" {
		t.Errorf("Metadata[version]: got %v, want %q", sk.Metadata["version"], "2.1")
	}
	if sk.Metadata["author"] != "avi" {
		t.Errorf("Metadata[author]: got %v, want %q", sk.Metadata["author"], "avi")
	}
	if sk.Metadata["experimental"] != true {
		t.Errorf("Metadata[experimental]: got %v, want true", sk.Metadata["experimental"])
	}
}

func TestParse_MissingName(t *testing.T) {
	content := `---
description: No name field
---

Body.
`
	_, err := ParseContent([]byte(content), "test.md")
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestParse_MissingDescription(t *testing.T) {
	content := `---
name: no-desc
---

Body.
`
	_, err := ParseContent([]byte(content), "test.md")
	if err == nil {
		t.Fatal("expected error for missing description")
	}
}

// contains checks if s contains substr.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
