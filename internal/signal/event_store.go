package signal

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
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

// articleSourceList is the single source of truth for sources where
// same-URL = same-content article. URL dedup (cross-source and intra-batch)
// only applies to these sources.
var articleSourceList = []string{SourceRSS, SourceDevTo}

// articleSources is a membership set derived from articleSourceList,
// used for O(1) lookups during ingest filtering.
var articleSources = func() map[string]bool {
	m := make(map[string]bool, len(articleSourceList))
	for _, src := range articleSourceList {
		m[src] = true
	}
	return m
}()

// Ingest stores a batch of raw events, skipping duplicates via the
// UNIQUE(source, source_item_id) constraint and a cross-source URL
// dedup pass for article sources only (rss/devto — where same-URL
// means same-content article arriving via multiple pollers). Returns
// the slice of newly inserted events (deduped out events are excluded).
func (s *EventStore) Ingest(ctx context.Context, events []*RawEvent) ([]*RawEvent, error) {
	if len(events) == 0 {
		return nil, nil
	}

	// Begin transaction first — URL dedup must run inside the tx to
	// prevent TOCTOU races with concurrent poller goroutines.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("signal: begin tx: %w", err)
	}
	defer tx.Rollback()

	// Cross-source URL dedup: collect URLs from article sources only,
	// query for existing matches inside the transaction, and filter out
	// events whose URL already exists from any article source.
	urls := make([]string, 0, len(events))
	for _, ev := range events {
		if ev.URL != "" && articleSources[ev.Source] {
			urls = append(urls, ev.URL)
		}
	}
	seenURLs := make(map[string]bool)
	if len(urls) > 0 {
		seenURLs = s.queryExistingURLsTx(ctx, tx, urls)
	}

	// Filter out article-source events with already-seen URLs
	// (in DB or earlier in this batch).
	var filtered []*RawEvent
	for _, ev := range events {
		if ev.URL != "" && articleSources[ev.Source] {
			if seenURLs[ev.URL] {
				continue
			}
			// Track within this batch to catch duplicates from RSS feeds
			// aggregating the same article across multiple feed URLs.
			seenURLs[ev.URL] = true
		}
		filtered = append(filtered, ev)
	}
	if len(filtered) == 0 {
		return nil, nil
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO events (id, source, source_item_id, kind, title, body, url, actor, group_key, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source, source_item_id) DO NOTHING`)
	if err != nil {
		return nil, fmt.Errorf("signal: prepare insert: %w", err)
	}
	defer stmt.Close()

	var inserted []*RawEvent
	for _, ev := range filtered {
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
			ev.autoArchiveID = id // track ID for post-commit auto-archive
			inserted = append(inserted, ev)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("signal: commit: %w", err)
	}

	// Auto-archive events flagged with Metadata["autoArchive"]=true.
	for _, ev := range inserted {
		if ev.autoArchiveID != "" && ev.Metadata != nil {
			if autoArchive, ok := ev.Metadata["autoArchive"].(bool); ok && autoArchive {
				_ = s.Archive(ctx, ev.autoArchiveID)
			}
		}
	}

	return inserted, nil
}

// queryExistingURLsTx returns a set of URLs that already exist in the events
// table for article sources only (rss, devto). This enables cross-source
// dedup for feed items that arrive through multiple pollers (e.g. dev.to
// articles via both the devto poller and an RSS feed subscription).
//
// Must be called inside a transaction to prevent TOCTOU races with
// concurrent Ingest calls.
func (s *EventStore) queryExistingURLsTx(ctx context.Context, tx *sql.Tx, urls []string) map[string]bool {
	if len(urls) == 0 {
		return map[string]bool{}
	}

	// Only query article sources to avoid false-positive dedup of event
	// sources (github, npm, crates) where same-URL ≠ same-content.
	placeholders := make([]string, len(articleSourceList))
	for i := range articleSourceList {
		placeholders[i] = "?"
	}

	urlPlaceholders := make([]string, len(urls))
	sourceArgs := make([]any, len(articleSourceList))
	urlArgs := make([]any, len(urls))
	for i, src := range articleSourceList {
		sourceArgs[i] = src
	}
	for i, u := range urls {
		urlPlaceholders[i] = "?"
		urlArgs[i] = u
	}

	allArgs := append(sourceArgs, urlArgs...)
	query := fmt.Sprintf("SELECT DISTINCT url FROM events WHERE source IN (%s) AND url IN (%s)", strings.Join(placeholders, ", "), strings.Join(urlPlaceholders, ", "))
	rows, err := tx.QueryContext(ctx, query, allArgs...)
	if err != nil {
		slog.Warn("signal: cross-source URL dedup query failed, skipping", "error", err)
		return map[string]bool{}
	}
	defer rows.Close()

	result := make(map[string]bool, len(urls))
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			slog.Warn("signal: cross-source URL dedup scan failed, skipping query", "error", err)
			return map[string]bool{}
		}
		result[url] = true
	}
	if err := rows.Err(); err != nil {
		slog.Warn("signal: cross-source URL dedup rows iteration failed, skipping", "error", err)
		return map[string]bool{}
	}
	return result
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
	if f.ExcludeArchived {
		clauses = append(clauses, "archived_at IS NULL")
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

// CountBySource returns event counts grouped by source (excluding archived).
func (s *EventStore) CountBySource(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT source, COUNT(*) FROM events WHERE archived_at IS NULL GROUP BY source")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]int{}
	for rows.Next() {
		var source string
		var count int
		if err := rows.Scan(&source, &count); err != nil {
			return nil, err
		}
		result[source] = count
	}
	return result, rows.Err()
}

// CountArchivedBySource returns archived event counts grouped by source.
func (s *EventStore) CountArchivedBySource(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT source, COUNT(*) FROM events WHERE archived_at IS NOT NULL GROUP BY source")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]int{}
	for rows.Next() {
		var source string
		var count int
		if err := rows.Scan(&source, &count); err != nil {
			return nil, err
		}
		result[source] = count
	}
	return result, rows.Err()
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
