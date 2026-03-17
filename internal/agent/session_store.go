package agent

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/avifenesh/cairn/internal/db"
	"github.com/avifenesh/cairn/internal/tool"
)

// SessionStore persists sessions and messages to SQLite.
type SessionStore struct {
	db *db.DB
}

// NewSessionStore creates a session store backed by the given database.
func NewSessionStore(d *db.DB) *SessionStore {
	return &SessionStore{db: d}
}

// Create persists a new session.
func (s *SessionStore) Create(ctx context.Context, session *Session) error {
	if session.ID == "" {
		session.ID = newID()
	}
	now := isoTime(time.Now())
	session.CreatedAt = time.Now()
	session.UpdatedAt = session.CreatedAt

	stateJSON, _ := json.Marshal(session.State)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (id, created_at, updated_at, title, mode, metadata)
		VALUES (?, ?, ?, ?, ?, ?)`,
		session.ID, now, now, session.Title, string(session.Mode), string(stateJSON),
	)
	if err != nil {
		return fmt.Errorf("session store: create %s: %w", session.ID, err)
	}
	return nil
}

// Get retrieves a session by ID, including its message history as Events.
func (s *SessionStore) Get(ctx context.Context, id string) (*Session, error) {
	var title, mode, createdAt, updatedAt string
	var metadata sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT id, title, mode, created_at, updated_at, metadata
		FROM sessions WHERE id = ?`, id).Scan(
		&id, &title, &mode, &createdAt, &updatedAt, &metadata,
	)
	if err != nil {
		return nil, fmt.Errorf("session store: get %s: %w", id, err)
	}

	session := &Session{
		ID:        id,
		Title:     title,
		Mode:      tool.Mode(mode),
		CreatedAt: parseTime(createdAt),
		UpdatedAt: parseTime(updatedAt),
		State:     make(map[string]any),
	}

	if metadata.Valid && metadata.String != "" {
		json.Unmarshal([]byte(metadata.String), &session.State)
	}

	// Load messages as events.
	events, err := s.loadEvents(ctx, id)
	if err != nil {
		return nil, err
	}
	session.Events = events

	return session, nil
}

// List returns sessions ordered by most recent first.
func (s *SessionStore) List(ctx context.Context, limit int) ([]*Session, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, mode, created_at, updated_at
		FROM sessions ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("session store: list: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		var id, title, mode, createdAt, updatedAt string
		if err := rows.Scan(&id, &title, &mode, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("session store: scan list: %w", err)
		}
		sessions = append(sessions, &Session{
			ID:        id,
			Title:     title,
			Mode:      tool.Mode(mode),
			CreatedAt: parseTime(createdAt),
			UpdatedAt: parseTime(updatedAt),
		})
	}
	return sessions, rows.Err()
}

// AppendEvent persists an event as a message in the session.
func (s *SessionStore) AppendEvent(ctx context.Context, sessionID string, ev *Event) error {
	if ev.ID == "" {
		ev.ID = newID()
	}
	ev.SessionID = sessionID
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}

	// Determine role from author.
	role := "assistant"
	if ev.Author == "user" {
		role = "user"
	}

	// Serialize parts as content text + tool data.
	content, toolCalls, toolResults := serializeParts(ev.Parts)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO messages (id, session_id, role, content, tool_calls, tool_results, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ev.ID, sessionID, role, content,
		nullJSON(toolCalls), nullJSON(toolResults),
		isoTime(ev.Timestamp),
	)
	if err != nil {
		return fmt.Errorf("session store: append event %s: %w", ev.ID, err)
	}

	// Update session timestamp.
	s.db.ExecContext(ctx, `UPDATE sessions SET updated_at = ? WHERE id = ?`,
		isoTime(time.Now()), sessionID)

	return nil
}

// Delete removes a session and its messages.
func (s *SessionStore) Delete(ctx context.Context, id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("session store: delete begin: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM messages WHERE session_id = ?", id); err != nil {
		tx.Rollback()
		return fmt.Errorf("session store: delete messages for %s: %w", id, err)
	}
	if _, err := tx.Exec("DELETE FROM sessions WHERE id = ?", id); err != nil {
		tx.Rollback()
		return fmt.Errorf("session store: delete session %s: %w", id, err)
	}
	return tx.Commit()
}

// loadEvents reads messages for a session and converts them to Events.
func (s *SessionStore) loadEvents(ctx context.Context, sessionID string) ([]*Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, role, content, tool_calls, tool_results, created_at
		FROM messages WHERE session_id = ? ORDER BY created_at ASC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session store: load events for %s: %w", sessionID, err)
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		var id, role, content, createdAt string
		var toolCalls, toolResults sql.NullString

		if err := rows.Scan(&id, &role, &content, &toolCalls, &toolResults, &createdAt); err != nil {
			return nil, fmt.Errorf("session store: scan event: %w", err)
		}

		author := role
		if role == "assistant" {
			author = "agent"
		}

		ev := &Event{
			ID:        id,
			SessionID: sessionID,
			Timestamp: parseTime(createdAt),
			Author:    author,
		}

		// Reconstruct parts from stored data.
		if content != "" {
			ev.Parts = append(ev.Parts, TextPart{Text: content})
		}
		if toolCalls.Valid && toolCalls.String != "" {
			var tcs []ToolPart
			if err := json.Unmarshal([]byte(toolCalls.String), &tcs); err != nil {
				slog.Warn("session store: failed to unmarshal tool_calls", "event", id, "error", err)
			} else {
				for _, tc := range tcs {
					ev.Parts = append(ev.Parts, tc)
				}
			}
		}
		if toolResults.Valid && toolResults.String != "" {
			var trs []ToolPart
			if err := json.Unmarshal([]byte(toolResults.String), &trs); err != nil {
				slog.Warn("session store: failed to unmarshal tool_results", "event", id, "error", err)
			} else {
				for _, tr := range trs {
					ev.Parts = append(ev.Parts, tr)
				}
			}
		}

		events = append(events, ev)
	}
	return events, rows.Err()
}

// serializeParts extracts text content and tool call/result JSON from parts.
func serializeParts(parts []Part) (content string, toolCalls, toolResults []byte) {
	var texts []string
	var calls []ToolPart
	var results []ToolPart

	for _, p := range parts {
		switch v := p.(type) {
		case TextPart:
			texts = append(texts, v.Text)
		case ReasoningPart:
			// Reasoning is stored as part of content for simplicity.
			texts = append(texts, v.Text)
		case ToolPart:
			if v.Status == ToolPending || v.Status == ToolRunning {
				calls = append(calls, v)
			} else {
				results = append(results, v)
			}
		}
	}

	for i, t := range texts {
		if i > 0 {
			content += "\n"
		}
		content += t
	}

	if len(calls) > 0 {
		var err error
		toolCalls, err = json.Marshal(calls)
		if err != nil {
			slog.Warn("session store: failed to marshal tool calls", "error", err)
		}
	}
	if len(results) > 0 {
		var err error
		toolResults, err = json.Marshal(results)
		if err != nil {
			slog.Warn("session store: failed to marshal tool results", "error", err)
		}
	}
	return
}

func nullJSON(data []byte) sql.NullString {
	if data == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: string(data), Valid: true}
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func isoTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

func parseTime(s string) time.Time {
	for _, layout := range []string{
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
