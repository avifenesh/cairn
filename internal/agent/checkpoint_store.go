package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/avifenesh/cairn/internal/db"
	"github.com/avifenesh/cairn/internal/tool"
)

// SessionCheckpoint persists execution state for crash recovery.
// The actual message content is in the messages table - this tracks
// metadata not derivable from events alone.
type SessionCheckpoint struct {
	SessionID   string         `json:"sessionId"`
	TaskID      string         `json:"taskId,omitempty"`
	Round       int            `json:"round"`
	Mode        tool.Mode      `json:"mode"`
	MaxRounds   int            `json:"maxRounds"`
	UserMessage string         `json:"userMessage"`
	Origin      string         `json:"origin"` // "chat", "task", "subagent"
	State       map[string]any `json:"state,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

// CheckpointStore persists session checkpoints to SQLite.
type CheckpointStore struct {
	db *db.DB
}

// NewCheckpointStore creates a checkpoint store backed by the given database.
func NewCheckpointStore(d *db.DB) *CheckpointStore {
	return &CheckpointStore{db: d}
}

// Save upserts a checkpoint for the given session.
func (s *CheckpointStore) Save(ctx context.Context, cp *SessionCheckpoint) error {
	now := isoTime(time.Now())
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now()
	}
	cp.UpdatedAt = time.Now()

	stateJSON, err := json.Marshal(cp.State)
	if err != nil {
		return fmt.Errorf("checkpoint store: marshal state for %s: %w", cp.SessionID, err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO session_checkpoints (session_id, task_id, round, mode, max_rounds, user_message, origin, state, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			task_id = excluded.task_id,
			round = excluded.round,
			mode = excluded.mode,
			max_rounds = excluded.max_rounds,
			user_message = excluded.user_message,
			origin = excluded.origin,
			state = excluded.state,
			updated_at = excluded.updated_at`,
		cp.SessionID, cp.TaskID, cp.Round, string(cp.Mode), cp.MaxRounds,
		cp.UserMessage, cp.Origin, string(stateJSON),
		isoTime(cp.CreatedAt), now,
	)
	if err != nil {
		return fmt.Errorf("checkpoint store: save %s: %w", cp.SessionID, err)
	}
	return nil
}

// Load retrieves a checkpoint for a session. Returns an error wrapping
// sql.ErrNoRows if no checkpoint exists for the session.
func (s *CheckpointStore) Load(ctx context.Context, sessionID string) (*SessionCheckpoint, error) {
	var taskID, mode, userMessage, origin, stateStr, createdAt, updatedAt string
	var round, maxRounds int

	err := s.db.QueryRowContext(ctx, `
		SELECT task_id, round, mode, max_rounds, user_message, origin, state, created_at, updated_at
		FROM session_checkpoints WHERE session_id = ?`, sessionID).Scan(
		&taskID, &round, &mode, &maxRounds, &userMessage, &origin, &stateStr, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("checkpoint store: load %s: %w", sessionID, err)
	}

	cp := &SessionCheckpoint{
		SessionID:   sessionID,
		TaskID:      taskID,
		Round:       round,
		Mode:        tool.Mode(mode),
		MaxRounds:   maxRounds,
		UserMessage: userMessage,
		Origin:      origin,
		State:       make(map[string]any),
		CreatedAt:   parseTime(createdAt),
		UpdatedAt:   parseTime(updatedAt),
	}
	if stateStr != "" {
		if err := json.Unmarshal([]byte(stateStr), &cp.State); err != nil {
			return nil, fmt.Errorf("checkpoint store: unmarshal state for %s: %w", sessionID, err)
		}
	}
	return cp, nil
}

// Delete removes a checkpoint (called when session completes successfully).
func (s *CheckpointStore) Delete(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM session_checkpoints WHERE session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("checkpoint store: delete %s: %w", sessionID, err)
	}
	return nil
}

// ListIncomplete returns all checkpoints (sessions interrupted mid-execution).
func (s *CheckpointStore) ListIncomplete(ctx context.Context) ([]*SessionCheckpoint, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT session_id, task_id, round, mode, max_rounds, user_message, origin, state, created_at, updated_at
		FROM session_checkpoints ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("checkpoint store: list: %w", err)
	}
	defer rows.Close()

	var results []*SessionCheckpoint
	for rows.Next() {
		var sid, taskID, mode, userMessage, origin, stateStr, createdAt, updatedAt string
		var round, maxRounds int
		if err := rows.Scan(&sid, &taskID, &round, &mode, &maxRounds, &userMessage, &origin, &stateStr, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("checkpoint store: scan: %w", err)
		}
		cp := &SessionCheckpoint{
			SessionID:   sid,
			TaskID:      taskID,
			Round:       round,
			Mode:        tool.Mode(mode),
			MaxRounds:   maxRounds,
			UserMessage: userMessage,
			Origin:      origin,
			State:       make(map[string]any),
			CreatedAt:   parseTime(createdAt),
			UpdatedAt:   parseTime(updatedAt),
		}
		if stateStr != "" {
			if err := json.Unmarshal([]byte(stateStr), &cp.State); err != nil {
				return nil, fmt.Errorf("checkpoint store: unmarshal state for %s: %w", sid, err)
			}
		}
		results = append(results, cp)
	}
	return results, rows.Err()
}
