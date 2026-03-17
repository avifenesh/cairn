package memory

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

// Soul loads and hot-reloads a SOUL.md file that defines the agent's
// procedural memory — identity, rules, and behavioral guidelines.
type Soul struct {
	mu       sync.RWMutex
	content  string
	filePath string
	modTime  time.Time
	onChange func(content string)
}

// NewSoul creates a Soul bound to the given file path.
// Call Load() to read the initial content, then Watch() to auto-reload.
func NewSoul(filePath string) *Soul {
	return &Soul{filePath: filePath}
}

// Load reads the file into memory. Safe to call multiple times.
func (s *Soul) Load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return fmt.Errorf("soul: load %q: %w", s.filePath, err)
	}

	info, err := os.Stat(s.filePath)
	if err != nil {
		return fmt.Errorf("soul: stat %q: %w", s.filePath, err)
	}

	s.mu.Lock()
	s.content = string(data)
	s.modTime = info.ModTime()
	s.mu.Unlock()

	slog.Debug("soul loaded", "path", s.filePath, "bytes", len(data))
	return nil
}

// Content returns the current SOUL.md content (thread-safe).
func (s *Soul) Content() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.content
}

// OnChange registers a callback invoked after each successful reload.
func (s *Soul) OnChange(fn func(string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = fn
}

// Watch polls the file for modifications every 5 seconds and reloads
// on change. It blocks until ctx is cancelled. Typically run as a goroutine.
func (s *Soul) Watch(ctx context.Context) error {
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

// checkReload stats the file and reloads if the modification time changed.
func (s *Soul) checkReload() {
	info, err := os.Stat(s.filePath)
	if err != nil {
		slog.Warn("soul: stat failed during watch", "path", s.filePath, "error", err)
		return
	}

	s.mu.RLock()
	changed := info.ModTime() != s.modTime
	s.mu.RUnlock()

	if !changed {
		return
	}

	if err := s.Load(); err != nil {
		slog.Warn("soul: reload failed", "path", s.filePath, "error", err)
		return
	}

	slog.Info("soul reloaded", "path", s.filePath)

	s.mu.RLock()
	fn := s.onChange
	content := s.content
	s.mu.RUnlock()

	if fn != nil {
		fn(content)
	}
}
