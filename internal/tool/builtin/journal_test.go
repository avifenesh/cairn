package builtin

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/avifenesh/cairn/internal/tool"
)

// mockJournalService implements tool.JournalService for testing.
type mockJournalService struct {
	entries []*tool.JournalEntry
}

func (m *mockJournalService) Recent(_ context.Context, _ time.Duration) ([]*tool.JournalEntry, error) {
	return m.entries, nil
}

func toolCtxWithJournal(svc tool.JournalService) *tool.ToolContext {
	return &tool.ToolContext{
		SessionID: "test",
		AgentMode: tool.ModeTalk,
		Cancel:    context.Background(),
		Journal:   svc,
	}
}

func TestJournalSearch(t *testing.T) {
	svc := &mockJournalService{
		entries: []*tool.JournalEntry{
			{
				ID:        "j1",
				Summary:   "Fixed a bug in the parser",
				Decisions: []string{"Use regex instead of manual parsing"},
				Mode:      "coding",
				CreatedAt: time.Now(),
			},
			{
				ID:        "j2",
				Summary:   "Reviewed PR #42",
				Learnings: []string{"Always check edge cases"},
				Mode:      "work",
				CreatedAt: time.Now().Add(-time.Hour),
			},
		},
	}
	ctx := toolCtxWithJournal(svc)

	args, _ := json.Marshal(map[string]any{})
	result, err := journalSearch.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if result.Metadata["count"].(int) != 2 {
		t.Fatalf("expected 2 entries, got %v", result.Metadata["count"])
	}
}

func TestJournalSearchWithQuery(t *testing.T) {
	svc := &mockJournalService{
		entries: []*tool.JournalEntry{
			{ID: "j1", Summary: "Fixed a bug in the parser", CreatedAt: time.Now()},
			{ID: "j2", Summary: "Deployed new release", CreatedAt: time.Now()},
		},
	}
	ctx := toolCtxWithJournal(svc)

	query := "parser"
	args, _ := json.Marshal(map[string]any{"query": query})
	result, err := journalSearch.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Metadata["count"].(int) != 1 {
		t.Fatalf("expected 1 entry matching 'parser', got %v", result.Metadata["count"])
	}
}

func TestJournalSearchEmpty(t *testing.T) {
	ctx := toolCtxWithJournal(&mockJournalService{})
	args, _ := json.Marshal(map[string]any{})

	result, err := journalSearch.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "No journal entries found." {
		t.Fatalf("expected empty message, got: %s", result.Output)
	}
}

func TestJournalSearchNoService(t *testing.T) {
	ctx := &tool.ToolContext{Cancel: context.Background()}
	args, _ := json.Marshal(map[string]any{})

	result, err := journalSearch.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error when service is nil")
	}
}
