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
	repos       map[string]bool   // normalized absolute paths of allowed repos
	taskRepos   map[string]string // taskID -> repoDir used at creation (for Remove)
	worktreeDir string
	mu          sync.Mutex
}

// NewWorktreeManager creates a manager that creates worktrees under worktreeDir.
// allowedRepos lists additional repo paths beyond defaultRepo. If empty, only
// defaultRepo is available. defaultRepo is always included in the allowed set.
func NewWorktreeManager(defaultRepo, worktreeDir string, allowedRepos []string) *WorktreeManager {
	// Normalize defaultRepo the same way as allowedRepos.
	if abs, err := filepath.Abs(defaultRepo); err == nil {
		defaultRepo = filepath.Clean(abs)
	}

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
		taskRepos:   make(map[string]string),
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

	// Track which repo this worktree was created from (needed for Remove).
	m.taskRepos[taskID] = selectedRepo

	slog.Info("worktree created", "taskID", taskID, "path", worktreePath, "branch", branchName, "repo", selectedRepo)
	return worktreePath, branchName, nil
}

// Remove deletes a worktree for the given task and prunes its branch.
// Uses the repo that the worktree was created from (tracked internally).
func (m *WorktreeManager) Remove(taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Use the repo that created this worktree; fall back to default.
	repoDir := m.defaultRepo
	if tracked, ok := m.taskRepos[taskID]; ok {
		repoDir = tracked
		delete(m.taskRepos, taskID)
	}

	worktreePath := filepath.Join(m.worktreeDir, taskID)

	cmd := exec.Command("git", "worktree", "remove", worktreePath, "--force")
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("worktree: remove %s: %w\n%s", taskID, err, string(out))
	}

	// Also delete the branch from the correct repo.
	branchName := "cairn/" + taskID
	cmd = exec.Command("git", "branch", "-D", branchName)
	cmd.Dir = repoDir
	_ = cmd.Run() // best-effort; branch may already be gone

	slog.Info("worktree removed", "taskID", taskID, "path", worktreePath, "repo", repoDir)
	return nil
}

// List returns all worktrees managed under the worktreeDir by parsing
// `git worktree list --porcelain` from each registered repo.
func (m *WorktreeManager) List() ([]WorktreeInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	seen := make(map[string]bool) // dedup by path
	var results []WorktreeInfo

	for repo := range m.repos {
		cmd := exec.Command("git", "worktree", "list", "--porcelain")
		cmd.Dir = repo
		out, err := cmd.Output()
		if err != nil {
			continue // skip repos that fail (e.g. missing)
		}
		for _, info := range parseWorktreeList(out, m.worktreeDir) {
			if !seen[info.Path] {
				info.RepoDir = repo
				seen[info.Path] = true
				results = append(results, info)
			}
		}
	}

	return results, nil
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
