package builtin

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/avifenesh/cairn/internal/tool"
)

// mockEventService implements tool.EventService for testing.
type mockEventService struct {
	events   []*tool.StoredEvent
	markRead map[string]bool
}

func newMockEventService() *mockEventService {
	return &mockEventService{markRead: make(map[string]bool)}
}

func (m *mockEventService) List(_ context.Context, f tool.EventFilter) ([]*tool.StoredEvent, error) {
	var result []*tool.StoredEvent
	for _, ev := range m.events {
		if f.Source != "" && ev.Source != f.Source {
			continue
		}
		if f.UnreadOnly && ev.ReadAt != nil {
			continue
		}
		result = append(result, ev)
		if f.Limit > 0 && len(result) >= f.Limit {
			break
		}
	}
	return result, nil
}

func (m *mockEventService) MarkRead(_ context.Context, id string) error {
	m.markRead[id] = true
	return nil
}

func (m *mockEventService) Ingest(_ context.Context, events []*tool.IngestEvent) ([]*tool.IngestEvent, error) {
	return events, nil
}

func (m *mockEventService) MarkAllRead(_ context.Context) (int, error) {
	count := 0
	for _, ev := range m.events {
		if ev.ReadAt == nil {
			count++
		}
	}
	return count, nil
}

func (m *mockEventService) Count(_ context.Context, f tool.EventFilter) (int, error) {
	count := 0
	for _, ev := range m.events {
		if f.Source != "" && ev.Source != f.Source {
			continue
		}
		if f.UnreadOnly && ev.ReadAt != nil {
			continue
		}
		count++
	}
	return count, nil
}

func (m *mockEventService) Archive(_ context.Context, id string) error {
	return nil
}

func (m *mockEventService) DeleteByID(_ context.Context, id string) error {
	return nil
}

func (m *mockEventService) CountBySource(_ context.Context) (map[string]int, error) {
	result := map[string]int{}
	for _, ev := range m.events {
		result[ev.Source]++
	}
	return result, nil
}

func toolCtxWithEvents(svc tool.EventService) *tool.ToolContext {
	return &tool.ToolContext{
		SessionID: "test",
		AgentMode: tool.ModeTalk,
		Cancel:    context.Background(),
		Events:    svc,
	}
}

func TestReadFeed(t *testing.T) {
	svc := newMockEventService()
	svc.events = []*tool.StoredEvent{
		{ID: "ev1", Source: "github", Kind: "pr", Title: "Fix bug #123", CreatedAt: time.Now()},
		{ID: "ev2", Source: "hn", Kind: "story", Title: "Go 1.25 released", CreatedAt: time.Now()},
	}
	ctx := toolCtxWithEvents(svc)

	args, _ := json.Marshal(map[string]any{})
	result, err := readFeed.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if result.Metadata["count"].(int) != 2 {
		t.Fatalf("expected 2 events, got %v", result.Metadata["count"])
	}
}

func TestReadFeedWithSourceFilter(t *testing.T) {
	svc := newMockEventService()
	svc.events = []*tool.StoredEvent{
		{ID: "ev1", Source: "github", Kind: "pr", Title: "PR #1", CreatedAt: time.Now()},
		{ID: "ev2", Source: "hn", Kind: "story", Title: "Story", CreatedAt: time.Now()},
	}
	ctx := toolCtxWithEvents(svc)

	args, _ := json.Marshal(map[string]string{"source": "github"})
	result, err := readFeed.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Metadata["count"].(int) != 1 {
		t.Fatalf("expected 1 event, got %v", result.Metadata["count"])
	}
}

func TestReadFeedEmpty(t *testing.T) {
	ctx := toolCtxWithEvents(newMockEventService())
	args, _ := json.Marshal(map[string]any{})

	result, err := readFeed.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "No events found." {
		t.Fatalf("expected empty message, got: %s", result.Output)
	}
}

func TestReadFeedNoService(t *testing.T) {
	ctx := &tool.ToolContext{Cancel: context.Background()}
	args, _ := json.Marshal(map[string]any{})

	result, err := readFeed.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error when service is nil")
	}
}

func TestMarkReadSingle(t *testing.T) {
	svc := newMockEventService()
	ctx := toolCtxWithEvents(svc)

	args, _ := json.Marshal(map[string]string{"id": "ev1"})
	result, err := markRead.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if !svc.markRead["ev1"] {
		t.Fatal("expected ev1 to be marked read")
	}
}

func TestMarkReadAll(t *testing.T) {
	svc := newMockEventService()
	svc.events = []*tool.StoredEvent{
		{ID: "ev1", Source: "github"},
		{ID: "ev2", Source: "hn"},
	}
	ctx := toolCtxWithEvents(svc)

	args, _ := json.Marshal(map[string]string{"id": "all"})
	result, err := markRead.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Metadata["count"].(int) != 2 {
		t.Fatalf("expected 2, got %v", result.Metadata["count"])
	}
}

// mockDigestService implements tool.DigestService for testing.
type mockDigestService struct {
	result *tool.DigestResult
}

func (m *mockDigestService) Generate(_ context.Context) (*tool.DigestResult, error) {
	return m.result, nil
}

func TestDigest(t *testing.T) {
	svc := &mockDigestService{
		result: &tool.DigestResult{
			Summary:    "2 events from 2 sources.",
			Highlights: []string{"Important PR merged"},
			EventCount: 2,
		},
	}
	ctx := &tool.ToolContext{
		Cancel: context.Background(),
		Digest: svc,
	}

	result, err := digest.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if result.Metadata["eventCount"].(int) != 2 {
		t.Fatalf("expected 2, got %v", result.Metadata["eventCount"])
	}
}

func TestDigestNoService(t *testing.T) {
	ctx := &tool.ToolContext{Cancel: context.Background()}
	result, err := digest.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error when service is nil")
	}
}
