package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/avifenesh/cairn/internal/db"
	"github.com/avifenesh/cairn/internal/tool"
)

func setupTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := d.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestSessionStore_CreateAndGet(t *testing.T) {
	d := setupTestDB(t)
	store := NewSessionStore(d)
	ctx := context.Background()

	session := &Session{
		Title: "Test session",
		Mode:  tool.ModeTalk,
		State: map[string]any{"foo": "bar"},
	}

	if err := store.Create(ctx, session); err != nil {
		t.Fatalf("create: %v", err)
	}
	if session.ID == "" {
		t.Fatal("expected ID to be set")
	}

	got, err := store.Get(ctx, session.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "Test session" {
		t.Errorf("expected title 'Test session', got %q", got.Title)
	}
	if got.Mode != tool.ModeTalk {
		t.Errorf("expected mode 'talk', got %q", got.Mode)
	}
}

func TestSessionStore_AppendAndLoadEvents(t *testing.T) {
	d := setupTestDB(t)
	store := NewSessionStore(d)
	ctx := context.Background()

	session := &Session{Title: "Chat", Mode: tool.ModeTalk}
	store.Create(ctx, session)

	// Append user message.
	userEv := &Event{
		Author: "user",
		Parts:  []Part{TextPart{Text: "hello"}},
	}
	if err := store.AppendEvent(ctx, session.ID, userEv); err != nil {
		t.Fatalf("append user: %v", err)
	}

	// Append assistant response.
	assistantEv := &Event{
		Author: "cairn",
		Parts:  []Part{TextPart{Text: "hi there"}},
	}
	if err := store.AppendEvent(ctx, session.ID, assistantEv); err != nil {
		t.Fatalf("append assistant: %v", err)
	}

	// Load session and verify events.
	got, err := store.Get(ctx, session.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got.Events))
	}
	if got.Events[0].Author != "user" {
		t.Errorf("event 0: expected author 'user', got %q", got.Events[0].Author)
	}
	if got.Events[1].Author != "agent" { // stored as "assistant", mapped to "agent"
		t.Errorf("event 1: expected author 'agent', got %q", got.Events[1].Author)
	}
}

func TestSessionStore_List(t *testing.T) {
	d := setupTestDB(t)
	store := NewSessionStore(d)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		store.Create(ctx, &Session{Title: fmt.Sprintf("Session %d", i), Mode: tool.ModeTalk})
	}

	sessions, err := store.List(ctx, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}
}

func TestSessionStore_Delete(t *testing.T) {
	d := setupTestDB(t)
	store := NewSessionStore(d)
	ctx := context.Background()

	session := &Session{Title: "To delete", Mode: tool.ModeTalk}
	store.Create(ctx, session)
	store.AppendEvent(ctx, session.ID, &Event{Author: "user", Parts: []Part{TextPart{Text: "msg"}}})

	if err := store.Delete(ctx, session.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := store.Get(ctx, session.ID)
	if err == nil {
		t.Fatal("expected error after delete, got nil")
	}
}

func TestSessionStore_ToolParts(t *testing.T) {
	d := setupTestDB(t)
	store := NewSessionStore(d)
	ctx := context.Background()

	session := &Session{Title: "Tools", Mode: tool.ModeWork}
	store.Create(ctx, session)

	// Append event with tool call result.
	ev := &Event{
		Author: "cairn",
		Parts: []Part{
			TextPart{Text: "Let me read that file"},
			ToolPart{
				ToolName: "cairn.readFile",
				CallID:   "call_123",
				Status:   ToolCompleted,
				Output:   "file contents here",
			},
		},
	}
	if err := store.AppendEvent(ctx, session.ID, ev); err != nil {
		t.Fatalf("append: %v", err)
	}

	got, err := store.Get(ctx, session.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got.Events))
	}
	// The text and tool parts should both be present.
	if len(got.Events[0].Parts) < 2 {
		t.Fatalf("expected at least 2 parts, got %d", len(got.Events[0].Parts))
	}
}

func TestSession_History(t *testing.T) {
	session := &Session{
		Events: []*Event{
			{Author: "user", Parts: []Part{TextPart{Text: "hello"}}},
			{Author: "cairn", Parts: []Part{TextPart{Text: "hi"}}},
		},
	}

	msgs := session.History()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("msg 0: expected role 'user', got %q", msgs[0].Role)
	}
	if msgs[1].Role != "assistant" {
		t.Errorf("msg 1: expected role 'assistant', got %q", msgs[1].Role)
	}
}
