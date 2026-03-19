package signal

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// timeFormat is a fixed-width ISO-8601 format with milliseconds for consistent
// SQLite TEXT column ordering. Variable-length RFC3339Nano breaks lexicographic sort.
const timeFormat = "2006-01-02T15:04:05.000Z"

// EventStore persists and queries signal events in SQLite.
type EventStore struct {
	db *sql.DB
}

// NewEventStore wraps a database connection for event operations.
func NewEventStore(db *sql.DB) *EventStore {
	return &EventStore{db: db}
}

// Ingest stores a batch of raw events, skipping duplicates via the
// UNIQUE(source, source_item_id) constraint. Returns the slice of
// newly inserted events (deduped out events are excluded).
func (s *EventStore) Ingest(ctx context.Context, events []*RawEvent) ([]*RawEvent, error) {
	if len(events) == 0 {
		return nil, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("signal: begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO events (id, source, source_item_id, kind, title, body, url, actor, group_key, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source, source_item_id) DO NOTHING`)
	if err != nil {
		return nil, fmt.Errorf("signal: prepare insert: %w", err)
	}
	defer stmt.Close()

	var inserted []*RawEvent
	for _, ev := range events {
		id := generateEventID()
		meta, err := json.Marshal(ev.Metadata)
		if err != nil {
			return inserted, fmt.Errorf("signal: marshal metadata for %s/%s: %w", ev.Source, ev.SourceID, err)
		}
		if meta == nil {
			meta = []byte("{}")
		}

		createdAt := ev.OccurredAt
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}

		res, err := stmt.ExecContext(ctx, id, ev.Source, ev.SourceID, ev.Kind,
			ev.Title, ev.Body, ev.URL, ev.Actor, ev.GroupKey,
			string(meta), createdAt.UTC().Format(timeFormat))
		if err != nil {
			return inserted, fmt.Errorf("signal: insert event: %w", err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			inserted = append(inserted, ev)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("signal: commit: %w", err)
	}
	return inserted, nil
}

// buildWhere constructs a WHERE clause and args from an EventFilter.
func buildWhere(f EventFilter) (string, []any) {
	var clauses []string
	var args []any

	if f.Source != "" {
		clauses = append(clauses, "source = ?")
		args = append(args, f.Source)
	}
	if f.Kind != "" {
		clauses = append(clauses, "kind = ?")
		args = append(args, f.Kind)
	}
	if f.UnreadOnly {
		clauses = append(clauses, "read_at IS NULL")
	}
	if f.Before != "" {
		// Stable cursor: tie-break on id for events sharing the same timestamp.
		clauses = append(clauses, "(created_at, id) < ((SELECT created_at FROM events WHERE id = ?), ?)")
		args = append(args, f.Before, f.Before)
	}

	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

// List returns events matching the filter, ordered by created_at DESC.
func (s *EventStore) List(ctx context.Context, f EventFilter) ([]*StoredEvent, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}

	where, args := buildWhere(f)
	query := fmt.Sprintf(`SELECT id, source, source_item_id, kind, title, body, url, actor,
		COALESCE(group_key, ''), COALESCE(metadata, '{}'), created_at, read_at, archived_at
		FROM events %s ORDER BY created_at DESC, id DESC LIMIT ?`, where)
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("signal: list events: %w", err)
	}
	defer rows.Close()

	var result []*StoredEvent
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, ev)
	}
	return result, rows.Err()
}

// Count returns the total number of events matching the filter.
func (s *EventStore) Count(ctx context.Context, f EventFilter) (int, error) {
	where, args := buildWhere(f)
	var count int
	err := s.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM events %s", where), args...).Scan(&count)
	return count, err
}

// MarkRead marks an event as read.
func (s *EventStore) MarkRead(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(timeFormat)
	_, err := s.db.ExecContext(ctx, "UPDATE events SET read_at = ? WHERE id = ? AND read_at IS NULL", now, id)
	return err
}

// MarkAllRead marks all unread events as read.
func (s *EventStore) MarkAllRead(ctx context.Context) (int, error) {
	now := time.Now().UTC().Format(timeFormat)
	res, err := s.db.ExecContext(ctx, "UPDATE events SET read_at = ? WHERE read_at IS NULL", now)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// Archive sets archived_at on a single event.
func (s *EventStore) Archive(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(timeFormat)
	res, err := s.db.ExecContext(ctx, "UPDATE events SET archived_at = ? WHERE id = ? AND archived_at IS NULL", now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("signal: event %s not found or already archived", id)
	}
	return nil
}

// DeleteByID hard-deletes a single event by ID.
func (s *EventStore) DeleteByID(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM events WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("signal: event %s not found", id)
	}
	return nil
}

// Delete removes events older than the given duration.
func (s *EventStore) Delete(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := time.Now().UTC().Add(-olderThan).Format(timeFormat)
	res, err := s.db.ExecContext(ctx, "DELETE FROM events WHERE created_at < ?", cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func scanEvent(rows *sql.Rows) (*StoredEvent, error) {
	var ev StoredEvent
	var metaStr string
	var createdStr string
	var readStr, archivedStr sql.NullString

	err := rows.Scan(&ev.ID, &ev.Source, &ev.SourceItemID, &ev.Kind,
		&ev.Title, &ev.Body, &ev.URL, &ev.Actor,
		&ev.GroupKey, &metaStr, &createdStr, &readStr, &archivedStr)
	if err != nil {
		return nil, fmt.Errorf("signal: scan event: %w", err)
	}

	if metaStr != "" {
		if err := json.Unmarshal([]byte(metaStr), &ev.Metadata); err != nil {
			return nil, fmt.Errorf("signal: parse metadata: %w", err)
		}
	}
	if ev.Metadata == nil {
		ev.Metadata = map[string]any{}
	}

	var parseErr error
	ev.CreatedAt, parseErr = time.Parse(timeFormat, createdStr)
	if parseErr != nil {
		return nil, fmt.Errorf("signal: parse created_at %q: %w", createdStr, parseErr)
	}
	if readStr.Valid {
		t, err := time.Parse(timeFormat, readStr.String)
		if err != nil {
			return nil, fmt.Errorf("signal: parse read_at: %w", err)
		}
		ev.ReadAt = &t
	}
	if archivedStr.Valid {
		t, err := time.Parse(timeFormat, archivedStr.String)
		if err != nil {
			return nil, fmt.Errorf("signal: parse archived_at: %w", err)
		}
		ev.ArchivedAt = &t
	}

	return &ev, nil
}

func generateEventID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return fmt.Sprintf("ev_%x", b)
}
