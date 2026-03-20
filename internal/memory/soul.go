package memory

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

// PendingSoulPatch represents a proposed change to SOUL.md awaiting human review.
type PendingSoulPatch struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"` // the patch text to append
	Source    string    `json:"source"`  // "reflection", "agent", "idle"
	CreatedAt time.Time `json:"createdAt"`
	Preview   string    `json:"preview"` // full SOUL.md content after applying
}

// Soul loads and hot-reloads a SOUL.md file that defines the agent's
// procedural memory — identity, rules, and behavioral guidelines.
type Soul struct {
	mu       sync.RWMutex
	content  string
	filePath string
	modTime  time.Time
	onChange func(content string)
	pending  *PendingSoulPatch
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

// FilePath returns the path to the SOUL.md file.
func (s *Soul) FilePath() string {
	return s.filePath
}

// ProposePatch creates a pending patch for human review.
// Only one patch can be pending at a time; new proposals replace old ones.
func (s *Soul) ProposePatch(content, source string) *PendingSoulPatch {
	s.mu.Lock()
	defer s.mu.Unlock()

	preview := s.content + "\n" + content
	patch := &PendingSoulPatch{
		ID:        fmt.Sprintf("sp_%d", time.Now().UnixMilli()),
		Content:   content,
		Source:    source,
		CreatedAt: time.Now(),
		Preview:   preview,
	}
	s.pending = patch
	slog.Info("soul: patch proposed", "id", patch.ID, "source", source, "bytes", len(content))
	return patch
}

// PendingPatch returns the current pending patch, or nil if none.
func (s *Soul) PendingPatch() *PendingSoulPatch {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pending
}

// ApprovePatch applies the pending patch to the file and clears it.
func (s *Soul) ApprovePatch(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.pending == nil {
		return fmt.Errorf("soul: no pending patch")
	}
	if s.pending.ID != id {
		return fmt.Errorf("soul: no pending patch with id %q", id)
	}

	// Verify the patch preview is still valid (content hasn't changed since proposal).
	expectedPreview := s.content + "\n" + s.pending.Content
	if expectedPreview != s.pending.Preview {
		// Content changed since patch was proposed - rebase the preview.
		s.pending.Preview = expectedPreview
		slog.Info("soul: patch rebased onto current content", "id", id)
	}

	newContent := expectedPreview
	if err := os.WriteFile(s.filePath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("soul: write patch: %w", err)
	}

	s.content = newContent
	source := s.pending.Source
	s.pending = nil

	slog.Info("soul: patch approved and applied", "id", id, "source", source)
	return nil
}

// DenyPatch clears the pending patch without applying it.
func (s *Soul) DenyPatch(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.pending == nil {
		return fmt.Errorf("soul: no pending patch")
	}
	if s.pending.ID != id {
		return fmt.Errorf("soul: no pending patch with id %q", id)
	}

	s.pending = nil
	slog.Info("soul: patch denied", "id", id)
	return nil
}
