package builtin

import (
	"fmt"
	"strings"

	"github.com/avifenesh/cairn/internal/tool"
)

// createTaskParams are the parameters for cairn.createTask.
type createTaskParams struct {
	Description string  `json:"description" desc:"What the task should accomplish"`
	Type        *string `json:"type,omitempty" desc:"Task type: chat, coding, digest, triage, workflow (default: chat)"`
	Priority    *int    `json:"priority,omitempty" desc:"Priority 0-9, lower is higher priority (default: 2)"`
}

var createTask = tool.Define("cairn.createTask",
	"Create a new task for the agent to work on.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p createTaskParams) (*tool.ToolResult, error) {
		if ctx.Tasks == nil {
			return &tool.ToolResult{Error: "task service not available"}, nil
		}
		if p.Description == "" {
			return &tool.ToolResult{Error: "description is required"}, nil
		}

		taskType := "chat"
		if p.Type != nil && *p.Type != "" {
			validTypes := map[string]bool{
				"chat": true, "coding": true, "digest": true,
				"triage": true, "workflow": true,
			}
			if !validTypes[*p.Type] {
				return &tool.ToolResult{Error: fmt.Sprintf("invalid type %q, must be one of: chat, coding, digest, triage, workflow", *p.Type)}, nil
			}
			taskType = *p.Type
		}

		priority := 2
		if p.Priority != nil {
			if *p.Priority < 0 || *p.Priority > 9 {
				return &tool.ToolResult{Error: "priority must be 0-9"}, nil
			}
			priority = *p.Priority
		}

		t, err := ctx.Tasks.Submit(ctx.Cancel, &tool.TaskSubmitRequest{
			Description: p.Description,
			Type:        taskType,
			Priority:    priority,
		})
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to create task: %v", err)}, nil
		}

		return &tool.ToolResult{
			Output: fmt.Sprintf("Task created (id: %s, type: %s, priority: %d)", t.ID, t.Type, t.Priority),
			Metadata: map[string]any{
				"id":       t.ID,
				"type":     t.Type,
				"priority": t.Priority,
			},
		}, nil
	},
)

// listTasksParams are the parameters for cairn.listTasks.
type listTasksParams struct {
	Status *string `json:"status,omitempty" desc:"Filter by status: queued, claimed, running, completed, failed, canceled"`
	Type   *string `json:"type,omitempty" desc:"Filter by type: chat, coding, digest, triage, workflow"`
	Limit  *int    `json:"limit,omitempty" desc:"Maximum tasks to return (default 10)"`
}

var listTasks = tool.Define("cairn.listTasks",
	"List tasks with optional status and type filters.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p listTasksParams) (*tool.ToolResult, error) {
		if ctx.Tasks == nil {
			return &tool.ToolResult{Error: "task service not available"}, nil
		}

		limit := 10
		if p.Limit != nil && *p.Limit > 0 {
			limit = *p.Limit
		}

		status := ""
		if p.Status != nil {
			status = *p.Status
		}
		taskType := ""
		if p.Type != nil {
			taskType = *p.Type
		}
		tasks, err := ctx.Tasks.List(ctx.Cancel, status, taskType, limit)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to list tasks: %v", err)}, nil
		}

		if len(tasks) == 0 {
			return &tool.ToolResult{Output: "No tasks found."}, nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Tasks: %d\n\n", len(tasks))
		for i, t := range tasks {
			fmt.Fprintf(&b, "%d. [%s] %s (id: %s, priority: %d)\n",
				i+1, t.Status, t.Description, t.ID, t.Priority)
			if t.Error != "" {
				fmt.Fprintf(&b, "   error: %s\n", t.Error)
			}
			fmt.Fprintf(&b, "   created: %s\n\n", t.CreatedAt.Format(displayTimeFormat))
		}

		return &tool.ToolResult{
			Output:   b.String(),
			Metadata: map[string]any{"count": len(tasks)},
		}, nil
	},
)

// completeTaskParams are the parameters for cairn.completeTask.
type completeTaskParams struct {
	ID     *string `json:"id" desc:"Task ID to complete (optional — auto-detected from current task context)"`
	Output *string `json:"output,omitempty" desc:"Optional output or result message"`
}

var completeTask = tool.Define("cairn.completeTask",
	"Mark a task as completed. In most cases you don't need to call this — the system auto-completes tasks when you finish responding. Only use for manually created tasks.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p completeTaskParams) (*tool.ToolResult, error) {
		if ctx.Tasks == nil {
			return &tool.ToolResult{Error: "task service not available"}, nil
		}

		// Resolve task ID: explicit param > context task ID.
		taskID := ""
		if p.ID != nil && *p.ID != "" {
			taskID = *p.ID
		} else if ctx.TaskID != "" {
			taskID = ctx.TaskID
		}
		if taskID == "" {
			return &tool.ToolResult{Error: "no task ID provided and no task context available — tasks are auto-completed when you finish responding, so you usually don't need to call this"}, nil
		}

		output := ""
		if p.Output != nil {
			output = *p.Output
		}
		if err := ctx.Tasks.Complete(ctx.Cancel, taskID, output); err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to complete task: %v", err)}, nil
		}

		return &tool.ToolResult{
			Output:   fmt.Sprintf("Task %s marked as completed.", taskID),
			Metadata: map[string]any{"id": taskID},
		}, nil
	},
)
