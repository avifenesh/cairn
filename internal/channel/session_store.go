package channel

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"time"

	"github.com/avifenesh/cairn/internal/tool/builtin"
)

const sessionTimeFormat = "2006-01-02T15:04:05.000Z"

// SessionStore maps channel conversations to Cairn sessions.
type SessionStore struct {
	db *sql.DB
}

// NewSessionStore creates a channel session store.
func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

// GetOrCreate looks up the active session for a channel+chatID.
// If no session exists or the existing one has timed out, creates a new one.
// Returns the session ID and whether it was newly created.
func (s *SessionStore) GetOrCreate(ctx context.Context, channel, chatID string, timeout time.Duration) (string, bool, error) {
	var sessionID, updatedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT session_id, updated_at FROM channel_sessions
		WHERE channel = ? AND chat_id = ?`,
		channel, chatID,
	).Scan(&sessionID, &updatedAt)

	if err == nil {
		// Check timeout.
		if t, parseErr := time.Parse(sessionTimeFormat, updatedAt); parseErr == nil {
			if time.Since(t) < timeout {
				// Session still active — touch updated_at.
				now := time.Now().UTC().Format(sessionTimeFormat)
				if _, err := s.db.ExecContext(ctx, `
					UPDATE channel_sessions SET updated_at = ? WHERE channel = ? AND chat_id = ?`,
					now, channel, chatID); err != nil {
					return sessionID, false, fmt.Errorf("channel session touch: %w", err)
				}
				return sessionID, false, nil
			}
		}
		// Timed out — clean up old session's file tracking state, create new.
		builtin.CleanupSessionFiles(sessionID)
		newID := generateSessionID()
		now := time.Now().UTC().Format(sessionTimeFormat)
		_, err = s.db.ExecContext(ctx, `
			UPDATE channel_sessions SET session_id = ?, updated_at = ? WHERE channel = ? AND chat_id = ?`,
			newID, now, channel, chatID)
		return newID, true, err
	}

	if err != sql.ErrNoRows {
		return "", false, fmt.Errorf("channel session lookup: %w", err)
	}

	// No mapping exists — create new.
	newID := generateSessionID()
	now := time.Now().UTC().Format(sessionTimeFormat)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO channel_sessions (channel, chat_id, session_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`,
		channel, chatID, newID, now, now)
	if err != nil {
		return "", false, fmt.Errorf("channel session create: %w", err)
	}

	return newID, true, nil
}

// Reset deletes the session mapping for a channel+chatID, forcing a new session
// on the next message. Used by the /new command.
func (s *SessionStore) Reset(ctx context.Context, channel, chatID string) error {
	// Clean up file tracking state for the old session before deleting.
	var oldSessionID string
	if err := s.db.QueryRowContext(ctx, `
		SELECT session_id FROM channel_sessions WHERE channel = ? AND chat_id = ?`,
		channel, chatID).Scan(&oldSessionID); err == nil {
		builtin.CleanupSessionFiles(oldSessionID)
	}
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM channel_sessions WHERE channel = ? AND chat_id = ?`,
		channel, chatID)
	return err
}

func generateSessionID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return fmt.Sprintf("ch_%x", b)
}
