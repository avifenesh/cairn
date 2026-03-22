package agenttype

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func writeAgentMD(t *testing.T, dir, name, content string) {
	t.Helper()
	d := filepath.Join(dir, name)
	os.MkdirAll(d, 0755)
	os.WriteFile(filepath.Join(d, "AGENT.md"), []byte(content), 0644)
}

func TestService_Discover(t *testing.T) {
	dir := t.TempDir()
	writeAgentMD(t, dir, "researcher", `---
name: researcher
mode: talk
max-rounds: 15
---
Research stuff.
`)
	writeAgentMD(t, dir, "coder", `---
name: coder
mode: coding
max-rounds: 80
worktree: true
---
Code stuff.
`)

	svc := NewService([]string{dir}, slog.Default())
	if err := svc.Discover(); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	types := svc.List()
	if len(types) != 2 {
		t.Fatalf("List() len = %d, want 2", len(types))
	}
}

func TestService_Get(t *testing.T) {
	dir := t.TempDir()
	writeAgentMD(t, dir, "executor", `---
name: executor
mode: work
max-rounds: 10
---
Execute.
`)

	svc := NewService([]string{dir}, slog.Default())
	svc.Discover()

	at := svc.Get("executor")
	if at == nil {
		t.Fatal("Get(executor) = nil, want non-nil")
	}
	if at.MaxRounds != 10 {
		t.Errorf("MaxRounds = %d, want 10", at.MaxRounds)
	}

	if svc.Get("nonexistent") != nil {
		t.Error("Get(nonexistent) should return nil")
	}
}

func TestService_MissingDir(t *testing.T) {
	svc := NewService([]string{"/tmp/cairn-nonexistent-dir-test"}, slog.Default())
	if err := svc.Discover(); err != nil {
		t.Fatalf("Discover on missing dir should not error: %v", err)
	}
	if len(svc.List()) != 0 {
		t.Error("expected empty list for missing dir")
	}
}

func TestService_CreateDelete(t *testing.T) {
	dir := t.TempDir()
	svc := NewService([]string{dir}, slog.Default())
	svc.Discover()

	content := `---
name: test-type
mode: talk
max-rounds: 5
---
Test body.
`
	if err := svc.Create("test-type", content); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if svc.Get("test-type") == nil {
		t.Error("Get after Create should return non-nil")
	}

	if err := svc.Delete("test-type"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if svc.Get("test-type") != nil {
		t.Error("Get after Delete should return nil")
	}
}

func TestService_InstallDir(t *testing.T) {
	svc := NewService([]string{"/first", "/second"}, slog.Default())
	if got := svc.InstallDir(); got != "/second" {
		t.Errorf("InstallDir() = %q, want %q", got, "/second")
	}
}
