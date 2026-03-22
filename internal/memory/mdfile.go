package memory

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MarkdownFile loads, watches, and atomically saves a single markdown file.
// Thread-safe for concurrent reads. Used as the building block for
// UserProfile, AgentsFile, and CuratedMemory.
type MarkdownFile struct {
	mu       sync.RWMutex
	content  string
	filePath string
	modTime  time.Time
	onChange func(content string)
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
