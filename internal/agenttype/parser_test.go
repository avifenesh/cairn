package agenttype

import (
	"testing"

	"github.com/avifenesh/cairn/internal/tool"
)

func TestParseContent_Full(t *testing.T) {
	// Tool names here are bare (no "cairn." prefix) — this is valid because
	// the parser stores strings as-is; the prefix is the AGENT.md author's choice.
	content := []byte(`---
name: researcher
description: "Research agent"
mode: talk
allowed-tools: readFile,searchFiles
max-rounds: 40
model: glm-5-turbo
worktree: false
---
You are a research agent. Gather information.
`)
	at, err := ParseContent(content, "test.md")
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}
	if at.Name != "researcher" {
		t.Errorf("Name = %q, want %q", at.Name, "researcher")
	}
	if at.Description != "Research agent" {
		t.Errorf("Description = %q, want %q", at.Description, "Research agent")
	}
	if at.Mode != tool.ModeTalk {
		t.Errorf("Mode = %q, want %q", at.Mode, tool.ModeTalk)
	}
	if len(at.AllowedTools) != 2 {
		t.Errorf("AllowedTools len = %d, want 2", len(at.AllowedTools))
	}
	if at.MaxRounds != 40 {
		t.Errorf("MaxRounds = %d, want 40", at.MaxRounds)
	}
	if at.Model != "glm-5-turbo" {
		t.Errorf("Model = %q, want %q", at.Model, "glm-5-turbo")
	}
	if at.Worktree {
		t.Error("Worktree = true, want false")
	}
	if at.Content == "" {
		t.Error("Content should not be empty")
	}
}

func TestParseContent_MissingName(t *testing.T) {
	content := []byte(`---
description: "No name"
mode: work
---
Body text.
`)
	_, err := ParseContent(content, "test.md")
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestParseContent_NoFrontmatter(t *testing.T) {
	content := []byte("Just a plain file without frontmatter.")
	_, err := ParseContent(content, "test.md")
	if err == nil {
		t.Fatal("expected error for missing frontmatter")
	}
}

func TestParseContent_DefaultValues(t *testing.T) {
	content := []byte(`---
name: minimal
---
Body.
`)
	at, err := ParseContent(content, "test.md")
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}
	if at.Mode != tool.ModeWork {
		t.Errorf("Mode = %q, want %q (default)", at.Mode, tool.ModeWork)
	}
	if at.MaxRounds != 20 {
		t.Errorf("MaxRounds = %d, want 20 (default)", at.MaxRounds)
	}
	if at.Worktree {
		t.Error("Worktree should default to false")
	}
}

func TestParseContent_CodingMode(t *testing.T) {
	content := []byte(`---
name: coder
mode: coding
max-rounds: 80
worktree: true
---
Code stuff.
`)
	at, err := ParseContent(content, "test.md")
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}
	if at.Mode != tool.ModeCoding {
		t.Errorf("Mode = %q, want %q", at.Mode, tool.ModeCoding)
	}
	if !at.Worktree {
		t.Error("Worktree = false, want true")
	}
}

func TestParseContent_ExtraMetadata(t *testing.T) {
	content := []byte(`---
name: custom
custom-field: some-value
---
Body.
`)
	at, err := ParseContent(content, "test.md")
	if err != nil {
		t.Fatalf("ParseContent: %v", err)
	}
	if v, ok := at.Metadata["custom-field"]; !ok || v != "some-value" {
		t.Errorf("Metadata[custom-field] = %v, want %q", v, "some-value")
	}
}
