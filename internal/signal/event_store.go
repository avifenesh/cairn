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

// EventStore persists and queries signal events in SQLite.
type EventStore struct {
	db *sql.DB
}

// NewEventStore wraps a database connection for event operations.
func NewEventStore(db *sql.DB) *EventStore {
	return &EventStore{db: db}
}

// Ingest stores a batch of raw events, skipping duplicates via the
// UNIQUE(source, source_item_id) constraint. Returns the count of
// newly inserted events.
func (s *EventStore) Ingest(ctx context.Context, events []*RawEvent) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("signal: begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO events (id, source, source_item_id, kind, title, body, url, actor, group_key, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source, source_item_id) DO NOTHING`)
	if err != nil {
		return 0, fmt.Errorf("signal: prepare insert: %w", err)
	}
	defer stmt.Close()

	inserted := 0
	for _, ev := range events {
		id := generateEventID()
		meta, _ := json.Marshal(ev.Metadata)
		if meta == nil {
			meta = []byte("{}")
		}

		createdAt := ev.OccurredAt
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}

		res, err := stmt.ExecContext(ctx, id, ev.Source, ev.SourceID, ev.Kind,
			ev.Title, ev.Body, ev.URL, ev.Actor, ev.GroupKey,
			string(meta), createdAt.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return inserted, fmt.Errorf("signal: insert event: %w", err)
		}
		n, _ := res.RowsAffected()
		inserted += int(n)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("signal: commit: %w", err)
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
		clauses = append(clauses, "created_at < (SELECT created_at FROM events WHERE id = ?)")
		args = append(args, f.Before)
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
		FROM events %s ORDER BY created_at DESC LIMIT ?`, where)
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
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, "UPDATE events SET read_at = ? WHERE id = ? AND read_at IS NULL", now, id)
	return err
}

// MarkAllRead marks all unread events as read.
func (s *EventStore) MarkAllRead(ctx context.Context) (int, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(ctx, "UPDATE events SET read_at = ? WHERE read_at IS NULL", now)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// Delete removes events older than the given duration.
func (s *EventStore) Delete(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339Nano)
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
		json.Unmarshal([]byte(metaStr), &ev.Metadata)
	}
	if ev.Metadata == nil {
		ev.Metadata = map[string]any{}
	}

	ev.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	if readStr.Valid {
		t, _ := time.Parse(time.RFC3339Nano, readStr.String)
		ev.ReadAt = &t
	}
	if archivedStr.Valid {
		t, _ := time.Parse(time.RFC3339Nano, archivedStr.String)
		ev.ArchivedAt = &t
	}

	return &ev, nil
}

func generateEventID() string {
	b := make([]byte, 12)
	rand.Read(b)
	return fmt.Sprintf("ev_%x", b)
}
