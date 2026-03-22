package agent

import (
	"context"
	"testing"

	"github.com/avifenesh/cairn/internal/tool"
)

func TestCheckpointStore_SaveLoadDelete(t *testing.T) {
	d := setupTestDB(t)
	store := NewCheckpointStore(d)
	ctx := context.Background()

	cp := &SessionCheckpoint{
		SessionID:   "sess-1",
		TaskID:      "task-1",
		Round:       5,
		Mode:        tool.ModeCoding,
		MaxRounds:   400,
		UserMessage: "implement feature X",
		Origin:      "task",
		State:       map[string]any{"workDir": "/tmp/wt-1"},
	}

	// Save.
	if err := store.Save(ctx, cp); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Load.
	loaded, err := store.Load(ctx, "sess-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Round != 5 {
		t.Errorf("round: got %d, want 5", loaded.Round)
	}
	if loaded.Mode != tool.ModeCoding {
		t.Errorf("mode: got %s, want coding", loaded.Mode)
	}
	if loaded.Origin != "task" {
		t.Errorf("origin: got %s, want task", loaded.Origin)
	}
	if loaded.UserMessage != "implement feature X" {
		t.Errorf("userMessage: got %q", loaded.UserMessage)
	}
	if loaded.State["workDir"] != "/tmp/wt-1" {
		t.Errorf("state.workDir: got %v", loaded.State["workDir"])
	}

	// Delete.
	if err := store.Delete(ctx, "sess-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = store.Load(ctx, "sess-1")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestCheckpointStore_UpsertOverwrites(t *testing.T) {
	d := setupTestDB(t)
	store := NewCheckpointStore(d)
	ctx := context.Background()

	cp := &SessionCheckpoint{
		SessionID: "sess-1",
		Round:     3,
		Mode:      tool.ModeWork,
		Origin:    "chat",
	}
	store.Save(ctx, cp)

	// Update round.
	cp.Round = 7
	store.Save(ctx, cp)

	loaded, err := store.Load(ctx, "sess-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Round != 7 {
		t.Errorf("round after upsert: got %d, want 7", loaded.Round)
	}
}

func TestCheckpointStore_ListIncomplete(t *testing.T) {
	d := setupTestDB(t)
	store := NewCheckpointStore(d)
	ctx := context.Background()

	store.Save(ctx, &SessionCheckpoint{SessionID: "s1", Round: 1, Origin: "chat", Mode: tool.ModeTalk})
	store.Save(ctx, &SessionCheckpoint{SessionID: "s2", Round: 5, Origin: "task", Mode: tool.ModeCoding})
	store.Save(ctx, &SessionCheckpoint{SessionID: "s3", Round: 2, Origin: "subagent", Mode: tool.ModeWork})

	list, err := store.ListIncomplete(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("list count: got %d, want 3", len(list))
	}
	// Verify ordered by created_at ASC.
	if list[0].SessionID != "s1" || list[1].SessionID != "s2" || list[2].SessionID != "s3" {
		t.Errorf("unexpected order: %s, %s, %s", list[0].SessionID, list[1].SessionID, list[2].SessionID)
	}
}

func TestRecoverSessions_CleansAllOrigins(t *testing.T) {
	d := setupTestDB(t)
	store := NewCheckpointStore(d)
	ctx := context.Background()

	store.Save(ctx, &SessionCheckpoint{SessionID: "chat-1", Origin: "chat", Mode: tool.ModeTalk})
	store.Save(ctx, &SessionCheckpoint{SessionID: "task-1", Origin: "task", Mode: tool.ModeCoding, TaskID: "t1"})
	store.Save(ctx, &SessionCheckpoint{SessionID: "sub-1", Origin: "subagent", Mode: tool.ModeWork})

	stats := RecoverSessions(ctx, SessionRecoveryDeps{
		CheckpointStore: store,
	})

	if stats.ChatCleaned != 1 || stats.TaskCleaned != 1 || stats.SubagentCleaned != 1 {
		t.Errorf("stats: chat=%d task=%d subagent=%d", stats.ChatCleaned, stats.TaskCleaned, stats.SubagentCleaned)
	}

	// All checkpoints should be deleted.
	remaining, _ := store.ListIncomplete(ctx)
	if len(remaining) != 0 {
		t.Errorf("expected 0 remaining checkpoints, got %d", len(remaining))
	}
}

func TestRecoverSessions_NilStore(t *testing.T) {
	stats := RecoverSessions(context.Background(), SessionRecoveryDeps{})
	if stats.ChatCleaned != 0 || stats.TaskCleaned != 0 || stats.SubagentCleaned != 0 {
		t.Error("expected zero stats with nil store")
	}
}
