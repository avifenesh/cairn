package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DailyLogEntry represents a single entry in the daily log.
type DailyLogEntry struct {
	Time    time.Time
	Type    string // "task", "idle", "reflection", "cron"
	Summary string
}

// AppendDailyLog appends an entry to the daily log file at logDir/YYYY-MM-DD.md.
// Creates the directory and file if they do not exist.
func AppendDailyLog(logDir string, entry DailyLogEntry) error {
	if logDir == "" {
		return nil
	}

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("daily log: mkdir: %w", err)
	}

	date := entry.Time.Format("2006-01-02")
	path := filepath.Join(logDir, date+".md")

	// Check if file exists to determine whether to write a header.
	_, statErr := os.Stat(path)
	needsHeader := os.IsNotExist(statErr)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("daily log: open: %w", err)
	}
	defer f.Close()

	if needsHeader {
		fmt.Fprintf(f, "# Daily Log: %s\n\n", date)
	}

	timeStr := entry.Time.Format("15:04:05")
	fmt.Fprintf(f, "- **%s** [%s] %s\n", timeStr, entry.Type, entry.Summary)

	return nil
}
