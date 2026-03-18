package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/avifenesh/cairn/internal/tool"
)

type mockTaskService struct {
	tasks  []*tool.TaskItem
	nextID int
}

func newMockTaskService() *mockTaskService {
	return &mockTaskService{}
}

func (m *mockTaskService) Submit(_ context.Context, req *tool.TaskSubmitRequest) (*tool.TaskItem, error) {
	m.nextID++
	t := &tool.TaskItem{
		ID:          fmt.Sprintf("task_%d", m.nextID),
		Type:        req.Type,
		Status:      "queued",
		Description: req.Description,
		Priority:    req.Priority,
		CreatedAt:   time.Now(),
	}
	m.tasks = append(m.tasks, t)
	return t, nil
}

func (m *mockTaskService) List(_ context.Context, status, taskType string, limit int) ([]*tool.TaskItem, error) {
	var result []*tool.TaskItem
	for _, t := range m.tasks {
		if status != "" && t.Status != status {
			continue
		}
		if taskType != "" && t.Type != taskType {
			continue
		}
		result = append(result, t)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (m *mockTaskService) Complete(_ context.Context, id string, _ string) error {
	for _, t := range m.tasks {
		if t.ID == id {
			t.Status = "completed"
			return nil
		}
	}
	return fmt.Errorf("not found: %s", id)
}

func toolCtxWithTasks(svc tool.TaskService) *tool.ToolContext {
	return &tool.ToolContext{
		SessionID: "test",
		AgentMode: tool.ModeTalk,
		Cancel:    context.Background(),
		Tasks:     svc,
	}
}

func TestCreateTask(t *testing.T) {
	svc := newMockTaskService()
	ctx := toolCtxWithTasks(svc)

	args, _ := json.Marshal(map[string]string{"description": "Fix the parser bug"})
	result, err := createTask.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if result.Metadata["id"] == nil {
		t.Fatal("expected task ID in metadata")
	}
	if len(svc.tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(svc.tasks))
	}
}

func TestCreateTaskInvalidType(t *testing.T) {
	ctx := toolCtxWithTasks(newMockTaskService())
	args, _ := json.Marshal(map[string]string{"description": "test", "type": "invalid"})

	result, err := createTask.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error for invalid type")
	}
}

func TestCreateTaskNoService(t *testing.T) {
	ctx := &tool.ToolContext{Cancel: context.Background()}
	args, _ := json.Marshal(map[string]string{"description": "test"})

	result, err := createTask.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error when service is nil")
	}
}

func TestListTasks(t *testing.T) {
	svc := newMockTaskService()
	svc.tasks = []*tool.TaskItem{
		{ID: "t1", Status: "queued", Description: "Task 1", CreatedAt: time.Now()},
		{ID: "t2", Status: "completed", Description: "Task 2", CreatedAt: time.Now()},
	}
	ctx := toolCtxWithTasks(svc)

	args, _ := json.Marshal(map[string]any{})
	result, err := listTasks.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Metadata["count"].(int) != 2 {
		t.Fatalf("expected 2 tasks, got %v", result.Metadata["count"])
	}
}

func TestListTasksWithFilter(t *testing.T) {
	svc := newMockTaskService()
	svc.tasks = []*tool.TaskItem{
		{ID: "t1", Status: "queued", Description: "Task 1", CreatedAt: time.Now()},
		{ID: "t2", Status: "completed", Description: "Task 2", CreatedAt: time.Now()},
	}
	ctx := toolCtxWithTasks(svc)

	args, _ := json.Marshal(map[string]string{"status": "queued"})
	result, err := listTasks.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Metadata["count"].(int) != 1 {
		t.Fatalf("expected 1 queued task, got %v", result.Metadata["count"])
	}
}

func TestListTasksEmpty(t *testing.T) {
	ctx := toolCtxWithTasks(newMockTaskService())
	args, _ := json.Marshal(map[string]any{})

	result, err := listTasks.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "No tasks found." {
		t.Fatalf("expected empty message, got: %s", result.Output)
	}
}

func TestCompleteTask(t *testing.T) {
	svc := newMockTaskService()
	svc.tasks = []*tool.TaskItem{{ID: "t1", Status: "running"}}
	ctx := toolCtxWithTasks(svc)

	args, _ := json.Marshal(map[string]string{"id": "t1", "output": "done"})
	result, err := completeTask.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected tool error: %s", result.Error)
	}
	if svc.tasks[0].Status != "completed" {
		t.Fatalf("expected completed, got %s", svc.tasks[0].Status)
	}
}

func TestCompleteTaskNotFound(t *testing.T) {
	ctx := toolCtxWithTasks(newMockTaskService())
	args, _ := json.Marshal(map[string]string{"id": "nonexistent"})

	result, err := completeTask.Execute(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error for nonexistent task")
	}
}
