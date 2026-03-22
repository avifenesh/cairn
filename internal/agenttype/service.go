package agenttype

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// safeAgentNameRe matches valid agent type names: lowercase alphanumeric with hyphens, max 64 chars.
var safeAgentNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}[a-z0-9]$|^[a-z0-9]$`)

// validateName checks if an agent type name is safe for filesystem use.
// Mirrors the skill.ValidateName pattern.
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("agent type name is required")
	}
	if name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("agent type name %q is not safe", name)
	}
	if !safeAgentNameRe.MatchString(name) {
		return fmt.Errorf("agent type name %q must be lowercase alphanumeric with hyphens, 1-64 chars", name)
	}
	return nil
}

// Service discovers, caches, and hot-reloads agent types from one or more
// directories. Each directory contains subdirectories named after agent types,
// each with an AGENT.md file.
type Service struct {
	mu       sync.RWMutex
	types    map[string]*AgentType
	dirs     []string
	logger   *slog.Logger
	onChange func()

	modTimes map[string]time.Time
}

// NewService creates a service that scans the given directories.
func NewService(dirs []string, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		types:    make(map[string]*AgentType),
		dirs:     dirs,
		logger:   logger,
		modTimes: make(map[string]time.Time),
	}
}

// Discover scans all configured directories for AGENT.md files and
// registers each parsed agent type. Existing types are replaced.
func (s *Service) Discover() error {
	found := make(map[string]*AgentType)
	modTimes := make(map[string]time.Time)

	for _, dir := range s.dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				s.logger.Debug("agent type dir does not exist, skipping", "dir", dir)
				continue
			}
			return fmt.Errorf("agenttype: read dir %q: %w", dir, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			agentPath := filepath.Join(dir, entry.Name(), "AGENT.md")
			info, err := os.Stat(agentPath)
			if err != nil {
				continue
			}

			at, err := Parse(agentPath)
			if err != nil {
				s.logger.Warn("agenttype: parse failed", "path", agentPath, "error", err)
				continue
			}

			found[at.Name] = at
			modTimes[agentPath] = info.ModTime()
		}
	}

	s.mu.Lock()
	s.types = found
	s.modTimes = modTimes
	s.mu.Unlock()

	s.logger.Debug("agent types discovered", "count", len(found))
	return nil
}

// Get returns an agent type by name, or nil if not found.
func (s *Service) Get(name string) *AgentType {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.types[name]
}

// List returns all discovered agent types.
func (s *Service) List() []*AgentType {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*AgentType, 0, len(s.types))
	for _, at := range s.types {
		result = append(result, at)
	}
	return result
}

// OnChange registers a callback invoked after agent types are re-discovered.
func (s *Service) OnChange(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = fn
}

// Watch polls directories every 5 seconds and re-discovers on change.
// Blocks until ctx is cancelled.
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

func (s *Service) checkReload() {
	if !s.hasChanges() {
		return
	}

	if err := s.Discover(); err != nil {
		s.logger.Warn("agenttype: re-discover failed", "error", err)
		return
	}

	s.logger.Info("agent types reloaded")

	s.mu.RLock()
	fn := s.onChange
	s.mu.RUnlock()

	if fn != nil {
		fn()
	}
}

// InstallDir returns the last configured directory (user override).
func (s *Service) InstallDir() string {
	if len(s.dirs) == 0 {
		return ""
	}
	return s.dirs[len(s.dirs)-1]
}

// Create writes a new AGENT.md and re-discovers.
func (s *Service) Create(name, content string) error {
	if err := validateName(name); err != nil {
		return err
	}
	if len(s.dirs) == 0 {
		return fmt.Errorf("no agent type directories configured")
	}
	if s.Get(name) != nil {
		return fmt.Errorf("agent type %q already exists", name)
	}

	targetDir := s.dirs[len(s.dirs)-1]
	dir := filepath.Join(targetDir, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create agent type dir: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte(content), 0644); err != nil {
		os.RemoveAll(dir)
		return fmt.Errorf("write AGENT.md: %w", err)
	}

	return s.Discover()
}

// Update overwrites the AGENT.md content and re-discovers.
func (s *Service) Update(name, content string) error {
	if err := validateName(name); err != nil {
		return err
	}
	at := s.Get(name)
	if at == nil {
		return fmt.Errorf("agent type %q not found", name)
	}
	if err := os.WriteFile(at.Location, []byte(content), 0644); err != nil {
		return fmt.Errorf("write AGENT.md: %w", err)
	}
	return s.Discover()
}

// Delete removes an agent type directory and re-discovers.
func (s *Service) Delete(name string) error {
	at := s.Get(name)
	if at == nil {
		return fmt.Errorf("agent type %q not found", name)
	}

	loc := at.Location
	if filepath.Base(loc) == "AGENT.md" {
		loc = filepath.Dir(loc)
	}
	if err := os.RemoveAll(loc); err != nil {
		return fmt.Errorf("remove agent type dir: %w", err)
	}

	return s.Discover()
}

// hasChanges checks whether any AGENT.md files were added, removed, or modified.
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

			agentPath := filepath.Join(dir, entry.Name(), "AGENT.md")
			info, err := os.Stat(agentPath)
			if err != nil {
				continue
			}
			currentPaths[agentPath] = info.ModTime()
		}
	}

	for path, modTime := range currentPaths {
		oldMod, exists := oldModTimes[path]
		if !exists || !modTime.Equal(oldMod) {
			return true
		}
	}

	for path := range oldModTimes {
		if _, exists := currentPaths[path]; !exists {
			return true
		}
	}

	return false
}
