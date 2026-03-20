package agent

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

const activityTimeFormat = "2006-01-02T15:04:05.000Z"

// ActivityEntry represents a single agent activity log entry.
type ActivityEntry struct {
	ID         string   `json:"id"`
	Type       string   `json:"type"`    // task, idle, reflection, cron, error
	Summary    string   `json:"summary"` // concise LLM-generated header
	Details    string   `json:"details"` // full expandable content
	Errors     []string `json:"errors"`  // error messages
	ToolCount  int      `json:"toolCount"`
	DurationMs int64    `json:"durationMs"`
	CreatedAt  string   `json:"createdAt"`
}

// ToolStatsEntry represents per-tool execution stats.
type ToolStatsEntry struct {
	ToolName  string `json:"toolName"`
	Calls     int    `json:"calls"`
	Errors    int    `json:"errors"`
	TotalMs   int64  `json:"totalMs"`
	LastError string `json:"lastError,omitempty"`
}

// ToolStatsOverview is the aggregate tool stats response.
type ToolStatsOverview struct {
	TotalCalls   int              `json:"totalCalls"`
	TotalErrors  int              `json:"totalErrors"`
	ByTool       map[string]int   `json:"byTool"`
	ErrorsByTool map[string]int   `json:"errorsByTool"`
	Tools        []ToolStatsEntry `json:"tools"`
}

// ActivityStore persists and queries agent activity.
type ActivityStore struct {
	db *sql.DB
}

// NewActivityStore wraps a database connection.
func NewActivityStore(db *sql.DB) *ActivityStore {
	return &ActivityStore{db: db}
}

// Record persists a new activity entry.
func (s *ActivityStore) Record(ctx context.Context, entry ActivityEntry) error {
	if entry.ID == "" {
		entry.ID = generateActivityID()
	}
	if entry.CreatedAt == "" {
		entry.CreatedAt = time.Now().UTC().Format(activityTimeFormat)
	}
	errJSON, _ := json.Marshal(entry.Errors)
	if errJSON == nil {
		errJSON = []byte("[]")
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_activity (id, type, summary, details, errors, tool_count, duration_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.Type, entry.Summary, entry.Details, string(errJSON),
		entry.ToolCount, entry.DurationMs, entry.CreatedAt)
	return err
}

// List returns recent activity entries, newest first.
func (s *ActivityStore) List(ctx context.Context, limit, offset int, activityType string) ([]ActivityEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	query := "SELECT id, type, summary, details, errors, tool_count, duration_ms, created_at FROM agent_activity"
	var args []any
	if activityType != "" && activityType != "all" {
		query += " WHERE type = ?"
		args = append(args, activityType)
	}
	query += " ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []ActivityEntry
	for rows.Next() {
		var e ActivityEntry
		var errStr string
		if err := rows.Scan(&e.ID, &e.Type, &e.Summary, &e.Details, &errStr, &e.ToolCount, &e.DurationMs, &e.CreatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(errStr), &e.Errors)
		if e.Errors == nil {
			e.Errors = []string{}
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// RecordToolCall upserts tool execution stats.
func (s *ActivityStore) RecordToolCall(ctx context.Context, toolName string, durationMs int64, errMsg string) error {
	now := time.Now().UTC().Format(activityTimeFormat)
	isErr := 0
	if errMsg != "" {
		isErr = 1
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tool_stats (tool_name, calls, errors, total_ms, last_error, updated_at)
		VALUES (?, 1, ?, ?, ?, ?)
		ON CONFLICT(tool_name) DO UPDATE SET
			calls = calls + 1,
			errors = errors + ?,
			total_ms = total_ms + ?,
			last_error = CASE WHEN ? != '' THEN ? ELSE last_error END,
			updated_at = ?`,
		toolName, isErr, durationMs, errMsg, now,
		isErr, durationMs, errMsg, errMsg, now)
	return err
}

// GetToolStats returns aggregate tool execution statistics.
func (s *ActivityStore) GetToolStats(ctx context.Context) (*ToolStatsOverview, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT tool_name, calls, errors, total_ms, COALESCE(last_error, '') FROM tool_stats ORDER BY calls DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	overview := &ToolStatsOverview{
		ByTool:       map[string]int{},
		ErrorsByTool: map[string]int{},
	}
	for rows.Next() {
		var e ToolStatsEntry
		if err := rows.Scan(&e.ToolName, &e.Calls, &e.Errors, &e.TotalMs, &e.LastError); err != nil {
			return nil, err
		}
		overview.TotalCalls += e.Calls
		overview.TotalErrors += e.Errors
		overview.ByTool[e.ToolName] = e.Calls
		if e.Errors > 0 {
			overview.ErrorsByTool[e.ToolName] = e.Errors
		}
		overview.Tools = append(overview.Tools, e)
	}
	return overview, rows.Err()
}

func generateActivityID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return fmt.Sprintf("act_%x", b)
}
