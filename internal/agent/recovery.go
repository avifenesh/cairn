package agent

import (
	"context"
	"database/sql"
	"log/slog"
	"time"
)

const loopStateID = "agent"

// LoopState holds persisted agent loop state for crash recovery.
type LoopState struct {
	TickCount      int64
	LastReflection time.Time
}

// RecoverOnStartup restores agent loop state from the database and fails
// any tasks stuck in running/claimed state with expired leases.
// Returns the restored state (zero values if no prior state exists).
func RecoverOnStartup(ctx context.Context, db *sql.DB, logger *slog.Logger) LoopState {
	state := LoopState{}
	if db == nil {
		return state
	}

	// 1. Restore loop state.
	var tickCount int64
	var lastReflectStr sql.NullString
	err := db.QueryRowContext(ctx,
		`SELECT tick_count, last_reflection_at FROM agent_loop_state WHERE id = ?`,
		loopStateID,
	).Scan(&tickCount, &lastReflectStr)
	if err == nil {
		state.TickCount = tickCount
		if lastReflectStr.Valid {
			if t, err := time.Parse("2006-01-02T15:04:05Z", lastReflectStr.String); err == nil {
				state.LastReflection = t
			}
		}
		logger.Info("agent state restored", "tickCount", state.TickCount, "lastReflection", state.LastReflection)
	} else if err != sql.ErrNoRows {
		logger.Warn("agent state restore failed", "error", err)
	}

	// 2. Fail stuck tasks (running or claimed with expired lease).
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	result, err := db.ExecContext(ctx,
		`UPDATE tasks SET status = 'failed', error = 'stuck_task_recovery: server restarted',
		 completed_at = ?
		 WHERE status IN ('running', 'claimed')
		   AND lease_expires_at IS NOT NULL
		   AND lease_expires_at < ?`,
		now, now)
	if err != nil {
		logger.Warn("stuck task recovery failed", "error", err)
	} else if n, _ := result.RowsAffected(); n > 0 {
		logger.Info("stuck tasks recovered", "count", n)
	}

	return state
}

// CheckpointState persists the current loop state to the database.
// Called after each tick for crash recovery.
func CheckpointState(ctx context.Context, db *sql.DB, tickCount int64, lastReflect time.Time) {
	if db == nil {
		return
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	lastStr := ""
	if !lastReflect.IsZero() {
		lastStr = lastReflect.Format("2006-01-02T15:04:05Z")
	}

	_, _ = db.ExecContext(ctx,
		`INSERT INTO agent_loop_state (id, tick_count, last_reflection_at, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET tick_count = ?, last_reflection_at = ?, updated_at = ?`,
		loopStateID, tickCount, lastStr, now,
		tickCount, lastStr, now)
}
