package signal

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// SourceState tracks per-source checkpoint data (last poll time, cursors, etc.)
// using the existing source_state table.
type SourceState struct {
	db *sql.DB
}

// NewSourceState wraps a database connection for source state operations.
func NewSourceState(db *sql.DB) *SourceState {
	return &SourceState{db: db}
}

// stateData is the JSON structure stored in the value column.
type stateData struct {
	LastPoll time.Time      `json:"lastPoll"`
	Cursor   string         `json:"cursor,omitempty"`
	Extra    map[string]any `json:"extra,omitempty"`
}

// GetLastPoll returns the last successful poll time for a source.
// Returns zero time if the source has never been polled.
func (s *SourceState) GetLastPoll(ctx context.Context, source string) (time.Time, error) {
	key := "signal:" + source
	var valueStr string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM source_state WHERE key = ?", key).Scan(&valueStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("signal: get state %q: %w", source, err)
	}

	var data stateData
	if err := json.Unmarshal([]byte(valueStr), &data); err != nil {
		return time.Time{}, fmt.Errorf("signal: parse poll state %q: %w", source, err)
	}
	return data.LastPoll, nil
}

// SetLastPoll records the last successful poll time for a source,
// preserving any existing cursor/extra data.
func (s *SourceState) SetLastPoll(ctx context.Context, source string, t time.Time) error {
	key := "signal:" + source

	// Read existing state to preserve cursor.
	existing := s.readState(ctx, key)
	existing.LastPoll = t.UTC()

	value, err := json.Marshal(existing)
	if err != nil {
		return fmt.Errorf("signal: marshal poll state %q: %w", source, err)
	}
	now := time.Now().UTC().Format(timeFormat)

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO source_state (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, string(value), now)
	return err
}

// readState loads existing stateData for a key, returning zero value if not found.
func (s *SourceState) readState(ctx context.Context, key string) stateData {
	var valueStr string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM source_state WHERE key = ?", key).Scan(&valueStr)
	if err != nil {
		return stateData{}
	}
	var data stateData
	json.Unmarshal([]byte(valueStr), &data)
	return data
}

// GetCursor returns the cursor (e.g. page token, since ID) for a source.
func (s *SourceState) GetCursor(ctx context.Context, source string) (string, error) {
	key := "signal:" + source
	var valueStr string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM source_state WHERE key = ?", key).Scan(&valueStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("signal: get cursor %q: %w", source, err)
	}

	var data stateData
	if err := json.Unmarshal([]byte(valueStr), &data); err != nil {
		return "", fmt.Errorf("signal: parse cursor state %q: %w", source, err)
	}
	return data.Cursor, nil
}

// GetExtra returns the extra metadata for a source.
func (s *SourceState) GetExtra(ctx context.Context, source string) (map[string]any, error) {
	key := "signal:" + source
	data := s.readState(ctx, key)
	if data.Extra == nil {
		return map[string]any{}, nil
	}
	return data.Extra, nil
}

// SetExtra updates the extra metadata for a source, preserving LastPoll/Cursor.
func (s *SourceState) SetExtra(ctx context.Context, source string, extra map[string]any) error {
	key := "signal:" + source
	existing := s.readState(ctx, key)
	existing.Extra = extra

	value, err := json.Marshal(existing)
	if err != nil {
		return fmt.Errorf("signal: marshal extra %q: %w", source, err)
	}
	now := time.Now().UTC().Format(timeFormat)

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO source_state (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, string(value), now)
	return err
}

// SetCursorAndPoll records both cursor and last poll time atomically.
func (s *SourceState) SetCursorAndPoll(ctx context.Context, source, cursor string, t time.Time) error {
	key := "signal:" + source
	data := stateData{LastPoll: t.UTC(), Cursor: cursor}
	value, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("signal: marshal cursor state %q: %w", source, err)
	}
	now := time.Now().UTC().Format(timeFormat)

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO source_state (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, string(value), now)
	return err
}
