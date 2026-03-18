package skill

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeSkill(t *testing.T, dir, name, content string) string {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestService_Discover(t *testing.T) {
	dir := t.TempDir()

	writeSkill(t, dir, "deploy", `---
name: deploy
description: Deploy the application
inclusion: always
---

Run deploy.sh
`)
	writeSkill(t, dir, "test-runner", `---
name: test-runner
description: Run tests
inclusion: on-demand
---

Run npm test
`)

	svc := NewService([]string{dir}, slog.Default())
	if err := svc.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	skills := svc.List()
	if len(skills) != 2 {
		t.Fatalf("List: got %d skills, want 2", len(skills))
	}
}

func TestService_Get(t *testing.T) {
	dir := t.TempDir()

	writeSkill(t, dir, "helper", `---
name: helper
description: Help with things
---

Help body.
`)

	svc := NewService([]string{dir}, slog.Default())
	if err := svc.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	sk := svc.Get("helper")
	if sk == nil {
		t.Fatal("Get(helper): got nil")
	}
	if sk.Name != "helper" {
		t.Errorf("Name: got %q, want %q", sk.Name, "helper")
	}

	// Non-existent skill.
	if got := svc.Get("nonexistent"); got != nil {
		t.Errorf("Get(nonexistent): got %v, want nil", got)
	}
}

func TestService_ForPrompt(t *testing.T) {
	dir := t.TempDir()

	writeSkill(t, dir, "core-a", `---
name: core-a
description: Core skill A
inclusion: always
---

Always included A.
`)
	writeSkill(t, dir, "core-b", `---
name: core-b
description: Core skill B
inclusion: always
---

Always included B.
`)
	writeSkill(t, dir, "optional", `---
name: optional
description: Optional skill
inclusion: on-demand
---

On demand body.
`)

	svc := NewService([]string{dir}, slog.Default())
	if err := svc.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	always := svc.ForPrompt(Always)
	if len(always) != 2 {
		t.Errorf("ForPrompt(Always): got %d, want 2", len(always))
	}

	onDemand := svc.ForPrompt(OnDemand)
	if len(onDemand) != 1 {
		t.Errorf("ForPrompt(OnDemand): got %d, want 1", len(onDemand))
	}
	if len(onDemand) > 0 && onDemand[0].Name != "optional" {
		t.Errorf("ForPrompt(OnDemand)[0].Name: got %q, want %q", onDemand[0].Name, "optional")
	}
}

func TestService_Watch(t *testing.T) {
	dir := t.TempDir()

	writeSkill(t, dir, "initial", `---
name: initial
description: Initial skill
---

Body.
`)

	svc := NewService([]string{dir}, slog.Default())
	if err := svc.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(svc.List()) != 1 {
		t.Fatalf("initial List: got %d, want 1", len(svc.List()))
	}

	// Register onChange callback.
	changed := make(chan struct{}, 1)
	svc.OnChange(func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})

	// Start watching.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.Watch(ctx)

	// Add a new skill file.
	path := writeSkill(t, dir, "added", `---
name: added
description: Added later
---

New body.
`)

	// Ensure mod time differs from anything seen.
	future := time.Now().Add(10 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	// Trigger a check manually for faster testing.
	svc.checkReload()

	select {
	case <-changed:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for onChange callback")
	}

	if len(svc.List()) != 2 {
		t.Errorf("after add, List: got %d, want 2", len(svc.List()))
	}

	if sk := svc.Get("added"); sk == nil {
		t.Error("Get(added): got nil after watch reload")
	}
}

func TestService_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	svc := NewService([]string{dir}, slog.Default())
	if err := svc.Discover(); err != nil {
		t.Fatalf("Discover on empty dir: %v", err)
	}

	if len(svc.List()) != 0 {
		t.Errorf("List: got %d, want 0", len(svc.List()))
	}
}

func TestService_NonexistentDir(t *testing.T) {
	svc := NewService([]string{"/nonexistent/skills/dir"}, slog.Default())
	if err := svc.Discover(); err != nil {
		t.Fatalf("Discover on nonexistent dir should not error: %v", err)
	}

	if len(svc.List()) != 0 {
		t.Errorf("List: got %d, want 0", len(svc.List()))
	}
}

func TestService_MultipleDirs(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	writeSkill(t, dir1, "skill-a", `---
name: skill-a
description: From dir1
---

A body.
`)
	writeSkill(t, dir2, "skill-b", `---
name: skill-b
description: From dir2
---

B body.
`)

	svc := NewService([]string{dir1, dir2}, slog.Default())
	if err := svc.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(svc.List()) != 2 {
		t.Fatalf("List: got %d, want 2", len(svc.List()))
	}
	if svc.Get("skill-a") == nil {
		t.Error("Get(skill-a): nil")
	}
	if svc.Get("skill-b") == nil {
		t.Error("Get(skill-b): nil")
	}
}

func TestService_MultiDirOverride(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	// Same skill name in both dirs — dir2 should override dir1 (last wins).
	writeSkill(t, dir1, "shared", `---
name: shared
description: From first directory
---

First body.
`)
	writeSkill(t, dir2, "shared", `---
name: shared
description: From second directory (override)
---

Second body.
`)

	// Also add a unique skill in dir1 that should survive.
	writeSkill(t, dir1, "unique", `---
name: unique
description: Only in first directory
---

Unique body.
`)

	svc := NewService([]string{dir1, dir2}, slog.Default())
	if err := svc.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	// Should have 2 skills: "shared" (from dir2) and "unique" (from dir1).
	if len(svc.List()) != 2 {
		t.Fatalf("List: got %d, want 2", len(svc.List()))
	}

	// The "shared" skill should come from dir2 (later dir overrides).
	sk := svc.Get("shared")
	if sk == nil {
		t.Fatal("Get(shared): nil")
	}
	if sk.Description != "From second directory (override)" {
		t.Errorf("shared.Description: got %q, want description from dir2", sk.Description)
	}
	if !searchString(sk.Content, "Second body") {
		t.Errorf("shared.Content should be from dir2, got %q", sk.Content)
	}

	// The "unique" skill should still exist.
	if svc.Get("unique") == nil {
		t.Error("Get(unique): nil — unique skill from dir1 should survive")
	}
}

func TestService_SkipsBadSkills(t *testing.T) {
	dir := t.TempDir()

	// Good skill.
	writeSkill(t, dir, "good", `---
name: good
description: A good skill
---

Good body.
`)

	// Bad skill (no frontmatter).
	badDir := filepath.Join(dir, "bad")
	if err := os.MkdirAll(badDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "SKILL.md"), []byte("no frontmatter here"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	svc := NewService([]string{dir}, slog.Default())
	if err := svc.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	// Only the good skill should be registered.
	if len(svc.List()) != 1 {
		t.Errorf("List: got %d, want 1 (bad skill should be skipped)", len(svc.List()))
	}
	if svc.Get("good") == nil {
		t.Error("Get(good): nil")
	}
}
