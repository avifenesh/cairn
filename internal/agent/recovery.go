package agent

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

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
	TaskEngine    *task.Engine
	ActivityStore *ActivityStore
	Logger        *slog.Logger
}

// RecoveryStats reports what happened during startup recovery.
type RecoveryStats struct {
	Requeued []string // task IDs re-queued for retry
	Failed   []string // task IDs terminally failed
	Total    int
}

// RecoverOnStartup recovers any tasks stuck in running/claimed state.
// Tasks with retries remaining are re-queued; exhausted tasks are marked
// failed with events published. Runs unconditionally on startup (not gated
// by idle mode). Loop state is restored separately via RecoverLoopState.
func RecoverOnStartup(ctx context.Context, deps RecoveryDeps) RecoveryStats {
	stats := RecoveryStats{}

	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Recover stuck tasks via engine (re-queues retryable, fails exhausted).
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
				if err := deps.ActivityStore.Record(ctx, entry); err != nil {
					logger.Warn("recovery: failed to record activity", "error", err)
				}
				// Note: no bus event here — SSE broadcaster isn't attached yet
				// at startup. Frontend loads activity from REST on connect.
			}
		}
	}

	return stats
}

// RecoverLoopState restores only the agent loop state (tick count, reflection time)
// from the database. Separated from RecoverOnStartup so loop state can be restored
// inside the idle-mode block while task recovery runs unconditionally.
func RecoverLoopState(ctx context.Context, db *sql.DB, logger *slog.Logger) (LoopState, error) {
	state := LoopState{}
	if db == nil {
		return state, nil
	}
	if logger == nil {
		logger = slog.Default()
	}

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

	return state, nil
}

// SessionRecoveryDeps carries dependencies for session checkpoint recovery.
type SessionRecoveryDeps struct {
	CheckpointStore *CheckpointStore
	TaskEngine      *task.Engine
	Logger          *slog.Logger
}

// SessionRecoveryStats reports what happened during session recovery.
type SessionRecoveryStats struct {
	ChatCleaned     int
	TaskCleaned     int
	SubagentCleaned int
}

// RecoverSessions detects incomplete sessions (those with checkpoints) and
// cleans them up. For task sessions, the existing RecoverOnStartup re-queues
// the task and the loop will load the persisted session for continuity.
func RecoverSessions(ctx context.Context, deps SessionRecoveryDeps) SessionRecoveryStats {
	stats := SessionRecoveryStats{}
	if deps.CheckpointStore == nil {
		return stats
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}

	checkpoints, err := deps.CheckpointStore.ListIncomplete(ctx)
	if err != nil {
		logger.Warn("session recovery: failed to list checkpoints", "error", err)
		return stats
	}
	if len(checkpoints) == 0 {
		return stats
	}

	for _, cp := range checkpoints {
		switch cp.Origin {
		case "chat":
			// Chat sessions: clean up checkpoint. User re-sends to continue.
			logger.Info("session recovery: chat session interrupted",
				"session", cp.SessionID, "round", cp.Round)
			stats.ChatCleaned++

		case "task":
			// Task sessions: checkpoint cleaned up. RecoverOnStartup already
			// re-queued the task; the loop will load the existing session.
			logger.Info("session recovery: task session interrupted",
				"session", cp.SessionID, "task", cp.TaskID, "round", cp.Round)
			stats.TaskCleaned++

		case "subagent":
			logger.Info("session recovery: subagent session interrupted",
				"session", cp.SessionID, "round", cp.Round)
			stats.SubagentCleaned++

		default:
			logger.Warn("session recovery: unknown origin", "origin", cp.Origin, "session", cp.SessionID)
		}

		// Always delete checkpoint after processing.
		deps.CheckpointStore.Delete(ctx, cp.SessionID)
	}

	logger.Info("session recovery complete",
		"chat", stats.ChatCleaned,
		"task", stats.TaskCleaned,
		"subagent", stats.SubagentCleaned)
	return stats
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
