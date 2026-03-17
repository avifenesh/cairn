package memory

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSoul_Load(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SOUL.md")

	content := "# Soul\n\nI am a helpful assistant.\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	soul := NewSoul(path)
	if err := soul.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := soul.Content()
	if got != content {
		t.Errorf("content: got %q, want %q", got, content)
	}
}

func TestSoul_LoadMissingFile(t *testing.T) {
	soul := NewSoul("/nonexistent/SOUL.md")
	if err := soul.Load(); err == nil {
		t.Fatal("expected error loading missing file")
	}
}

func TestSoul_Watch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SOUL.md")

	initial := "initial content"
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	soul := NewSoul(path)
	if err := soul.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Set up onChange callback.
	changed := make(chan string, 1)
	soul.OnChange(func(content string) {
		select {
		case changed <- content:
		default:
		}
	})

	// Start watching.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go soul.Watch(ctx)

	// Wait a moment, then modify the file.
	// We need the mod time to differ, so ensure at least 1s gap on
	// filesystems with second-level timestamp resolution.
	time.Sleep(100 * time.Millisecond)

	updated := "updated content"
	// Touch the file with a future mod time to guarantee detection.
	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		t.Fatalf("WriteFile update: %v", err)
	}
	// Force a different mod time.
	future := time.Now().Add(10 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	// Wait for the callback to fire (Watch polls every 5s, but we can
	// trigger checkReload manually for faster tests).
	soul.checkReload()

	select {
	case got := <-changed:
		if got != updated {
			t.Errorf("onChange content: got %q, want %q", got, updated)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for onChange callback")
	}

	// Verify Content() returns updated value.
	if got := soul.Content(); got != updated {
		t.Errorf("Content() after reload: got %q, want %q", got, updated)
	}
}

func TestSoul_ThreadSafe(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SOUL.md")

	if err := os.WriteFile(path, []byte("initial"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	soul := NewSoul(path)
	if err := soul.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Concurrent reads during reloads.
	var wg sync.WaitGroup
	var readCount atomic.Int64

	const goroutines = 50
	const iterations = 100

	// Half the goroutines read, half reload.
	wg.Add(goroutines)
	for i := 0; i < goroutines/2; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				content := soul.Content()
				if content == "" {
					t.Error("Content() returned empty during concurrent access")
					return
				}
				readCount.Add(1)
			}
		}()
	}

	for i := 0; i < goroutines/2; i++ {
		go func(n int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// Reload the file content (Load is safe to call concurrently).
				_ = soul.Load()
			}
		}(i)
	}

	wg.Wait()

	if readCount.Load() == 0 {
		t.Error("no reads completed")
	}
}
