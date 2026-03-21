package agent

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/task"
)

const loopStateID = "agent"

// LoopState holds persisted agent loop state for crash recovery.
type LoopState struct {
	TickCount      int64
	LastReflection time.Time
}

// RecoveryDeps carries dependencies for startup recovery.
type RecoveryDeps struct {
	DB            *sql.DB
	TaskEngine    *task.Engine
	ActivityStore *ActivityStore
	Bus           *eventbus.Bus
	Logger        *slog.Logger
}

// RecoveryStats reports what happened during startup recovery.
type RecoveryStats struct {
	Requeued []string // task IDs re-queued for retry
	Failed   []string // task IDs terminally failed
	Total    int
}

// RecoverOnStartup restores agent loop state from the database and recovers
// any tasks stuck in running/claimed state. Tasks with retries remaining are
// re-queued; exhausted tasks are marked failed with events published.
// Returns the restored state and recovery statistics.
func RecoverOnStartup(ctx context.Context, deps RecoveryDeps) (LoopState, RecoveryStats) {
	state := LoopState{}
	stats := RecoveryStats{}

	if deps.DB == nil {
		return state, stats
	}

	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// 1. Restore loop state.
	var tickCount int64
	var lastReflectStr sql.NullString
	err := deps.DB.QueryRowContext(ctx,
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

	// 2. Recover stuck tasks via engine (re-queues retryable, fails exhausted).
	if deps.TaskEngine != nil {
		requeued, failed := deps.TaskEngine.RecoverStuck(ctx, "stuck_task_recovery: server restarted")
		stats.Requeued = requeued
		stats.Failed = failed
		stats.Total = len(requeued) + len(failed)

		if stats.Total > 0 {
			logger.Info("stuck tasks recovered",
				"total", stats.Total,
				"requeued", len(requeued),
				"failed", len(failed))

			// Record activity for UI visibility.
			if deps.ActivityStore != nil {
				summary := fmt.Sprintf("Restart recovery: %d tasks (%d requeued, %d failed)",
					stats.Total, len(requeued), len(failed))
				var details strings.Builder
				if len(requeued) > 0 {
					fmt.Fprintf(&details, "Re-queued for retry: %s\n", strings.Join(requeued, ", "))
				}
				if len(failed) > 0 {
					fmt.Fprintf(&details, "Failed (retries exhausted): %s\n", strings.Join(failed, ", "))
				}
				entry := ActivityEntry{
					Type:    "recovery",
					Summary: summary,
					Details: details.String(),
				}
				deps.ActivityStore.Record(ctx, entry)

				// Publish for SSE so frontend sees it immediately.
				if deps.Bus != nil {
					eventbus.Publish(deps.Bus, AgentActivityEvent{
						EventMeta: eventbus.NewMeta("recovery"),
						Entry:     entry,
					})
				}
			}
		}
	}

	return state, stats
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
