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
	TaskID  string
	Path    string
	Branch  string
	RepoDir string // which repository this worktree belongs to
}

// WorktreeManager creates and removes git worktrees for coding tasks.
// Supports multiple repositories — worktrees can be created in any allowed repo.
// All git operations are serialized via a mutex.
type WorktreeManager struct {
	defaultRepo string
	repos       map[string]bool // normalized absolute paths of allowed repos
	worktreeDir string
	mu          sync.Mutex
}

// NewWorktreeManager creates a manager that creates worktrees under worktreeDir.
// allowedRepos lists additional repo paths beyond defaultRepo. If empty, only
// defaultRepo is available. defaultRepo is always included in the allowed set.
func NewWorktreeManager(defaultRepo, worktreeDir string, allowedRepos []string) *WorktreeManager {
	repos := make(map[string]bool)
	repos[defaultRepo] = true
	for _, r := range allowedRepos {
		r = strings.TrimSpace(r)
		if r != "" {
			if abs, err := filepath.Abs(r); err == nil {
				r = filepath.Clean(abs)
			}
			repos[r] = true
		}
	}
	return &WorktreeManager{
		defaultRepo: defaultRepo,
		repos:       repos,
		worktreeDir: worktreeDir,
	}
}

// RepoDir returns the default repository directory.
func (m *WorktreeManager) RepoDir() string {
	return m.defaultRepo
}

// RepoDirs returns all allowed repository directories.
func (m *WorktreeManager) RepoDirs() []string {
	dirs := make([]string, 0, len(m.repos))
	for r := range m.repos {
		dirs = append(dirs, r)
	}
	return dirs
}

// Create adds a git worktree for the given task, branching from baseBranch.
// repoDir selects which repository to branch from. Empty string uses the default.
// Returns the worktree path and branch name.
func (m *WorktreeManager) Create(taskID, baseBranch, repoDir string) (worktreePath, branchName string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	selectedRepo := m.defaultRepo
	if repoDir != "" {
		normalized := repoDir
		if abs, err := filepath.Abs(repoDir); err == nil {
			normalized = filepath.Clean(abs)
		}
		if !m.repos[normalized] {
			return "", "", fmt.Errorf("worktree: repo %q not in allowed repos", repoDir)
		}
		selectedRepo = normalized
	}

	branchName = "cairn/" + taskID
	worktreePath = filepath.Join(m.worktreeDir, taskID)

	// Ensure the worktree base directory exists.
	if err := os.MkdirAll(m.worktreeDir, 0o755); err != nil {
		return "", "", fmt.Errorf("worktree: mkdir %s: %w", m.worktreeDir, err)
	}

	cmd := exec.Command("git", "worktree", "add", worktreePath, "-b", branchName, baseBranch)
	cmd.Dir = selectedRepo
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("worktree: create %s: %w\n%s", taskID, err, string(out))
	}

	slog.Info("worktree created", "taskID", taskID, "path", worktreePath, "branch", branchName, "repo", selectedRepo)
	return worktreePath, branchName, nil
}

// Remove deletes a worktree for the given task and prunes its branch.
func (m *WorktreeManager) Remove(taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	worktreePath := filepath.Join(m.worktreeDir, taskID)

	cmd := exec.Command("git", "worktree", "remove", worktreePath, "--force")
	cmd.Dir = m.defaultRepo
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("worktree: remove %s: %w\n%s", taskID, err, string(out))
	}

	// Also delete the branch.
	branchName := "cairn/" + taskID
	cmd = exec.Command("git", "branch", "-D", branchName)
	cmd.Dir = m.defaultRepo
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
	cmd.Dir = m.defaultRepo
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
			absPath, _ := filepath.Abs(current.Path)
			absBase, _ := filepath.Abs(baseDir)
			if current.Path != "" && strings.HasPrefix(absPath, absBase+string(filepath.Separator)) {
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
	absPath, _ := filepath.Abs(current.Path)
	absBase, _ := filepath.Abs(baseDir)
	if current.Path != "" && strings.HasPrefix(absPath, absBase+string(filepath.Separator)) {
		current.TaskID = filepath.Base(current.Path)
		results = append(results, current)
	}

	return results
}
