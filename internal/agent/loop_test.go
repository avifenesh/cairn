package agent

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/avifenesh/cairn/internal/db"
	"github.com/avifenesh/cairn/internal/eventbus"
)

func setupTestDBForAgent(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database.DB
}

// --- JournalStore tests ---

func TestJournalStore_SaveAndRecent(t *testing.T) {
	sqlDB := setupTestDBForAgent(t)
	store := NewJournalStore(sqlDB)
	ctx := context.Background()

	entry := &JournalEntry{
		SessionID:  "session-1",
		Summary:    "Fixed a bug in the event store",
		Decisions:  []string{"used ON CONFLICT for dedup"},
		Errors:     []string{},
		Learnings:  []string{"SQLite dedup is fast"},
		Entities:   []string{"event_store", "SQLite"},
		ToolCount:  3,
		RoundCount: 2,
		Mode:       "coding",
		DurationMs: 5000,
	}

	if err := store.Save(ctx, entry); err != nil {
		t.Fatalf("save: %v", err)
	}
	if entry.ID == "" {
		t.Error("expected ID to be set after save")
	}

	entries, err := store.Recent(ctx, 1*time.Hour)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("recent count = %d, want 1", len(entries))
	}
	if entries[0].Summary != "Fixed a bug in the event store" {
		t.Errorf("summary = %q", entries[0].Summary)
	}
	if len(entries[0].Decisions) != 1 {
		t.Errorf("decisions = %v", entries[0].Decisions)
	}
	if entries[0].ToolCount != 3 {
		t.Errorf("toolCount = %d, want 3", entries[0].ToolCount)
	}
}

func TestJournalStore_RecentFiltersOld(t *testing.T) {
	sqlDB := setupTestDBForAgent(t)
	store := NewJournalStore(sqlDB)
	ctx := context.Background()

	// Save an old entry.
	old := &JournalEntry{
		SessionID: "old-session",
		Summary:   "Old session",
		Mode:      "talk",
		CreatedAt: time.Now().UTC().Add(-72 * time.Hour),
	}
	store.Save(ctx, old)

	// Save a recent entry.
	recent := &JournalEntry{
		SessionID: "new-session",
		Summary:   "Recent session",
		Mode:      "work",
	}
	store.Save(ctx, recent)

	entries, err := store.Recent(ctx, 48*time.Hour)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("recent count = %d, want 1 (should filter old)", len(entries))
	}
}

// --- Reflection tests ---

func TestReflection_ParseResult(t *testing.T) {
	r := &ReflectionEngine{}

	raw := `{
		"memories": [
			{"content": "User prefers concise responses", "category": "preference", "confidence": 0.8},
			{"content": "Low confidence thing", "category": "fact", "confidence": 0.3}
		],
		"soulPatch": "Be concise."
	}`

	result := r.parseResult(raw)
	if len(result.Memories) != 1 {
		t.Errorf("memories = %d, want 1 (low-confidence filtered)", len(result.Memories))
	}
	if result.Memories[0].Content != "User prefers concise responses" {
		t.Errorf("memory content = %q", result.Memories[0].Content)
	}
	if result.SoulPatch != "Be concise." {
		t.Errorf("soulPatch = %q", result.SoulPatch)
	}
}

func TestReflection_ParseResultMarkdownFences(t *testing.T) {
	r := &ReflectionEngine{}

	raw := "```json\n{\"memories\": [], \"soulPatch\": \"\"}\n```"
	result := r.parseResult(raw)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestReflection_TooFewEntries(t *testing.T) {
	sqlDB := setupTestDBForAgent(t)
	journal := NewJournalStore(sqlDB)

	r := NewReflectionEngine(journal, nil, nil, nil, "", ReflectionConfig{})

	result, err := r.Reflect(context.Background())
	if err != nil {
		t.Fatalf("reflect: %v", err)
	}
	if len(result.Memories) != 0 {
		t.Errorf("expected no memories from too-few entries")
	}
}

// --- Loop tests ---

func TestLoop_StartAndClose(t *testing.T) {
	bus := eventbus.New()
	defer bus.Close()

	got := make(chan AgentHeartbeat, 10)
	eventbus.Subscribe(bus, func(e AgentHeartbeat) {
		got <- e
	})

	loop := NewLoop(LoopConfig{
		TickInterval: 50 * time.Millisecond,
	}, LoopDeps{
		Bus:    bus,
		Logger: slogDiscard(),
	})

	loop.Start()

	// Wait for at least one heartbeat.
	select {
	case hb := <-got:
		if hb.TickNumber < 1 {
			t.Errorf("tick = %d, want >= 1", hb.TickNumber)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for heartbeat")
	}

	loop.Close()

	if loop.TickCount() < 1 {
		t.Errorf("tickCount = %d, want >= 1", loop.TickCount())
	}
}

func TestBuildTranscript(t *testing.T) {
	session := &Session{
		Events: []*Event{
			{Author: "user", Parts: []Part{TextPart{Text: "Fix the bug"}}},
			{Author: "agent", Round: 1, Parts: []Part{
				TextPart{Text: "I'll look at the code"},
				ToolPart{ToolName: "readFile", Status: ToolCompleted},
			}},
			{Author: "agent", Round: 2, Parts: []Part{TextPart{Text: "Fixed it"}}},
		},
	}

	transcript := buildTranscript(session)
	if transcript == "" {
		t.Fatal("expected non-empty transcript")
	}
	if !contains(transcript, "Fix the bug") {
		t.Error("missing user message")
	}
	if !contains(transcript, "readFile") {
		t.Error("missing tool name")
	}
}

func TestParseJournalResult(t *testing.T) {
	session := &Session{
		ID:   "s1",
		Mode: "coding",
		Events: []*Event{
			{Round: 3, Parts: []Part{
				ToolPart{ToolName: "shell", Status: ToolCompleted},
				ToolPart{ToolName: "editFile", Status: ToolCompleted},
			}},
		},
	}

	raw := `{"summary":"Fixed bug","decisions":["used editFile"],"errors":[],"learnings":["tests pass"],"entities":["shell"]}`
	entry := parseJournalResult(raw, session, 5*time.Second)

	if entry.Summary != "Fixed bug" {
		t.Errorf("summary = %q", entry.Summary)
	}
	if entry.ToolCount != 2 {
		t.Errorf("toolCount = %d, want 2", entry.ToolCount)
	}
	if entry.RoundCount != 3 {
		t.Errorf("roundCount = %d, want 3", entry.RoundCount)
	}
	if entry.DurationMs != 5000 {
		t.Errorf("durationMs = %d, want 5000", entry.DurationMs)
	}
}

// helpers

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsString(s, substr))
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func slogDiscard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
