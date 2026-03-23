package builtin

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
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

func confinedCtx(t *testing.T) (*tool.ToolContext, string) {
	t.Helper()
	dir := t.TempDir()
	return &tool.ToolContext{
		SessionID: "test-confined",
		WorkDir:   dir,
		Confined:  true,
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

func TestShell_GrepNoMatch(t *testing.T) {
	// grep returning exit 1 (no match) should NOT cause the shell to abort.
	// With set -e this would kill the shell; without it, the command
	// completes and subsequent commands (like the echo) still run.
	if _, err := exec.LookPath("grep"); err != nil {
		t.Skip("grep not available in test environment")
	}

	ctx, dir := testCtx(t)

	// Create a file with known content.
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello world\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// grep for something that doesn't exist — exit 1 expected.
	// Shell-quote the path so spaces/special chars don't break the command.
	escapedPath := "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'"
	cmdStr := "grep nonexistent " + escapedPath + "; grep_rc=$?; echo after-grep rc=$grep_rc"
	args, marshalErr := json.Marshal(map[string]string{"command": cmdStr})
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	result, err := shell.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	// The command should complete fully; "after-grep" should appear in output.
	if !strings.Contains(result.Output, "after-grep") {
		t.Fatalf("expected 'after-grep' in output (shell aborted early), got: %s", result.Output)
	}
	// Verify grep actually returned exit 1.
	if !strings.Contains(result.Output, "rc=1") {
		t.Fatalf("expected 'rc=1' (grep no-match exit code), got: %s", result.Output)
	}
}

func TestShell_TestFalse(t *testing.T) {
	// test returning exit 1 (false) should NOT cause the shell to abort.
	ctx, _ := testCtx(t)

	args, marshalErr := json.Marshal(map[string]string{"command": "test -f /nonexistent_file; test_rc=$?; echo after-test rc=$test_rc"})
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	result, err := shell.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "after-test") {
		t.Fatalf("expected 'after-test' in output (shell aborted early), got: %s", result.Output)
	}
	// Verify test actually returned exit 1.
	if !strings.Contains(result.Output, "rc=1") {
		t.Fatalf("expected 'rc=1' (test false exit code), got: %s", result.Output)
	}
}

func TestShell_PipefailStillActive(t *testing.T) {
	// set -o pipefail should still be active, catching mid-pipe failures.
	// Skip on shells that don't support pipefail (e.g., /bin/sh on some systems).
	si := detectShell()
	if !si.supportsPipefail {
		t.Skipf("skipping: detected shell %q does not support pipefail", si.path)
	}
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat not available in test environment")
	}

	ctx, _ := testCtx(t)

	// false | cat — with pipefail this should return non-zero (1 from false).
	// Without pipefail, the exit code would be 0 (from cat).
	args, marshalErr := json.Marshal(map[string]string{"command": "false | cat; echo exit=$?"})
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	result, err := shell.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	// pipefail should propagate the non-zero exit from false.
	if !strings.Contains(result.Output, "exit=1") {
		t.Fatalf("expected 'exit=1' (pipefail should propagate false's exit), got: %s", result.Output)
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
	if len(tools) != 41 {
		t.Fatalf("expected 41 built-in tools, got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, tl := range tools {
		names[tl.Name()] = true
	}

	expected := []string{
		"cairn.readFile", "cairn.writeFile", "cairn.editFile", "cairn.deleteFile", "cairn.undoEdit",
		"cairn.listFiles", "cairn.searchFiles", "cairn.shell", "cairn.gitRun",
		"cairn.createMemory", "cairn.searchMemory", "cairn.manageMemory",
		"cairn.readFeed", "cairn.markRead", "cairn.archiveFeedItem", "cairn.deleteFeedItem", "cairn.digest",
		"cairn.journalSearch",
		"cairn.webSearch", "cairn.webFetch",
		"cairn.createTask", "cairn.listTasks", "cairn.completeTask",
		"cairn.compose", "cairn.getStatus",
		"cairn.loadSkill", "cairn.listSkills", "cairn.createSkill", "cairn.editSkill", "cairn.deleteSkill",
		"cairn.notify",
		"cairn.createCron", "cairn.listCrons", "cairn.deleteCron",
		"cairn.patchConfig", "cairn.getConfig",
		"cairn.searchSkills", "cairn.skillInfo", "cairn.installSkill",
	}
	for _, name := range expected {
		if !names[name] {
			t.Fatalf("expected tool %q to be registered", name)
		}
	}
}

// --- Shell containment tests ---

func TestShell_ConfinedBlocksAbsolutePath(t *testing.T) {
	ctx, _ := confinedCtx(t)

	// Attempt to use an absolute workDir outside the confined directory.
	args := json.RawMessage(`{"command": "echo escaped", "workDir": "/tmp"}`)
	result, err := shell.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected containment error for absolute path outside workDir")
	}
	if !strings.Contains(result.Error, "shell containment") {
		t.Fatalf("expected shell containment error, got: %s", result.Error)
	}
}

func TestShell_ConfinedBlocksTraversal(t *testing.T) {
	ctx, _ := confinedCtx(t)

	// Attempt to escape via relative path traversal.
	args := json.RawMessage(`{"command": "echo escaped", "workDir": "../../.."}`)
	result, err := shell.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected containment error for path traversal")
	}
	if !strings.Contains(result.Error, "shell containment") {
		t.Fatalf("expected shell containment error, got: %s", result.Error)
	}
}

func TestShell_ConfinedAllowsRelative(t *testing.T) {
	ctx, dir := confinedCtx(t)

	// Create a subdirectory within the confined area.
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}

	// Relative path within workDir should succeed.
	args := json.RawMessage(`{"command": "pwd", "workDir": "subdir"}`)
	result, err := shell.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("expected success for relative path within workDir, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "subdir") {
		t.Fatalf("expected output to contain 'subdir', got: %s", result.Output)
	}
}

func TestShell_ConfinedDefaultWorkDir(t *testing.T) {
	ctx, _ := confinedCtx(t)

	// No workDir param - should use ctx.WorkDir (the confined directory) and succeed.
	args := json.RawMessage(`{"command": "echo ok"}`)
	result, err := shell.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("expected success with default workDir, got error: %s", result.Error)
	}
	if strings.TrimSpace(result.Output) != "ok" {
		t.Fatalf("expected 'ok', got: %q", result.Output)
	}
}

func TestShell_UnconfinedAllowsAnywhere(t *testing.T) {
	ctx, _ := testCtx(t)

	// Non-confined context should allow absolute paths.
	args := json.RawMessage(`{"command": "echo free", "workDir": "/tmp"}`)
	result, err := shell.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("non-confined shell should allow /tmp, got error: %s", result.Error)
	}
	if strings.TrimSpace(result.Output) != "free" {
		t.Fatalf("expected 'free', got: %q", result.Output)
	}
}

// --- Read-only shell policy tests ---

func readOnlyCtx(t *testing.T) (*tool.ToolContext, string) {
	t.Helper()
	dir := t.TempDir()
	return &tool.ToolContext{
		SessionID: "test-readonly",
		WorkDir:   dir,
		ReadOnly:  true,
		Cancel:    context.Background(),
	}, dir
}

func TestShell_ReadOnlyBlocksGitCommit(t *testing.T) {
	ctx, _ := readOnlyCtx(t)
	args := json.RawMessage(`{"command": "git commit -m 'test'"}`)
	result, err := shell.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" || !strings.Contains(result.Error, "read-only") {
		t.Fatalf("expected read-only denial for git commit, got: %q", result.Error)
	}
}

func TestShell_ReadOnlyBlocksGitPush(t *testing.T) {
	ctx, _ := readOnlyCtx(t)
	args := json.RawMessage(`{"command": "git push origin main"}`)
	result, err := shell.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" || !strings.Contains(result.Error, "read-only") {
		t.Fatalf("expected read-only denial for git push, got: %q", result.Error)
	}
}

func TestShell_ReadOnlyBlocksGhPrCreate(t *testing.T) {
	ctx, _ := readOnlyCtx(t)
	args := json.RawMessage(`{"command": "gh pr create --title test"}`)
	result, err := shell.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" || !strings.Contains(result.Error, "read-only") {
		t.Fatalf("expected read-only denial for gh pr create, got: %q", result.Error)
	}
}

func TestShell_ReadOnlyBlocksSedInPlace(t *testing.T) {
	ctx, _ := readOnlyCtx(t)
	args := json.RawMessage(`{"command": "sed -i 's/old/new/' file.txt"}`)
	result, err := shell.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" || !strings.Contains(result.Error, "read-only") {
		t.Fatalf("expected read-only denial for sed -i, got: %q", result.Error)
	}
}

func TestShell_ReadOnlyAllowsGitLog(t *testing.T) {
	ctx, dir := readOnlyCtx(t)
	// Init a git repo so git log works.
	if err := exec.Command("git", "init", dir).Run(); err != nil {
		t.Skipf("git init failed: %v", err)
	}
	args := json.RawMessage(`{"command": "git log --oneline -1 2>/dev/null; echo ok"}`)
	result, err := shell.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("git log should be allowed in read-only mode, got error: %s", result.Error)
	}
}

func TestShell_ReadOnlyAllowsGrep(t *testing.T) {
	ctx, dir := readOnlyCtx(t)
	if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}
	args := json.RawMessage(`{"command": "grep hello test.txt"}`)
	result, err := shell.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("grep should be allowed in read-only mode, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "hello") {
		t.Fatalf("expected grep output, got: %s", result.Output)
	}
}

func TestShell_ReadOnlyAllowsGhPrView(t *testing.T) {
	ctx, _ := readOnlyCtx(t)
	// gh pr view will fail (no repo) but should NOT be blocked by policy.
	args := json.RawMessage(`{"command": "gh pr view 1 2>&1; echo policy-passed"}`)
	result, err := shell.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The command should run (policy allows it), even if gh fails.
	if strings.Contains(result.Error, "read-only") {
		t.Fatalf("gh pr view should be allowed in read-only mode, got: %s", result.Error)
	}
}

func TestShell_NonReadOnlyAllowsGitCommit(t *testing.T) {
	ctx, _ := testCtx(t)
	// Non-read-only should allow git commit (it will fail without a repo, but not be policy-blocked).
	args := json.RawMessage(`{"command": "git commit -m test 2>&1; echo policy-passed"}`)
	result, err := shell.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result.Error, "read-only") {
		t.Fatalf("non-read-only context should allow git commit, got: %s", result.Error)
	}
}
