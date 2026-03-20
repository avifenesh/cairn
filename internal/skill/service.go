package skill

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Service discovers, caches, and hot-reloads skills from one or more
// directories. Each directory is expected to contain subdirectories
// named after skills, each with a SKILL.md file:
//
//	{dir}/{skill-name}/SKILL.md
type Service struct {
	mu       sync.RWMutex
	skills   map[string]*Skill
	dirs     []string
	logger   *slog.Logger
	onChange func()

	// modTimes tracks the last-seen mod time per SKILL.md path.
	modTimes map[string]time.Time
}

// NewService creates a skill service that will scan the given directories.
func NewService(dirs []string, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		skills:   make(map[string]*Skill),
		dirs:     dirs,
		logger:   logger,
		modTimes: make(map[string]time.Time),
	}
}

// Discover scans all configured directories for SKILL.md files and
// registers each parsed skill. Existing skills are replaced.
func (s *Service) Discover() error {
	found := make(map[string]*Skill)
	modTimes := make(map[string]time.Time)

	for _, dir := range s.dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				s.logger.Debug("skill dir does not exist, skipping", "dir", dir)
				continue
			}
			return fmt.Errorf("skill: read dir %q: %w", dir, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			skillPath := filepath.Join(dir, entry.Name(), "SKILL.md")
			info, err := os.Stat(skillPath)
			if err != nil {
				continue // no SKILL.md in this subdirectory
			}

			sk, err := Parse(skillPath)
			if err != nil {
				s.logger.Warn("skill: parse failed", "path", skillPath, "error", err)
				continue
			}

			found[sk.Name] = sk
			modTimes[skillPath] = info.ModTime()
		}
	}

	s.mu.Lock()
	s.skills = found
	s.modTimes = modTimes
	s.mu.Unlock()

	s.logger.Debug("skills discovered", "count", len(found))
	return nil
}

// Get returns a skill by name, or nil if not found.
func (s *Service) Get(name string) *Skill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.skills[name]
}

// List returns all discovered skills.
func (s *Service) List() []*Skill {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Skill, 0, len(s.skills))
	for _, sk := range s.skills {
		result = append(result, sk)
	}
	return result
}

// ForPrompt returns skills filtered by inclusion type.
func (s *Service) ForPrompt(inclusion Inclusion) []*Skill {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Skill
	for _, sk := range s.skills {
		if sk.Inclusion == inclusion {
			result = append(result, sk)
		}
	}
	return result
}

// OnChange registers a callback invoked after skills are re-discovered
// due to file changes. The callback runs outside the lock.
func (s *Service) OnChange(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = fn
}

// Watch polls skill directories every 5 seconds and re-discovers on change.
// It blocks until ctx is cancelled. Typically run as a goroutine.
func (s *Service) Watch(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			s.checkReload()
		}
	}
}

// checkReload stats all known SKILL.md files and any new subdirectories,
// then re-discovers if anything changed.
func (s *Service) checkReload() {
	if !s.hasChanges() {
		return
	}

	if err := s.Discover(); err != nil {
		s.logger.Warn("skill: re-discover failed", "error", err)
		return
	}

	s.logger.Info("skills reloaded")

	s.mu.RLock()
	fn := s.onChange
	s.mu.RUnlock()

	if fn != nil {
		fn()
	}
}

// hasChanges checks whether any SKILL.md files were added, removed, or modified.
func (s *Service) hasChanges() bool {
	s.mu.RLock()
	oldModTimes := s.modTimes
	s.mu.RUnlock()

	currentPaths := make(map[string]time.Time)

	for _, dir := range s.dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			skillPath := filepath.Join(dir, entry.Name(), "SKILL.md")
			info, err := os.Stat(skillPath)
			if err != nil {
				continue
			}
			currentPaths[skillPath] = info.ModTime()
		}
	}

	// Check for additions or modifications.
	for path, modTime := range currentPaths {
		oldMod, exists := oldModTimes[path]
		if !exists || !modTime.Equal(oldMod) {
			return true
		}
	}

	// Check for removals.
	for path := range oldModTimes {
		if _, exists := currentPaths[path]; !exists {
			return true
		}
	}

	return false
}

// Create writes a new SKILL.md to the last (user override) skill directory and re-discovers.
func (s *Service) Create(name, description, content, inclusion string, allowedTools []string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if err := validateInclusion(inclusion); err != nil {
		return err
	}
	if s.Get(name) != nil {
		return fmt.Errorf("skill %q already exists", name)
	}
	if len(s.dirs) == 0 {
		return fmt.Errorf("no skill directories configured")
	}

	// Use last directory (user override), not first (bundled/project skills).
	targetDir := s.dirs[len(s.dirs)-1]
	dir := filepath.Join(targetDir, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}

	// Cairn owns this machine — no auto-gating for shell tools.
	// Only explicitly set disable-model-invocation if the caller requests it.
	md := buildSkillMD(name, description, content, inclusion, allowedTools, false)
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(md), 0644); err != nil {
		os.RemoveAll(dir)
		return fmt.Errorf("write SKILL.md: %w", err)
	}

	return s.Discover()
}

func hasShellTool(tools []string) bool {
	for _, t := range tools {
		if t == "cairn.shell" {
			return true
		}
	}
	return false
}

// Update modifies an existing skill's SKILL.md and re-discovers.
func (s *Service) Update(name, description, content, inclusion string, allowedTools []string) error {
	sk := s.Get(name)
	if sk == nil {
		return fmt.Errorf("skill %q not found", name)
	}
	if err := validateInclusion(inclusion); err != nil {
		return err
	}

	// Merge: use existing values for empty fields.
	if description == "" {
		description = sk.Description
	}
	if content == "" {
		content = sk.Content
	}
	if inclusion == "" {
		inclusion = string(sk.Inclusion)
	}
	// nil = keep existing, non-nil (even empty) = set explicitly.
	if allowedTools == nil {
		allowedTools = sk.AllowedTools
	}

	// Preserve existing disable-model-invocation flag if set.
	md := buildSkillMD(name, description, content, inclusion, allowedTools, sk.DisableModel)
	loc := sk.Location
	if filepath.Base(loc) == "SKILL.md" {
		loc = filepath.Dir(loc)
	}
	if err := os.WriteFile(filepath.Join(loc, "SKILL.md"), []byte(md), 0644); err != nil {
		return fmt.Errorf("write SKILL.md: %w", err)
	}

	return s.Discover()
}

// Delete removes a skill directory and re-discovers.
func (s *Service) Delete(name string) error {
	sk := s.Get(name)
	if sk == nil {
		return fmt.Errorf("skill %q not found", name)
	}

	loc := sk.Location
	if filepath.Base(loc) == "SKILL.md" {
		loc = filepath.Dir(loc)
	}
	if err := os.RemoveAll(loc); err != nil {
		return fmt.Errorf("remove skill dir: %w", err)
	}

	return s.Discover()
}

// validateInclusion checks if inclusion is a valid value.
func validateInclusion(inclusion string) error {
	if inclusion == "" {
		return nil // will default to on-demand
	}
	if inclusion != "always" && inclusion != "on-demand" {
		return fmt.Errorf("inclusion must be 'always' or 'on-demand', got %q", inclusion)
	}
	return nil
}

// buildSkillMD generates a SKILL.md with optional disable-model-invocation flag.
func buildSkillMD(name, description, content, inclusion string, allowedTools []string, disableModel bool) string {
	if inclusion == "" {
		inclusion = "on-demand"
	}
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", name)
	fmt.Fprintf(&b, "description: %q\n", description)
	fmt.Fprintf(&b, "inclusion: %s\n", inclusion)
	if len(allowedTools) > 0 {
		fmt.Fprintf(&b, "allowed-tools: %q\n", strings.Join(allowedTools, ","))
	}
	if disableModel {
		b.WriteString("disable-model-invocation: true\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(content)
	b.WriteString("\n")
	return b.String()
}
