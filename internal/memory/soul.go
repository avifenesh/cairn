package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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

// DeniedPatch records a soul patch that was rejected by the user.
type DeniedPatch struct {
	Content  string    `json:"content"`
	Reason   string    `json:"reason,omitempty"`
	DeniedAt time.Time `json:"deniedAt"`
}

// maxDeniedPatches limits how many denied patches are retained.
const maxDeniedPatches = 20

// Soul loads and hot-reloads a SOUL.md file that defines the agent's
// procedural memory — identity, rules, and behavioral guidelines.
type Soul struct {
	mu       sync.RWMutex
	content  string
	filePath string
	modTime  time.Time
	onChange func(content string)
	pending  *PendingSoulPatch
	denied   []DeniedPatch
}

// NewSoul creates a Soul bound to the given file path.
// Call Load() to read the initial content, then Watch() to auto-reload.
func NewSoul(filePath string) *Soul {
	return &Soul{filePath: filePath}
}

// patchFilePath returns the path for the persisted pending patch file.
func (s *Soul) patchFilePath() string {
	return filepath.Join(filepath.Dir(s.filePath), ".soul_patch.json")
}

// deniedFilePath returns the path for the persisted denied patches file.
func (s *Soul) deniedFilePath() string {
	return filepath.Join(filepath.Dir(s.filePath), ".soul_denied.json")
}

// Load reads the file into memory and restores any pending patch. Safe to call multiple times.
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

	// Restore pending patch from disk.
	if patchData, err := os.ReadFile(s.patchFilePath()); err == nil {
		var pp persistablePatch
		if json.Unmarshal(patchData, &pp) == nil && pp.ID != "" {
			s.pending = &PendingSoulPatch{
				ID:        pp.ID,
				Content:   pp.Content,
				Source:    pp.Source,
				CreatedAt: pp.CreatedAt,
				Preview:   s.content + "\n" + pp.Content,
			}
			slog.Info("soul: restored pending patch from disk", "id", pp.ID)
		}
	}

	// Restore denied patches from disk.
	if deniedData, err := os.ReadFile(s.deniedFilePath()); err == nil {
		var denied []DeniedPatch
		if json.Unmarshal(deniedData, &denied) == nil {
			s.denied = denied
		}
	}
	s.mu.Unlock()

	slog.Debug("soul loaded", "path", s.filePath, "bytes", len(data))
	return nil
}

// persistablePatch is the on-disk representation (excludes Preview to save space).
type persistablePatch struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"createdAt"`
}

// persistPatch writes the pending patch to disk (or removes the file if nil).
// Uses write-rename for atomic writes. Must be called with s.mu held.
func (s *Soul) persistPatch() {
	path := s.patchFilePath()
	if s.pending == nil {
		os.Remove(path)
		return
	}
	p := persistablePatch{
		ID:        s.pending.ID,
		Content:   s.pending.Content,
		Source:    s.pending.Source,
		CreatedAt: s.pending.CreatedAt,
	}
	data, err := json.Marshal(p)
	if err != nil {
		slog.Warn("soul: failed to persist patch", "error", err)
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		slog.Warn("soul: failed to write patch file", "error", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		slog.Warn("soul: failed to rename patch file", "error", err)
		os.Remove(tmp)
	}
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

// ProposePatch creates a pending patch for human review and persists it to disk.
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
	s.persistPatch()
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
	s.persistPatch() // removes the file

	slog.Info("soul: patch approved and applied", "id", id, "source", source)
	return nil
}

// DenyPatch clears the pending patch without applying it and records the
// denied content so future reflection cycles avoid re-proposing it.
func (s *Soul) DenyPatch(id string) error {
	return s.DenyPatchWithReason(id, "")
}

// DenyPatchWithReason denies the patch and records the reason.
func (s *Soul) DenyPatchWithReason(id, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.pending == nil {
		return fmt.Errorf("soul: no pending patch")
	}
	if s.pending.ID != id {
		return fmt.Errorf("soul: no pending patch with id %q", id)
	}

	// Record denied patch content for future reflection cycles.
	s.denied = append(s.denied, DeniedPatch{
		Content:  s.pending.Content,
		Reason:   reason,
		DeniedAt: time.Now(),
	})
	// Keep only recent denials.
	if len(s.denied) > maxDeniedPatches {
		s.denied = s.denied[len(s.denied)-maxDeniedPatches:]
	}
	s.persistDenied()

	s.pending = nil
	s.persistPatch() // removes the file
	slog.Info("soul: patch denied", "id", id, "reason", reason)
	return nil
}

// DeniedPatches returns a copy of recently denied patches (thread-safe).
func (s *Soul) DeniedPatches() []DeniedPatch {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]DeniedPatch, len(s.denied))
	copy(out, s.denied)
	return out
}

// persistDenied writes the denied patches list to disk. Must be called with s.mu held.
func (s *Soul) persistDenied() {
	path := s.deniedFilePath()
	if len(s.denied) == 0 {
		os.Remove(path)
		return
	}
	data, err := json.Marshal(s.denied)
	if err != nil {
		slog.Warn("soul: failed to persist denied patches", "error", err)
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		slog.Warn("soul: failed to write denied patches file", "error", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		slog.Warn("soul: failed to rename denied patches file", "error", err)
		os.Remove(tmp)
	}
}
