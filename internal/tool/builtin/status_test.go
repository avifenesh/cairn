package builtin

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/avifenesh/cairn/internal/tool"
)

type mockStatusService struct {
	status *tool.SystemStatus
}

func (m *mockStatusService) GetStatus(_ context.Context) (*tool.SystemStatus, error) {
	return m.status, nil
}

func TestGetStatus(t *testing.T) {
	svc := &mockStatusService{
		status: &tool.SystemStatus{
			Uptime:       "2h30m0s",
			ActiveTasks:  3,
			UnreadEvents: 15,
			MemoryCount:  42,
			PollerStatus: []tool.PollerInfo{
				{Source: "github", Active: true},
				{Source: "hn", Active: true},
			},
		},
	}
	ctx := &tool.ToolContext{Cancel: context.Background(), Status: svc}

	result, err := getStatus.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if result.Metadata["activeTasks"].(int) != 3 {
		t.Fatalf("expected 3, got %v", result.Metadata["activeTasks"])
	}
	if result.Metadata["unreadEvents"].(int) != 15 {
		t.Fatalf("expected 15, got %v", result.Metadata["unreadEvents"])
	}
}

func TestGetStatusNoService(t *testing.T) {
	ctx := &tool.ToolContext{Cancel: context.Background()}
	result, err := getStatus.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error when service is nil")
	}
}

// mockEventServiceWithIngest extends mockEventService with Ingest.
type mockEventServiceWithIngest struct {
	mockEventService
	ingested []*tool.IngestEvent
}

func (m *mockEventServiceWithIngest) Ingest(_ context.Context, events []*tool.IngestEvent) ([]*tool.IngestEvent, error) {
	m.ingested = append(m.ingested, events...)
	return events, nil
}

func TestCompose(t *testing.T) {
	svc := &mockEventServiceWithIngest{}
	ctx := &tool.ToolContext{
		Cancel: context.Background(),
		Events: svc,
	}

	args, _ := json.Marshal(map[string]string{
		"title":    "Daily update",
		"body":     "Everything is running smoothly.",
		"priority": "high",
	})

	result, err := compose.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if len(svc.ingested) != 1 {
		t.Fatalf("expected 1 ingested event, got %d", len(svc.ingested))
	}
	if svc.ingested[0].Title != "Daily update" {
		t.Fatalf("expected title 'Daily update', got %q", svc.ingested[0].Title)
	}
}

func TestComposeInvalidPriority(t *testing.T) {
	svc := &mockEventServiceWithIngest{}
	ctx := &tool.ToolContext{Cancel: context.Background(), Events: svc}

	args, _ := json.Marshal(map[string]string{
		"title": "test", "body": "test", "priority": "urgent",
	})
	result, err := compose.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error for invalid priority")
	}
}

func TestComposeMissingTitle(t *testing.T) {
	ctx := &tool.ToolContext{
		Cancel: context.Background(),
		Events: &mockEventServiceWithIngest{},
	}
	args, _ := json.Marshal(map[string]string{"body": "test"})

	result, err := compose.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error for missing title")
	}
}
