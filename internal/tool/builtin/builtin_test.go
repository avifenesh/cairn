package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/avifenesh/cairn/internal/tool"
)

func testCtx(t *testing.T) (*tool.ToolContext, string) {
	t.Helper()
	dir := t.TempDir()
	return &tool.ToolContext{
		SessionID: "test-session",
		WorkDir:   dir,
		Cancel:    context.Background(),
	}, dir
}

func TestReadFile(t *testing.T) {
	ctx, dir := testCtx(t)

	// Create a test file.
	content := "hello\nworld\nfoo\n"
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	args := json.RawMessage(`{"path": "test.txt"}`)
	result, err := readFile.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if result.Output != content {
		t.Fatalf("expected content %q, got %q", content, result.Output)
	}
}

func TestWriteFile(t *testing.T) {
	ctx, dir := testCtx(t)

	args := json.RawMessage(`{"path": "output.txt", "content": "written content"}`)
	result, err := writeFile.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}

	// Verify the file was written.
	data, err := os.ReadFile(filepath.Join(dir, "output.txt"))
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(data) != "written content" {
		t.Fatalf("expected %q, got %q", "written content", string(data))
	}
}

func TestEditFile(t *testing.T) {
	ctx, dir := testCtx(t)

	// Write initial content.
	path := filepath.Join(dir, "edit.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	args := json.RawMessage(`{"path": "edit.txt", "old": "world", "new": "Go"}`)
	result, err := editFile.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}

	// Verify the edit.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello Go" {
		t.Fatalf("expected %q, got %q", "hello Go", string(data))
	}
}

func TestListFiles(t *testing.T) {
	ctx, dir := testCtx(t)

	// Create some files.
	for _, name := range []string{"a.go", "b.go", "c.txt", "d.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Create a subdirectory.
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}

	// List all files.
	args := json.RawMessage(`{"path": "."}`)
	result, err := listFiles.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}

	lines := strings.Split(strings.TrimSpace(result.Output), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 entries, got %d: %v", len(lines), lines)
	}

	// List with pattern filter.
	args = json.RawMessage(`{"path": ".", "pattern": "*.go"}`)
	result, err = listFiles.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines = strings.Split(strings.TrimSpace(result.Output), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 .go files, got %d: %v", len(lines), lines)
	}
}

func TestShell(t *testing.T) {
	ctx, _ := testCtx(t)

	args := json.RawMessage(`{"command": "echo hello"}`)
	result, err := shell.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if strings.TrimSpace(result.Output) != "hello" {
		t.Fatalf("expected %q, got %q", "hello", strings.TrimSpace(result.Output))
	}
}

func TestShell_Timeout(t *testing.T) {
	ctx, _ := testCtx(t)

	args := json.RawMessage(`{"command": "sleep 60", "timeout": 1}`)
	result, err := shell.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(result.Error, "timed out") {
		t.Fatalf("expected timeout error, got: %s", result.Error)
	}
}

func TestGitRun(t *testing.T) {
	ctx, dir := testCtx(t)

	// Initialize a git repo in the temp dir.
	initArgs := json.RawMessage(`{"args": ["init"]}`)
	result, err := gitRun.Execute(ctx, initArgs)
	if err != nil {
		t.Fatalf("git init error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("git init tool error: %s", result.Error)
	}

	// Create a file so git has something to track.
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Run git status.
	statusArgs := json.RawMessage(`{"args": ["status", "--short"]}`)
	result, err = gitRun.Execute(ctx, statusArgs)
	if err != nil {
		t.Fatalf("git status error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("git status tool error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "readme.md") {
		t.Fatalf("expected git status to mention readme.md, got: %s", result.Output)
	}
}

func TestGitRun_ProtectedBranch(t *testing.T) {
	ctx, _ := testCtx(t)

	// Attempt to push to main — should be rejected.
	args := json.RawMessage(`{"args": ["push", "origin", "main"]}`)
	result, err := gitRun.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error for push to main")
	}
	if !strings.Contains(result.Error, "protected branch") {
		t.Fatalf("expected protected branch error, got: %s", result.Error)
	}

	// Also test master.
	args = json.RawMessage(`{"args": ["push", "origin", "master"]}`)
	result, err = gitRun.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error for push to master")
	}
}

func TestPathTraversal(t *testing.T) {
	ctx, _ := testCtx(t)

	// Attempt to read /etc/passwd via path traversal.
	args := json.RawMessage(`{"path": "../../../etc/passwd"}`)
	_, err := readFile.Execute(ctx, args)
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
	if !strings.Contains(err.Error(), "path traversal denied") {
		t.Fatalf("expected path traversal error, got: %v", err)
	}

	// Also try absolute path.
	args = json.RawMessage(`{"path": "/etc/passwd"}`)
	_, err = readFile.Execute(ctx, args)
	if err == nil {
		t.Fatal("expected error for absolute path outside workdir")
	}
	if !strings.Contains(err.Error(), "path traversal denied") {
		t.Fatalf("expected path traversal error, got: %v", err)
	}
}

func TestSearchFiles(t *testing.T) {
	ctx, dir := testCtx(t)

	// Create some files with searchable content.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lib.go"), []byte("package main\n\nfunc helper() string {\n\treturn \"world\"\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	args := json.RawMessage(`{"pattern": "func.*\\(", "path": "."}`)
	result, err := searchFiles.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "func main()") {
		t.Fatalf("expected search to find 'func main()', got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "func helper()") {
		t.Fatalf("expected search to find 'func helper()', got: %s", result.Output)
	}
}

func TestAll(t *testing.T) {
	tools := All()
	if len(tools) != 27 {
		t.Fatalf("expected 27 built-in tools, got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, tl := range tools {
		names[tl.Name()] = true
	}

	expected := []string{
		"cairn.readFile", "cairn.writeFile", "cairn.editFile", "cairn.deleteFile",
		"cairn.listFiles", "cairn.searchFiles", "cairn.shell", "cairn.gitRun",
		"cairn.createMemory", "cairn.searchMemory", "cairn.manageMemory",
		"cairn.readFeed", "cairn.markRead", "cairn.archiveFeedItem", "cairn.deleteFeedItem", "cairn.digest",
		"cairn.journalSearch",
		"cairn.webSearch", "cairn.webFetch",
		"cairn.createTask", "cairn.listTasks", "cairn.completeTask",
		"cairn.compose", "cairn.getStatus",
		"cairn.loadSkill", "cairn.listSkills",
		"cairn.notify",
	}
	for _, name := range expected {
		if !names[name] {
			t.Fatalf("expected tool %q to be registered", name)
		}
	}
}
