package rules

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const timeFormat = "2006-01-02T15:04:05.000Z"

// Store provides SQLite CRUD for automation rules and execution logs.
type Store struct {
	db *sql.DB
}

// NewStore creates a rules store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func generateID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return fmt.Sprintf("rule_%x", b)
}

func generateExecID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return fmt.Sprintf("rx_%x", b)
}

// Create inserts a new rule.
func (s *Store) Create(ctx context.Context, r *Rule) error {
	if r.ID == "" {
		r.ID = generateID()
	}
	now := time.Now().UTC()
	r.CreatedAt = now
	r.UpdatedAt = now

	triggerJSON, err := json.Marshal(r.Trigger)
	if err != nil {
		return fmt.Errorf("rules: marshal trigger: %w", err)
	}
	actionsJSON, err := json.Marshal(r.Actions)
	if err != nil {
		return fmt.Errorf("rules: marshal actions: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO rules (id, name, description, enabled, trigger, condition, actions, throttle_ms, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Name, r.Description, r.Enabled,
		string(triggerJSON), r.Condition, string(actionsJSON),
		r.ThrottleMs,
		now.Format(timeFormat), now.Format(timeFormat),
	)
	if err != nil {
		return fmt.Errorf("rules: create %q: %w", r.Name, err)
	}
	return nil
}

// Get retrieves a rule by ID.
func (s *Store) Get(ctx context.Context, id string) (*Rule, error) {
	return s.scanOne(ctx, `SELECT id, name, description, enabled, trigger, condition, actions, throttle_ms, created_at, updated_at, last_fired_at FROM rules WHERE id = ?`, id)
}

// GetByName retrieves a rule by name.
func (s *Store) GetByName(ctx context.Context, name string) (*Rule, error) {
	return s.scanOne(ctx, `SELECT id, name, description, enabled, trigger, condition, actions, throttle_ms, created_at, updated_at, last_fired_at FROM rules WHERE name = ?`, name)
}

// List returns all rules ordered by name.
func (s *Store) List(ctx context.Context) ([]*Rule, error) {
	return s.scanMany(ctx, `SELECT id, name, description, enabled, trigger, condition, actions, throttle_ms, created_at, updated_at, last_fired_at FROM rules ORDER BY name`)
}

// ListEnabled returns only enabled rules.
func (s *Store) ListEnabled(ctx context.Context) ([]*Rule, error) {
	return s.scanMany(ctx, `SELECT id, name, description, enabled, trigger, condition, actions, throttle_ms, created_at, updated_at, last_fired_at FROM rules WHERE enabled = 1 ORDER BY name`)
}

// Update modifies a rule.
func (s *Store) Update(ctx context.Context, id string, opts UpdateOpts) error {
	// Build SET clause dynamically.
	sets := []string{"updated_at = ?"}
	args := []any{time.Now().UTC().Format(timeFormat)}

	if opts.Enabled != nil {
		sets = append(sets, "enabled = ?")
		args = append(args, *opts.Enabled)
	}
	if opts.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *opts.Name)
	}
	if opts.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *opts.Description)
	}
	if opts.Trigger != nil {
		b, err := json.Marshal(opts.Trigger)
		if err != nil {
			return fmt.Errorf("rules: marshal trigger: %w", err)
		}
		sets = append(sets, "trigger = ?")
		args = append(args, string(b))
	}
	if opts.Condition != nil {
		sets = append(sets, "condition = ?")
		args = append(args, *opts.Condition)
	}
	if opts.Actions != nil {
		b, err := json.Marshal(opts.Actions)
		if err != nil {
			return fmt.Errorf("rules: marshal actions: %w", err)
		}
		sets = append(sets, "actions = ?")
		args = append(args, string(b))
	}
	if opts.ThrottleMs != nil {
		sets = append(sets, "throttle_ms = ?")
		args = append(args, *opts.ThrottleMs)
	}

	query := "UPDATE rules SET "
	for i, s := range sets {
		if i > 0 {
			query += ", "
		}
		query += s
	}
	query += " WHERE id = ?"
	args = append(args, id)

	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("rules: update %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a rule and its executions (cascade).
func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("rules: delete %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateLastFired sets the last_fired_at timestamp.
func (s *Store) UpdateLastFired(ctx context.Context, id string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE rules SET last_fired_at = ? WHERE id = ?`,
		at.Format(timeFormat), id)
	return err
}

// RecordExecution logs a rule execution.
func (s *Store) RecordExecution(ctx context.Context, exec *Execution) error {
	if exec.ID == "" {
		exec.ID = generateExecID()
	}
	if exec.CreatedAt.IsZero() {
		exec.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO rule_executions (id, rule_id, trigger_event, status, error, duration_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		exec.ID, exec.RuleID, exec.TriggerEvent, exec.Status, exec.Error, exec.DurationMs,
		exec.CreatedAt.Format(timeFormat),
	)
	return err
}

// ListExecutions returns executions for a rule, most recent first.
func (s *Store) ListExecutions(ctx context.Context, ruleID string, limit int) ([]*Execution, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, rule_id, trigger_event, status, error, duration_ms, created_at
		FROM rule_executions WHERE rule_id = ? ORDER BY created_at DESC LIMIT ?`,
		ruleID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanExecutions(rows)
}

// ListRecentExecutions returns recent executions across all rules.
func (s *Store) ListRecentExecutions(ctx context.Context, limit int) ([]*Execution, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, rule_id, trigger_event, status, error, duration_ms, created_at
		FROM rule_executions ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanExecutions(rows)
}

// --- Internal scan helpers ---

func (s *Store) scanOne(ctx context.Context, query string, args ...any) (*Rule, error) {
	row := s.db.QueryRowContext(ctx, query, args...)
	r, err := scanRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return r, err
}

func (s *Store) scanMany(ctx context.Context, query string, args ...any) ([]*Rule, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*Rule
	for rows.Next() {
		r, err := scanRuleRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func scanRule(row *sql.Row) (*Rule, error) {
	var r Rule
	var triggerJSON, actionsJSON string
	var createdAt, updatedAt string
	var lastFired sql.NullString

	err := row.Scan(&r.ID, &r.Name, &r.Description, &r.Enabled,
		&triggerJSON, &r.Condition, &actionsJSON,
		&r.ThrottleMs, &createdAt, &updatedAt, &lastFired)
	if err != nil {
		return nil, err
	}
	return parseRule(&r, triggerJSON, actionsJSON, createdAt, updatedAt, lastFired)
}

func scanRuleRow(rows *sql.Rows) (*Rule, error) {
	var r Rule
	var triggerJSON, actionsJSON string
	var createdAt, updatedAt string
	var lastFired sql.NullString

	err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.Enabled,
		&triggerJSON, &r.Condition, &actionsJSON,
		&r.ThrottleMs, &createdAt, &updatedAt, &lastFired)
	if err != nil {
		return nil, err
	}
	return parseRule(&r, triggerJSON, actionsJSON, createdAt, updatedAt, lastFired)
}

func parseRule(r *Rule, triggerJSON, actionsJSON, createdAt, updatedAt string, lastFired sql.NullString) (*Rule, error) {
	if err := json.Unmarshal([]byte(triggerJSON), &r.Trigger); err != nil {
		return nil, fmt.Errorf("rules: unmarshal trigger: %w", err)
	}
	if err := json.Unmarshal([]byte(actionsJSON), &r.Actions); err != nil {
		return nil, fmt.Errorf("rules: unmarshal actions: %w", err)
	}
	r.CreatedAt, _ = time.Parse(timeFormat, createdAt)
	r.UpdatedAt, _ = time.Parse(timeFormat, updatedAt)
	if lastFired.Valid {
		t, _ := time.Parse(timeFormat, lastFired.String)
		r.LastFiredAt = &t
	}
	return r, nil
}

func scanExecutions(rows *sql.Rows) ([]*Execution, error) {
	var result []*Execution
	for rows.Next() {
		var e Execution
		var triggerEvent sql.NullString
		var errStr sql.NullString
		var durationMs sql.NullInt64
		var createdAt string

		if err := rows.Scan(&e.ID, &e.RuleID, &triggerEvent, &e.Status, &errStr, &durationMs, &createdAt); err != nil {
			return nil, err
		}
		if triggerEvent.Valid {
			e.TriggerEvent = triggerEvent.String
		}
		if errStr.Valid {
			e.Error = errStr.String
		}
		if durationMs.Valid {
			e.DurationMs = durationMs.Int64
		}
		e.CreatedAt, _ = time.Parse(timeFormat, createdAt)
		result = append(result, &e)
	}
	return result, rows.Err()
}
