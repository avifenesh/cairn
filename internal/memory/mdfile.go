package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// PendingPatch represents a proposed change to a markdown file awaiting human review.
type PendingPatch struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"` // the content to append
	Source    string    `json:"source"`  // who proposed it (e.g. "orchestrator", "reflection")
	CreatedAt time.Time `json:"createdAt"`
	Preview   string    `json:"preview"` // full file content after applying the patch
}

// MarkdownFile loads, watches, and atomically saves a single markdown file.
// Thread-safe for concurrent reads. Used as the building block for
// UserProfile, AgentsFile, and CuratedMemory.
type MarkdownFile struct {
	mu       sync.RWMutex
	content  string
	filePath string
	modTime  time.Time
	onChange func(content string)
	pending  *PendingPatch
}

// NewMarkdownFile creates a MarkdownFile bound to the given path.
// Call Load() to read the initial content, then Watch() to auto-reload.
func NewMarkdownFile(filePath string) *MarkdownFile {
	return &MarkdownFile{filePath: filePath}
}

// Load reads the file into memory. Returns nil if the file does not exist.
// Safe to call multiple times.
func (m *MarkdownFile) Load() error {
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			m.mu.Lock()
			m.content = ""
			m.modTime = time.Time{}
			m.mu.Unlock()
			return nil
		}
		return err
	}

	info, err := os.Stat(m.filePath)
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.content = string(data)
	m.modTime = info.ModTime()
	m.mu.Unlock()

	return nil
}

// Content returns the current file content (thread-safe).
func (m *MarkdownFile) Content() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.content
}

// Exists returns true if the file has been loaded and has non-empty content.
// Intentionally returns false for empty files on disk - an empty markdown file
// is treated as "not configured" since it carries no useful content.
func (m *MarkdownFile) Exists() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.content != ""
}

// FilePath returns the path to the file on disk.
func (m *MarkdownFile) FilePath() string {
	return m.filePath
}

// OnChange registers a callback invoked after each successful reload.
func (m *MarkdownFile) OnChange(fn func(string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChange = fn
}

// Watch polls the file for modifications every 5 seconds and reloads on
// change. It also detects newly created files. Blocks until ctx is
// cancelled. Typically run as a goroutine.
func (m *MarkdownFile) Watch(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			m.checkReload()
		}
	}
}

// Save atomically writes content to the file using write-rename.
func (m *MarkdownFile) Save(content string) error {
	dir := filepath.Dir(m.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Use a unique temp file in the same directory for concurrent safety.
	tmp, err := os.CreateTemp(dir, filepath.Base(m.filePath)+".tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write([]byte(content)); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, m.filePath); err != nil {
		os.Remove(tmpName)
		return err
	}

	// Update in-memory state.
	info, _ := os.Stat(m.filePath)
	m.mu.Lock()
	m.content = content
	if info != nil {
		m.modTime = info.ModTime()
	}
	m.mu.Unlock()

	return nil
}

// checkReload stats the file and reloads if the modification time changed
// or if a previously missing file now exists.
func (m *MarkdownFile) checkReload() {
	info, err := os.Stat(m.filePath)
	if err != nil {
		return // file still missing or inaccessible
	}

	m.mu.RLock()
	changed := !info.ModTime().Equal(m.modTime)
	m.mu.RUnlock()

	if !changed {
		return
	}

	if err := m.Load(); err != nil {
		return
	}

	m.mu.RLock()
	fn := m.onChange
	content := m.content
	m.mu.RUnlock()

	if fn != nil {
		fn(content)
	}
}

// ProposePatch creates a pending patch for human review.
// Only one patch can be pending at a time; new proposals replace old ones.
// The patch is persisted to .<basename>_patch.json alongside the file.
func (m *MarkdownFile) ProposePatch(content, source string) *PendingPatch {
	m.mu.Lock()
	defer m.mu.Unlock()

	sep := "\n"
	if m.content == "" {
		sep = ""
	} else if !strings.HasSuffix(m.content, "\n") {
		sep = "\n\n"
	}
	preview := m.content + sep + content

	patch := &PendingPatch{
		ID:        fmt.Sprintf("mp_%d", time.Now().UnixMilli()),
		Content:   content,
		Source:    source,
		CreatedAt: time.Now(),
		Preview:   preview,
	}
	m.pending = patch
	m.persistPatch()
	slog.Info("mdfile: patch proposed", "file", filepath.Base(m.filePath), "id", patch.ID, "source", source)
	return patch
}

// PendingPatch returns the current pending patch, or nil if none.
func (m *MarkdownFile) PendingPatch() *PendingPatch {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pending
}

// ApprovePatch applies the pending patch and clears it.
func (m *MarkdownFile) ApprovePatch(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.pending == nil {
		return fmt.Errorf("no pending patch")
	}
	if m.pending.ID != id {
		return fmt.Errorf("no pending patch with id %q", id)
	}

	// Rebase preview onto current content (file may have changed since proposal).
	sep := "\n"
	if m.content == "" {
		sep = ""
	} else if !strings.HasSuffix(m.content, "\n") {
		sep = "\n\n"
	}
	newContent := m.content + sep + m.pending.Content

	// Unlock before Save (which takes its own lock), then re-lock.
	m.mu.Unlock()
	if err := m.Save(newContent); err != nil {
		m.mu.Lock()
		return fmt.Errorf("write patch: %w", err)
	}
	m.mu.Lock()

	m.pending = nil
	m.clearPatchFile()
	slog.Info("mdfile: patch approved", "file", filepath.Base(m.filePath), "id", id)
	return nil
}

// DenyPatch discards the pending patch.
func (m *MarkdownFile) DenyPatch(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.pending == nil {
		return fmt.Errorf("no pending patch")
	}
	if m.pending.ID != id {
		return fmt.Errorf("no pending patch with id %q", id)
	}

	m.pending = nil
	m.clearPatchFile()
	slog.Info("mdfile: patch denied", "file", filepath.Base(m.filePath), "id", id)
	return nil
}

// patchFilePath returns the path to the patch persistence file.
func (m *MarkdownFile) patchFilePath() string {
	base := strings.TrimSuffix(filepath.Base(m.filePath), filepath.Ext(m.filePath))
	return filepath.Join(filepath.Dir(m.filePath), "."+base+"_patch.json")
}

func (m *MarkdownFile) persistPatch() {
	if m.pending == nil {
		m.clearPatchFile()
		return
	}
	data, err := json.Marshal(m.pending)
	if err != nil {
		slog.Warn("mdfile: marshal pending patch", "file", filepath.Base(m.filePath), "err", err)
		return
	}
	if err := os.WriteFile(m.patchFilePath(), data, 0644); err != nil {
		slog.Warn("mdfile: persist patch file", "file", filepath.Base(m.filePath), "err", err)
	}
}

func (m *MarkdownFile) clearPatchFile() {
	os.Remove(m.patchFilePath())
}

// LoadPendingPatch restores a pending patch from disk (called once at startup).
func (m *MarkdownFile) LoadPendingPatch() {
	data, err := os.ReadFile(m.patchFilePath())
	if err != nil {
		return
	}
	var patch PendingPatch
	if err := json.Unmarshal(data, &patch); err != nil {
		return
	}
	m.mu.Lock()
	// Rebase preview onto current content (file may have changed since proposal).
	sep := "\n"
	if m.content == "" {
		sep = ""
	} else if !strings.HasSuffix(m.content, "\n") {
		sep = "\n\n"
	}
	patch.Preview = m.content + sep + patch.Content
	m.pending = &patch
	m.mu.Unlock()
}
