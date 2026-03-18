package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/avifenesh/cairn/internal/tool"
)

// mockMemoryService implements tool.MemoryService for testing.
type mockMemoryService struct {
	memories map[string]*tool.MemoryItem
	nextID   int
}

func newMockMemoryService() *mockMemoryService {
	return &mockMemoryService{memories: make(map[string]*tool.MemoryItem)}
}

func (m *mockMemoryService) Create(_ context.Context, item *tool.MemoryItem) error {
	m.nextID++
	item.ID = fmt.Sprintf("mem_%d", m.nextID)
	item.Status = "proposed"
	m.memories[item.ID] = item
	return nil
}

func (m *mockMemoryService) Search(_ context.Context, query string, limit int) ([]tool.MemorySearchResult, error) {
	var results []tool.MemorySearchResult
	for _, mem := range m.memories {
		if limit > 0 && len(results) >= limit {
			break
		}
		results = append(results, tool.MemorySearchResult{Memory: mem, Score: 0.85})
	}
	return results, nil
}

func (m *mockMemoryService) Get(_ context.Context, id string) (*tool.MemoryItem, error) {
	item, ok := m.memories[id]
	if !ok {
		return nil, fmt.Errorf("not found: %s", id)
	}
	return item, nil
}

func (m *mockMemoryService) Accept(_ context.Context, id string) error {
	item, ok := m.memories[id]
	if !ok {
		return fmt.Errorf("not found: %s", id)
	}
	item.Status = "accepted"
	return nil
}

func (m *mockMemoryService) Reject(_ context.Context, id string) error {
	item, ok := m.memories[id]
	if !ok {
		return fmt.Errorf("not found: %s", id)
	}
	item.Status = "rejected"
	return nil
}

func (m *mockMemoryService) Delete(_ context.Context, id string) error {
	if _, ok := m.memories[id]; !ok {
		return fmt.Errorf("not found: %s", id)
	}
	delete(m.memories, id)
	return nil
}

func toolCtxWithMemory(svc tool.MemoryService) *tool.ToolContext {
	return &tool.ToolContext{
		SessionID: "test",
		AgentMode: tool.ModeTalk,
		Cancel:    context.Background(),
		Memories:  svc,
	}
}

func TestCreateMemory(t *testing.T) {
	svc := newMockMemoryService()
	ctx := toolCtxWithMemory(svc)

	args, _ := json.Marshal(map[string]string{
		"content":  "Go 1.25 was released in 2026",
		"category": "fact",
	})

	result, err := createMemory.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if result.Metadata["id"] == nil {
		t.Fatal("expected memory ID in metadata")
	}
	if len(svc.memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(svc.memories))
	}
}

func TestCreateMemoryInvalidCategory(t *testing.T) {
	ctx := toolCtxWithMemory(newMockMemoryService())
	args, _ := json.Marshal(map[string]string{
		"content":  "test",
		"category": "invalid",
	})

	result, err := createMemory.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error for invalid category")
	}
}

func TestCreateMemoryNoService(t *testing.T) {
	ctx := &tool.ToolContext{Cancel: context.Background()}
	args, _ := json.Marshal(map[string]string{"content": "test"})

	result, err := createMemory.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error when service is nil")
	}
}

func TestSearchMemory(t *testing.T) {
	svc := newMockMemoryService()
	svc.memories["mem_1"] = &tool.MemoryItem{
		ID: "mem_1", Content: "Go is great", Category: "fact", Status: "accepted",
	}
	ctx := toolCtxWithMemory(svc)

	args, _ := json.Marshal(map[string]string{"query": "Go"})
	result, err := searchMemory.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if result.Metadata["count"].(int) != 1 {
		t.Fatalf("expected 1 result, got %v", result.Metadata["count"])
	}
}

func TestSearchMemoryEmpty(t *testing.T) {
	ctx := toolCtxWithMemory(newMockMemoryService())
	args, _ := json.Marshal(map[string]string{"query": "nothing"})

	result, err := searchMemory.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "No memories found." {
		t.Fatalf("expected no-results message, got: %s", result.Output)
	}
}

func TestManageMemoryAccept(t *testing.T) {
	svc := newMockMemoryService()
	svc.memories["mem_1"] = &tool.MemoryItem{ID: "mem_1", Status: "proposed"}
	ctx := toolCtxWithMemory(svc)

	args, _ := json.Marshal(map[string]string{"id": "mem_1", "action": "accept"})
	result, err := manageMemory.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if svc.memories["mem_1"].Status != "accepted" {
		t.Fatalf("expected accepted, got %s", svc.memories["mem_1"].Status)
	}
}

func TestManageMemoryDelete(t *testing.T) {
	svc := newMockMemoryService()
	svc.memories["mem_1"] = &tool.MemoryItem{ID: "mem_1"}
	ctx := toolCtxWithMemory(svc)

	args, _ := json.Marshal(map[string]string{"id": "mem_1", "action": "delete"})
	result, err := manageMemory.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if len(svc.memories) != 0 {
		t.Fatal("expected memory to be deleted")
	}
}

func TestManageMemoryInvalidAction(t *testing.T) {
	ctx := toolCtxWithMemory(newMockMemoryService())
	args, _ := json.Marshal(map[string]string{"id": "mem_1", "action": "invalid"})

	result, err := manageMemory.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error for invalid action")
	}
}
