package memory

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestMarkdownFile_Load(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	os.WriteFile(path, []byte("# Hello\nWorld"), 0644)

	mf := NewMarkdownFile(path)
	if err := mf.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := mf.Content(); got != "# Hello\nWorld" {
		t.Errorf("Content() = %q, want %q", got, "# Hello\nWorld")
	}
	if !mf.Exists() {
		t.Error("Exists() = false, want true")
	}
	if mf.FilePath() != path {
		t.Errorf("FilePath() = %q, want %q", mf.FilePath(), path)
	}
}

func TestMarkdownFile_LoadMissing(t *testing.T) {
	mf := NewMarkdownFile("/tmp/does-not-exist-cairn-test.md")
	if err := mf.Load(); err != nil {
		t.Fatalf("Load on missing file should return nil, got: %v", err)
	}
	if mf.Content() != "" {
		t.Errorf("Content() = %q, want empty", mf.Content())
	}
	if mf.Exists() {
		t.Error("Exists() = true, want false for missing file")
	}
}

func TestMarkdownFile_Save(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "save.md")

	mf := NewMarkdownFile(path)
	if err := mf.Save("# Saved"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify on disk.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "# Saved" {
		t.Errorf("disk content = %q, want %q", string(data), "# Saved")
	}

	// Verify in memory.
	if got := mf.Content(); got != "# Saved" {
		t.Errorf("Content() = %q, want %q", got, "# Saved")
	}
}

func TestMarkdownFile_Watch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watch.md")
	os.WriteFile(path, []byte("v1"), 0644)

	mf := NewMarkdownFile(path)
	if err := mf.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	changed := make(chan string, 1)
	mf.OnChange(func(content string) {
		changed <- content
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mf.Watch(ctx)

	// Wait a bit, then modify the file.
	time.Sleep(100 * time.Millisecond)
	// Ensure mod time changes (some filesystems have 1s granularity).
	time.Sleep(1100 * time.Millisecond)
	os.WriteFile(path, []byte("v2"), 0644)

	select {
	case got := <-changed:
		if got != "v2" {
			t.Errorf("onChange got %q, want %q", got, "v2")
		}
	case <-time.After(15 * time.Second):
		t.Error("timed out waiting for onChange callback")
	}
}

func TestMarkdownFile_WatchDetectsNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.md")

	mf := NewMarkdownFile(path)
	mf.Load() // file doesn't exist yet

	changed := make(chan string, 1)
	mf.OnChange(func(content string) {
		changed <- content
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mf.Watch(ctx)

	// Create the file after watch starts.
	time.Sleep(100 * time.Millisecond)
	os.WriteFile(path, []byte("new content"), 0644)

	select {
	case got := <-changed:
		if got != "new content" {
			t.Errorf("onChange got %q, want %q", got, "new content")
		}
	case <-time.After(15 * time.Second):
		t.Error("timed out waiting for new file detection")
	}
}

func TestMarkdownFile_ThreadSafe(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "thread.md")
	os.WriteFile(path, []byte("initial"), 0644)

	mf := NewMarkdownFile(path)
	mf.Load()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = mf.Content()
			_ = mf.Exists()
			_ = mf.FilePath()
		}()
	}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mf.Load()
		}()
	}
	wg.Wait()
}
