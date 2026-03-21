package cron

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

const timeFormat = "2006-01-02T15:04:05Z"

// CronJob represents a scheduled recurring task.
type CronJob struct {
	ID          string     `json:"id"`
	Enabled     bool       `json:"enabled"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Schedule    string     `json:"schedule"`
	Instruction string     `json:"instruction"`
	Timezone    string     `json:"timezone"`
	Priority    int        `json:"priority"`
	CooldownMs  int64      `json:"cooldownMs"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	LastRunAt   *time.Time `json:"lastRunAt,omitempty"`
	NextRunAt   *time.Time `json:"nextRunAt,omitempty"`
}

// CronExecution records a single fire of a cron job.
type CronExecution struct {
	ID        string    `json:"id"`
	CronJobID string    `json:"cronJobId"`
	TaskID    string    `json:"taskId"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// Store manages cron jobs in SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates a new cron store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Create inserts a new cron job. Validates the schedule expression.
func (s *Store) Create(ctx context.Context, job *CronJob) error {
	if err := Validate(job.Schedule); err != nil {
		return err
	}
	if job.ID == "" {
		job.ID = newID("cron")
	}
	now := time.Now().UTC()
	job.CreatedAt = now
	job.UpdatedAt = now

	// Compute initial next_run in the job's timezone.
	loc := time.UTC
	if job.Timezone != "" && job.Timezone != "UTC" {
		if l, err := time.LoadLocation(job.Timezone); err == nil {
			loc = l
		}
	}
	if next, err := NextRun(job.Schedule, now.In(loc)); err == nil {
		nextUTC := next.UTC()
		job.NextRunAt = &nextUTC
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO cron_jobs (id, enabled, name, description, schedule, instruction, timezone, priority, cooldown_ms, created_at, updated_at, next_run_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, boolToInt(job.Enabled), job.Name, job.Description,
		job.Schedule, job.Instruction, job.Timezone,
		job.Priority, job.CooldownMs,
		now.Format(timeFormat), now.Format(timeFormat),
		formatTimePtr(job.NextRunAt),
	)
	return err
}

// List returns all cron jobs ordered by name.
func (s *Store) List(ctx context.Context) ([]*CronJob, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, enabled, name, description, schedule, instruction, timezone, priority, cooldown_ms, created_at, updated_at, last_run_at, next_run_at
		 FROM cron_jobs ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*CronJob
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// Get returns a single cron job by ID.
func (s *Store) Get(ctx context.Context, id string) (*CronJob, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, enabled, name, description, schedule, instruction, timezone, priority, cooldown_ms, created_at, updated_at, last_run_at, next_run_at
		 FROM cron_jobs WHERE id = ?`, id)
	return scanJobRow(row)
}

// GetByName returns a cron job by name.
func (s *Store) GetByName(ctx context.Context, name string) (*CronJob, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, enabled, name, description, schedule, instruction, timezone, priority, cooldown_ms, created_at, updated_at, last_run_at, next_run_at
		 FROM cron_jobs WHERE name = ?`, name)
	return scanJobRow(row)
}

// Update modifies a cron job. Only non-nil fields are updated.
func (s *Store) Update(ctx context.Context, id string, enabled *bool, schedule, instruction, description *string, priority *int, cooldownMs *int64) error {
	sets := []string{"updated_at = ?"}
	args := []any{time.Now().UTC().Format(timeFormat)}

	if enabled != nil {
		sets = append(sets, "enabled = ?")
		args = append(args, boolToInt(*enabled))
	}
	if schedule != nil {
		if err := Validate(*schedule); err != nil {
			return err
		}
		sets = append(sets, "schedule = ?")
		args = append(args, *schedule)
		// Recompute next_run.
		if next, err := NextRun(*schedule, time.Now().UTC()); err == nil {
			sets = append(sets, "next_run_at = ?")
			args = append(args, next.Format(timeFormat))
		}
	}
	if instruction != nil {
		sets = append(sets, "instruction = ?")
		args = append(args, *instruction)
	}
	if description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *description)
	}
	if priority != nil {
		sets = append(sets, "priority = ?")
		args = append(args, *priority)
	}
	if cooldownMs != nil {
		sets = append(sets, "cooldown_ms = ?")
		args = append(args, *cooldownMs)
	}

	args = append(args, id)
	query := "UPDATE cron_jobs SET " + joinStrings(sets, ", ") + " WHERE id = ?"
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Delete removes a cron job by ID. Cascade deletes executions.
// Returns an error if the job does not exist.
func (s *Store) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM cron_jobs WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetDueJobs returns enabled jobs that are due to run.
// A job is due if next_run_at <= now AND cooldown has elapsed since last_run_at.
func (s *Store) GetDueJobs(ctx context.Context, now time.Time) ([]*CronJob, error) {
	nowStr := now.Format(timeFormat)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, enabled, name, description, schedule, instruction, timezone, priority, cooldown_ms, created_at, updated_at, last_run_at, next_run_at
		 FROM cron_jobs
		 WHERE enabled = 1
		   AND (next_run_at IS NULL OR next_run_at <= ?)`,
		nowStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var due []*CronJob
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		// Check cooldown.
		if job.LastRunAt != nil {
			elapsed := now.Sub(*job.LastRunAt)
			if elapsed.Milliseconds() < job.CooldownMs {
				continue // cooldown active, skip
			}
		}
		due = append(due, job)
	}
	return due, rows.Err()
}

// UpdateAfterRun updates last_run_at and next_run_at after a job fires.
func (s *Store) UpdateAfterRun(ctx context.Context, id string, lastRun, nextRun time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE cron_jobs SET last_run_at = ?, next_run_at = ?, updated_at = ? WHERE id = ?`,
		lastRun.Format(timeFormat), nextRun.Format(timeFormat),
		time.Now().UTC().Format(timeFormat), id)
	return err
}

// RecordExecution logs a cron fire attempt.
func (s *Store) RecordExecution(ctx context.Context, cronJobID, taskID, status string, execErr error) error {
	errStr := ""
	if execErr != nil {
		errStr = execErr.Error()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO cron_executions (id, cron_job_id, task_id, status, error, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		newID("cexec"), cronJobID, taskID, status, errStr,
		time.Now().UTC().Format(timeFormat))
	return err
}

// ListExecutions returns recent executions for a cron job.
func (s *Store) ListExecutions(ctx context.Context, cronJobID string, limit int) ([]*CronExecution, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, cron_job_id, task_id, status, error, created_at
		 FROM cron_executions WHERE cron_job_id = ? ORDER BY created_at DESC LIMIT ?`,
		cronJobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var execs []*CronExecution
	for rows.Next() {
		var e CronExecution
		var createdStr string
		if err := rows.Scan(&e.ID, &e.CronJobID, &e.TaskID, &e.Status, &e.Error, &createdStr); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse(timeFormat, createdStr)
		execs = append(execs, &e)
	}
	return execs, rows.Err()
}

// --- helpers ---

func scanJob(rows *sql.Rows) (*CronJob, error) {
	var j CronJob
	var enabled int
	var createdStr, updatedStr string
	var lastRunStr, nextRunStr sql.NullString

	err := rows.Scan(&j.ID, &enabled, &j.Name, &j.Description,
		&j.Schedule, &j.Instruction, &j.Timezone,
		&j.Priority, &j.CooldownMs,
		&createdStr, &updatedStr, &lastRunStr, &nextRunStr)
	if err != nil {
		return nil, err
	}
	j.Enabled = enabled == 1
	j.CreatedAt, _ = time.Parse(timeFormat, createdStr)
	j.UpdatedAt, _ = time.Parse(timeFormat, updatedStr)
	if lastRunStr.Valid {
		t, _ := time.Parse(timeFormat, lastRunStr.String)
		j.LastRunAt = &t
	}
	if nextRunStr.Valid {
		t, _ := time.Parse(timeFormat, nextRunStr.String)
		j.NextRunAt = &t
	}
	return &j, nil
}

func scanJobRow(row *sql.Row) (*CronJob, error) {
	var j CronJob
	var enabled int
	var createdStr, updatedStr string
	var lastRunStr, nextRunStr sql.NullString

	err := row.Scan(&j.ID, &enabled, &j.Name, &j.Description,
		&j.Schedule, &j.Instruction, &j.Timezone,
		&j.Priority, &j.CooldownMs,
		&createdStr, &updatedStr, &lastRunStr, &nextRunStr)
	if err != nil {
		return nil, err
	}
	j.Enabled = enabled == 1
	j.CreatedAt, _ = time.Parse(timeFormat, createdStr)
	j.UpdatedAt, _ = time.Parse(timeFormat, updatedStr)
	if lastRunStr.Valid {
		t, _ := time.Parse(timeFormat, lastRunStr.String)
		j.LastRunAt = &t
	}
	if nextRunStr.Valid {
		t, _ := time.Parse(timeFormat, nextRunStr.String)
		j.NextRunAt = &t
	}
	return &j, nil
}

func newID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID.
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(b)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func formatTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format(timeFormat)
}

func joinStrings(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	result := ss[0]
	for _, s := range ss[1:] {
		result += sep + s
	}
	return result
}
