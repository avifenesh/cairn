package task

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/avifenesh/cairn/internal/db"
)

// Store provides SQLite-backed persistence for tasks.
type Store struct {
	db *db.DB
}

// NewStore creates a Store backed by the given database.
func NewStore(d *db.DB) *Store {
	return &Store{db: d}
}

// isoTime formats a time.Time as ISO-8601 for SQLite TEXT columns.
func isoTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// parseTime parses an ISO-8601 string from SQLite back into time.Time.
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02T15:04:05.000Z", s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// Create inserts a new task into the database.
func (s *Store) Create(ctx context.Context, t *Task) error {
	inputStr := "{}"
	if len(t.Input) > 0 {
		inputStr = string(t.Input)
	}

	metadata := "{}"
	// Pack extra fields not in the base schema into metadata.
	meta := map[string]any{
		"parent_id":   t.ParentID,
		"session_id":  t.SessionID,
		"mode":        t.Mode,
		"worktree_dir": t.WorktreeDir,
		"retries":     t.Retries,
		"max_retries": t.MaxRetries,
		"cost_usd":    t.CostUSD,
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("task store: marshal metadata: %w", err)
	}
	metadata = string(metaBytes)

	now := isoTime(time.Now())

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO tasks (id, type, status, description, input, output, error, priority,
			created_at, started_at, completed_at, lease_owner, lease_expires_at, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID,
		string(t.Type),
		string(t.Status),
		"", // description stored in metadata or input
		inputStr,
		nullStr(string(t.Output)),
		nullStr(t.Error),
		int(t.Priority),
		now,
		nullStr(""),
		nullStr(""),
		nullStr(t.LeaseOwner),
		nullStr(isoTime(t.LeaseExpiry)),
		metadata,
	)
	if err != nil {
		return fmt.Errorf("task store: create %s: %w", t.ID, err)
	}
	return nil
}

// Get retrieves a task by ID.
func (s *Store) Get(ctx context.Context, id string) (*Task, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, type, status, input, output, error, priority,
			created_at, started_at, completed_at, lease_owner, lease_expires_at, metadata
		FROM tasks WHERE id = ?`, id)
	return scanTask(row)
}

// List retrieves tasks matching the given filters.
func (s *Store) List(ctx context.Context, opts ListOpts) ([]*Task, error) {
	query := `SELECT id, type, status, input, output, error, priority,
		created_at, started_at, completed_at, lease_owner, lease_expires_at, metadata
		FROM tasks WHERE 1=1`
	var args []any

	if opts.Status != "" {
		query += " AND status = ?"
		args = append(args, string(opts.Status))
	}
	if opts.Type != "" {
		query += " AND type = ?"
		args = append(args, string(opts.Type))
	}
	if !opts.Before.IsZero() {
		query += " AND created_at < ?"
		args = append(args, isoTime(opts.Before))
	}
	if opts.Archived {
		query += " AND archived_at IS NOT NULL"
	} else {
		query += " AND archived_at IS NULL"
	}

	query += " ORDER BY priority ASC, created_at ASC"

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("task store: list: %w", err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		t, err := scanTaskRows(rows)
		if err != nil {
			return nil, fmt.Errorf("task store: scan list row: %w", err)
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// Update persists changes to an existing task.
func (s *Store) Update(ctx context.Context, t *Task) error {
	meta := map[string]any{
		"parent_id":   t.ParentID,
		"session_id":  t.SessionID,
		"mode":        t.Mode,
		"worktree_dir": t.WorktreeDir,
		"retries":     t.Retries,
		"max_retries": t.MaxRetries,
		"cost_usd":    t.CostUSD,
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("task store: marshal metadata: %w", err)
	}

	outputStr := sql.NullString{}
	if len(t.Output) > 0 {
		outputStr = sql.NullString{String: string(t.Output), Valid: true}
	}

	// Preserve started_at: only set when transitioning to running, never overwrite.
	startedAtExpr := "started_at" // keep existing value
	var startedAtArg any
	if !t.StartedAt.IsZero() {
		startedAtExpr = "?"
		startedAtArg = isoTime(t.StartedAt)
	}

	query := fmt.Sprintf(`
		UPDATE tasks SET
			status = ?, output = ?, error = ?, priority = ?,
			started_at = COALESCE(started_at, %s), completed_at = ?,
			lease_owner = ?, lease_expires_at = ?,
			metadata = ?
		WHERE id = ?`, startedAtExpr)

	args := []any{
		string(t.Status),
		outputStr,
		nullStr(t.Error),
		int(t.Priority),
	}
	if startedAtArg != nil {
		args = append(args, startedAtArg)
	}
	args = append(args,
		nullStr(isoTime(t.CompletedAt)),
		nullStr(t.LeaseOwner),
		nullStr(isoTime(t.LeaseExpiry)),
		string(metaBytes),
		t.ID,
	)

	_, err = s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("task store: update %s: %w", t.ID, err)
	}
	return nil
}

// Claim atomically picks the highest-priority queued task of the given type,
// sets it to claimed status with a lease, and returns it.
func (s *Store) Claim(ctx context.Context, taskType TaskType, owner string, leaseDuration time.Duration) (*Task, error) {
	expiry := isoTime(time.Now().Add(leaseDuration))

	// SQLite doesn't support UPDATE ... ORDER BY ... LIMIT ... RETURNING in all drivers,
	// so we use a subquery to find the ID, then update it.
	row := s.db.QueryRowContext(ctx, `
		UPDATE tasks SET status = 'claimed', lease_owner = ?, lease_expires_at = ?
		WHERE id = (
			SELECT id FROM tasks
			WHERE status = 'queued' AND type = ?
			ORDER BY priority ASC, created_at ASC
			LIMIT 1
		)
		RETURNING id, type, status, input, output, error, priority,
			created_at, started_at, completed_at, lease_owner, lease_expires_at, metadata`,
		owner, expiry, string(taskType),
	)

	t, err := scanTask(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // no queued tasks of this type
		}
		return nil, fmt.Errorf("task store: claim: %w", err)
	}
	return t, nil
}

// Heartbeat extends the lease on a claimed/running task.
func (s *Store) Heartbeat(ctx context.Context, id string, leaseDuration time.Duration) error {
	expiry := isoTime(time.Now().Add(leaseDuration))
	result, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET lease_expires_at = ?
		WHERE id = ? AND status IN ('claimed', 'running')`,
		expiry, id,
	)
	if err != nil {
		return fmt.Errorf("task store: heartbeat %s: %w", id, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("task store: heartbeat %s: task not found or not claimable", id)
	}
	return nil
}

// FindExpiredLeases returns tasks whose leases have expired.
func (s *Store) FindExpiredLeases(ctx context.Context) ([]*Task, error) {
	now := isoTime(time.Now())
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, type, status, input, output, error, priority,
			created_at, started_at, completed_at, lease_owner, lease_expires_at, metadata
		FROM tasks
		WHERE status IN ('claimed', 'running')
			AND lease_expires_at IS NOT NULL
			AND lease_expires_at != ''
			AND lease_expires_at < ?`, now)
	if err != nil {
		return nil, fmt.Errorf("task store: find expired leases: %w", err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		t, err := scanTaskRows(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// scannable is an interface satisfied by both *sql.Row and *sql.Rows.
type scannable interface {
	Scan(dest ...any) error
}

func scanTask(row scannable) (*Task, error) {
	var (
		id, typ, status       string
		input, metadata       string
		output, taskErr       sql.NullString
		priority              int
		createdAt             string
		startedAt, completedAt sql.NullString
		leaseOwner            sql.NullString
		leaseExpiresAt        sql.NullString
	)

	err := row.Scan(
		&id, &typ, &status, &input, &output, &taskErr, &priority,
		&createdAt, &startedAt, &completedAt, &leaseOwner, &leaseExpiresAt, &metadata,
	)
	if err != nil {
		return nil, err
	}

	t := &Task{
		ID:        id,
		Type:      TaskType(typ),
		Status:    TaskStatus(status),
		Priority:  Priority(priority),
		Input:     json.RawMessage(input),
		CreatedAt: parseTime(createdAt),
	}

	if output.Valid {
		t.Output = json.RawMessage(output.String)
	}
	if taskErr.Valid {
		t.Error = taskErr.String
	}
	if leaseOwner.Valid {
		t.LeaseOwner = leaseOwner.String
	}
	if leaseExpiresAt.Valid {
		t.LeaseExpiry = parseTime(leaseExpiresAt.String)
	}
	if startedAt.Valid {
		t.StartedAt = parseTime(startedAt.String)
	}
	if completedAt.Valid {
		t.CompletedAt = parseTime(completedAt.String)
	}

	// Unpack metadata fields.
	if metadata != "" && metadata != "{}" {
		var meta map[string]any
		if err := json.Unmarshal([]byte(metadata), &meta); err == nil {
			if v, ok := meta["parent_id"].(string); ok {
				t.ParentID = v
			}
			if v, ok := meta["session_id"].(string); ok {
				t.SessionID = v
			}
			if v, ok := meta["mode"].(string); ok {
				t.Mode = v
			}
			if v, ok := meta["worktree_dir"].(string); ok {
				t.WorktreeDir = v
			}
			if v, ok := meta["retries"].(float64); ok {
				t.Retries = int(v)
			}
			if v, ok := meta["max_retries"].(float64); ok {
				t.MaxRetries = int(v)
			}
			if v, ok := meta["cost_usd"].(float64); ok {
				t.CostUSD = v
			}
		}
	}

	return t, nil
}

func scanTaskRows(rows *sql.Rows) (*Task, error) {
	return scanTask(rows)
}

// nullStr returns a sql.NullString — valid only when s is non-empty.
func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

