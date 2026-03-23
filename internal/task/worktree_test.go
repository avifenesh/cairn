package task

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initTestRepo creates a temporary git repo with an initial commit.
func initTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init", "-b", "main"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	// Create a file and commit so there is a HEAD.
	readmePath := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test repo\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}

	cmds = [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "initial commit"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	return dir
}

func TestWorktree_CreateAndRemove(t *testing.T) {
	repoDir := initTestRepo(t)
	wtDir := filepath.Join(t.TempDir(), "worktrees")

	m := NewWorktreeManager(repoDir, wtDir, nil)

	taskID := "test-task-001"
	wtPath, branchName, err := m.Create(taskID, "main", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Verify worktree directory exists.
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Fatalf("worktree path %q does not exist", wtPath)
	}

	// Verify branch name.
	expectedBranch := "cairn/" + taskID
	if branchName != expectedBranch {
		t.Errorf("Branch: got %q, want %q", branchName, expectedBranch)
	}

	// Verify it is a valid git worktree by checking for .git file.
	gitFile := filepath.Join(wtPath, ".git")
	if _, err := os.Stat(gitFile); os.IsNotExist(err) {
		t.Fatalf("worktree %q missing .git file", wtPath)
	}

	// Remove the worktree.
	if err := m.Remove(taskID); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Verify worktree directory is gone.
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree path %q still exists after removal", wtPath)
	}
}

func TestWorktree_List(t *testing.T) {
	repoDir := initTestRepo(t)
	wtDir := filepath.Join(t.TempDir(), "worktrees")

	m := NewWorktreeManager(repoDir, wtDir, nil)

	// Create two worktrees.
	ids := []string{"task-a", "task-b"}
	for _, id := range ids {
		if _, _, err := m.Create(id, "main", ""); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
	}

	// List worktrees.
	list, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(list) != 2 {
		t.Fatalf("List: got %d worktrees, want 2", len(list))
	}

	// Verify both are present.
	found := make(map[string]bool)
	for _, info := range list {
		found[info.TaskID] = true
		if info.Branch == "" {
			t.Errorf("worktree %s has empty branch", info.TaskID)
		}
		if info.Path == "" {
			t.Errorf("worktree %s has empty path", info.TaskID)
		}
	}

	for _, id := range ids {
		if !found[id] {
			t.Errorf("worktree %q not found in list", id)
		}
	}

	// Cleanup.
	for _, id := range ids {
		m.Remove(id)
	}
}

func TestWorktree_CreateDefaultRepo(t *testing.T) {
	repoDir := initTestRepo(t)
	wtDir := filepath.Join(t.TempDir(), "worktrees")

	m := NewWorktreeManager(repoDir, wtDir, nil)

	// Empty repoDir should use the default.
	wtPath, _, err := m.Create("default-test", "main", "")
	if err != nil {
		t.Fatalf("Create with empty repoDir: %v", err)
	}
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Fatalf("worktree path %q does not exist", wtPath)
	}
	m.Remove("default-test")
}

func TestWorktree_CreateSpecificRepo(t *testing.T) {
	repo1 := initTestRepo(t)
	repo2 := initTestRepo(t)
	wtDir := filepath.Join(t.TempDir(), "worktrees")

	m := NewWorktreeManager(repo1, wtDir, []string{repo2})

	// Create in the second repo explicitly.
	wtPath, _, err := m.Create("cross-repo-test", "main", repo2)
	if err != nil {
		t.Fatalf("Create in repo2: %v", err)
	}
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		t.Fatalf("worktree path %q does not exist", wtPath)
	}

	// The worktree should be a git dir with content from repo2.
	gitFile := filepath.Join(wtPath, ".git")
	if _, err := os.Stat(gitFile); os.IsNotExist(err) {
		t.Fatalf("worktree %q missing .git file", wtPath)
	}

	// Cleanup — remove needs to work from any repo's worktree dir.
	m.Remove("cross-repo-test")
}

func TestWorktree_CreateInvalidRepo(t *testing.T) {
	repoDir := initTestRepo(t)
	wtDir := filepath.Join(t.TempDir(), "worktrees")

	m := NewWorktreeManager(repoDir, wtDir, nil)

	// Try to create in a repo not in the allowed list.
	_, _, err := m.Create("bad-repo-test", "main", "/tmp/not-allowed")
	if err == nil {
		t.Fatal("expected error for invalid repo, got nil")
	}
	if want := "not in allowed repos"; !contains(err.Error(), want) {
		t.Errorf("error = %q, want containing %q", err.Error(), want)
	}
}

func TestWorktree_RepoDirs(t *testing.T) {
	repo1 := initTestRepo(t)
	repo2 := initTestRepo(t)
	wtDir := filepath.Join(t.TempDir(), "worktrees")

	m := NewWorktreeManager(repo1, wtDir, []string{repo2})

	dirs := m.RepoDirs()
	if len(dirs) != 2 {
		t.Fatalf("RepoDirs: got %d, want 2", len(dirs))
	}

	found := make(map[string]bool)
	for _, d := range dirs {
		found[d] = true
	}

	absRepo1, _ := filepath.Abs(repo1)
	absRepo2, _ := filepath.Abs(repo2)
	if !found[absRepo1] && !found[repo1] {
		t.Errorf("RepoDirs missing repo1 %q", repo1)
	}
	if !found[absRepo2] && !found[repo2] {
		t.Errorf("RepoDirs missing repo2 %q", repo2)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsHelper(s, sub))
}

func containsHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
