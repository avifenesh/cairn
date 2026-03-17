package task

import (
	"bufio"
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// WorktreeInfo describes an active git worktree.
type WorktreeInfo struct {
	TaskID string
	Path   string
	Branch string
}

// WorktreeManager creates and removes git worktrees for coding tasks.
// All git operations are serialized via a mutex.
type WorktreeManager struct {
	repoDir     string
	worktreeDir string
	mu          sync.Mutex
}

// NewWorktreeManager creates a manager that creates worktrees under worktreeDir
// branching from the repo at repoDir.
func NewWorktreeManager(repoDir, worktreeDir string) *WorktreeManager {
	return &WorktreeManager{
		repoDir:     repoDir,
		worktreeDir: worktreeDir,
	}
}

// Create adds a git worktree for the given task, branching from baseBranch.
// Returns the worktree path and branch name.
func (m *WorktreeManager) Create(taskID, baseBranch string) (worktreePath, branchName string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	branchName = "cairn/" + taskID
	worktreePath = filepath.Join(m.worktreeDir, taskID)

	// Ensure the worktree base directory exists.
	if err := os.MkdirAll(m.worktreeDir, 0o755); err != nil {
		return "", "", fmt.Errorf("worktree: mkdir %s: %w", m.worktreeDir, err)
	}

	cmd := exec.Command("git", "worktree", "add", worktreePath, "-b", branchName, baseBranch)
	cmd.Dir = m.repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("worktree: create %s: %w\n%s", taskID, err, string(out))
	}

	slog.Info("worktree created", "taskID", taskID, "path", worktreePath, "branch", branchName)
	return worktreePath, branchName, nil
}

// Remove deletes a worktree for the given task and prunes its branch.
func (m *WorktreeManager) Remove(taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	worktreePath := filepath.Join(m.worktreeDir, taskID)

	cmd := exec.Command("git", "worktree", "remove", worktreePath, "--force")
	cmd.Dir = m.repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("worktree: remove %s: %w\n%s", taskID, err, string(out))
	}

	// Also delete the branch.
	branchName := "cairn/" + taskID
	cmd = exec.Command("git", "branch", "-D", branchName)
	cmd.Dir = m.repoDir
	_ = cmd.Run() // best-effort; branch may already be gone

	slog.Info("worktree removed", "taskID", taskID, "path", worktreePath)
	return nil
}

// List returns all worktrees managed under the worktreeDir by parsing
// `git worktree list --porcelain`.
func (m *WorktreeManager) List() ([]WorktreeInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = m.repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("worktree: list: %w", err)
	}

	return parseWorktreeList(out, m.worktreeDir), nil
}

// parseWorktreeList parses the porcelain output of `git worktree list`.
// It filters to only worktrees under baseDir.
func parseWorktreeList(data []byte, baseDir string) []WorktreeInfo {
	var results []WorktreeInfo
	var current WorktreeInfo

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			// End of a worktree entry.
			if current.Path != "" && strings.HasPrefix(current.Path, baseDir) {
				// Extract taskID from path.
				current.TaskID = filepath.Base(current.Path)
				results = append(results, current)
			}
			current = WorktreeInfo{}
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			current.Path = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch ") {
			ref := strings.TrimPrefix(line, "branch ")
			// ref is like "refs/heads/cairn/abc123"
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		}
	}

	// Handle last entry if no trailing newline.
	if current.Path != "" && strings.HasPrefix(current.Path, baseDir) {
		current.TaskID = filepath.Base(current.Path)
		results = append(results, current)
	}

	return results
}
