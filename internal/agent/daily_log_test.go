package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendDailyLog_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")

	now := time.Date(2026, 3, 22, 14, 30, 0, 0, time.UTC)
	entry := DailyLogEntry{
		Time:    now,
		Type:    "task",
		Summary: "Completed onboarding",
	}

	if err := AppendDailyLog(logDir, entry); err != nil {
		t.Fatalf("AppendDailyLog: %v", err)
	}

	path := filepath.Join(logDir, "2026-03-22.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "# Daily Log: 2026-03-22") {
		t.Error("missing header")
	}
	if !strings.Contains(content, "Completed onboarding") {
		t.Error("missing entry")
	}
}

func TestAppendDailyLog_AppendsToExisting(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC)

	AppendDailyLog(dir, DailyLogEntry{Time: now, Type: "task", Summary: "First"})
	AppendDailyLog(dir, DailyLogEntry{Time: now.Add(time.Hour), Type: "idle", Summary: "Second"})

	data, _ := os.ReadFile(filepath.Join(dir, "2026-03-22.md"))
	content := string(data)

	if strings.Count(content, "# Daily Log") != 1 {
		t.Error("header should appear exactly once")
	}
	if !strings.Contains(content, "First") || !strings.Contains(content, "Second") {
		t.Error("missing entries")
	}
}

func TestAppendDailyLog_EmptyDir(t *testing.T) {
	err := AppendDailyLog("", DailyLogEntry{Time: time.Now(), Type: "test", Summary: "noop"})
	if err != nil {
		t.Errorf("empty dir should be a no-op, got: %v", err)
	}
}
