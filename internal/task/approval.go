package task

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ApprovalStatus represents the lifecycle state of an approval.
type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "pending"
	ApprovalApproved ApprovalStatus = "approved"
	ApprovalDenied   ApprovalStatus = "denied"
	ApprovalExpired  ApprovalStatus = "expired"
)

// Approval represents a human-in-the-loop gate.
type Approval struct {
	ID          string          `json:"id"`
	TaskID      string          `json:"taskId"`
	Type        string          `json:"type"` // merge_pr, send_email, push_main, budget_override, soul_patch, etc.
	Status      ApprovalStatus  `json:"status"`
	Description string          `json:"description"`
	Context     json.RawMessage `json:"context"` // type-specific metadata (PR URL, email preview, etc.)
	DecidedAt   *time.Time      `json:"decidedAt,omitempty"`
	DecidedBy   string          `json:"decidedBy,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
}

// ApprovalStore manages approvals in SQLite.
type ApprovalStore struct {
	db *sql.DB
}

// NewApprovalStore creates an approval store.
func NewApprovalStore(db *sql.DB) *ApprovalStore {
	return &ApprovalStore{db: db}
}

// Create inserts a new pending approval.
func (s *ApprovalStore) Create(ctx context.Context, a *Approval) error {
	if a.ID == "" {
		b := make([]byte, 12)
		rand.Read(b)
		a.ID = fmt.Sprintf("apr_%x", b)
	}
	if a.Status == "" {
		a.Status = ApprovalPending
	}
	if a.Context == nil {
		a.Context = json.RawMessage("{}")
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	// Use NULL for empty TaskID to satisfy FK constraint.
	var taskID any
	if a.TaskID != "" {
		taskID = a.TaskID
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO approvals (id, task_id, type, status, description, context, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ID, taskID, a.Type, a.Status, a.Description, string(a.Context), now,
	)
	return err
}

// Get retrieves an approval by ID.
func (s *ApprovalStore) Get(ctx context.Context, id string) (*Approval, error) {
	var a Approval
	var taskID sql.NullString
	var ctxStr, createdStr string
	var decidedStr sql.NullString
	var decidedBy sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, task_id, type, status, description, context, decided_at, decided_by, created_at
		FROM approvals WHERE id = ?`, id,
	).Scan(&a.ID, &taskID, &a.Type, &a.Status, &a.Description, &ctxStr, &decidedStr, &decidedBy, &createdStr)
	if err != nil {
		return nil, err
	}
	if taskID.Valid {
		a.TaskID = taskID.String
	}
	a.Context = json.RawMessage(ctxStr)
	a.CreatedAt, _ = time.Parse("2006-01-02T15:04:05.000Z", createdStr)
	if decidedStr.Valid {
		t, _ := time.Parse("2006-01-02T15:04:05.000Z", decidedStr.String)
		a.DecidedAt = &t
	}
	if decidedBy.Valid {
		a.DecidedBy = decidedBy.String
	}
	return &a, nil
}

// ListPending returns all pending approvals.
func (s *ApprovalStore) ListPending(ctx context.Context) ([]*Approval, error) {
	return s.list(ctx, "pending")
}

// List returns approvals filtered by status (empty = all).
func (s *ApprovalStore) List(ctx context.Context, status string) ([]*Approval, error) {
	return s.list(ctx, status)
}

func (s *ApprovalStore) list(ctx context.Context, status string) ([]*Approval, error) {
	query := `SELECT id, task_id, type, status, description, context, decided_at, decided_by, created_at FROM approvals`
	var args []any
	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC LIMIT 50"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var approvals []*Approval
	for rows.Next() {
		var a Approval
		var taskID sql.NullString
		var ctxStr, createdStr string
		var decidedStr, decidedBy sql.NullString
		if err := rows.Scan(&a.ID, &taskID, &a.Type, &a.Status, &a.Description, &ctxStr, &decidedStr, &decidedBy, &createdStr); err != nil {
			return nil, err
		}
		if taskID.Valid {
			a.TaskID = taskID.String
		}
		a.Context = json.RawMessage(ctxStr)
		a.CreatedAt, _ = time.Parse("2006-01-02T15:04:05.000Z", createdStr)
		if decidedStr.Valid {
			t, _ := time.Parse("2006-01-02T15:04:05.000Z", decidedStr.String)
			a.DecidedAt = &t
		}
		if decidedBy.Valid {
			a.DecidedBy = decidedBy.String
		}
		approvals = append(approvals, &a)
	}
	return approvals, nil
}

// Approve marks an approval as approved.
func (s *ApprovalStore) Approve(ctx context.Context, id, decidedBy string) error {
	return s.decide(ctx, id, ApprovalApproved, decidedBy)
}

// Deny marks an approval as denied.
func (s *ApprovalStore) Deny(ctx context.Context, id, decidedBy string) error {
	return s.decide(ctx, id, ApprovalDenied, decidedBy)
}

func (s *ApprovalStore) decide(ctx context.Context, id string, status ApprovalStatus, decidedBy string) error {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := s.db.ExecContext(ctx, `
		UPDATE approvals SET status = ?, decided_at = ?, decided_by = ?
		WHERE id = ? AND status = 'pending'`,
		status, now, decidedBy, id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
