package builtin

import (
	"fmt"
	"strings"

	"github.com/avifenesh/cairn/internal/cron"
	"github.com/avifenesh/cairn/internal/tool"
)

type createCronParams struct {
	Name        string `json:"name" desc:"Unique name for the cron job (e.g. morning-email)"`
	Schedule    string `json:"schedule" desc:"5-field cron expression (e.g. '0 9 * * 1-5' for weekdays at 9am)"`
	Instruction string `json:"instruction" desc:"Natural language instruction for what the agent should do each time"`
	Priority    *int   `json:"priority" desc:"Task priority 0-4 (0=critical, 2=normal, 4=idle). Default: 3 (low)"`
}

var createCron = tool.Define("cairn.createCron",
	"Create a recurring scheduled task. The agent will execute the instruction on the specified cron schedule.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p createCronParams) (*tool.ToolResult, error) {
		if ctx.Crons == nil {
			return &tool.ToolResult{Error: "cron service not configured"}, nil
		}

		if strings.TrimSpace(p.Name) == "" {
			return &tool.ToolResult{Error: "name is required"}, nil
		}
		if strings.TrimSpace(p.Schedule) == "" {
			return &tool.ToolResult{Error: "schedule is required"}, nil
		}
		if strings.TrimSpace(p.Instruction) == "" {
			return &tool.ToolResult{Error: "instruction is required"}, nil
		}

		if err := cron.Validate(p.Schedule); err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("invalid schedule: %v", err)}, nil
		}

		priority := 3
		if p.Priority != nil {
			priority = *p.Priority
		}

		id, err := ctx.Crons.Create(ctx.Cancel, p.Name, p.Schedule, p.Instruction, priority)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to create cron job: %v", err)}, nil
		}

		return &tool.ToolResult{
			Output: fmt.Sprintf("Cron job %q created (ID: %s). Schedule: %s", p.Name, id, p.Schedule),
			Metadata: map[string]any{
				"id":       id,
				"name":     p.Name,
				"schedule": p.Schedule,
			},
		}, nil
	},
)

var listCrons = tool.Define("cairn.listCrons",
	"List all configured cron jobs with their schedules, next run times, and status.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p struct{}) (*tool.ToolResult, error) {
		if ctx.Crons == nil {
			return &tool.ToolResult{Error: "cron service not configured"}, nil
		}

		jobs, err := ctx.Crons.List(ctx.Cancel)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to list cron jobs: %v", err)}, nil
		}

		if len(jobs) == 0 {
			return &tool.ToolResult{Output: "No cron jobs configured."}, nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "## Cron Jobs (%d)\n\n", len(jobs))
		for _, j := range jobs {
			status := "enabled"
			if !j.Enabled {
				status = "disabled"
			}
			fmt.Fprintf(&b, "- **%s** [%s] `%s` — %s\n", j.Name, status, j.Schedule, j.Instruction)
			if j.NextRun != nil {
				fmt.Fprintf(&b, "  Next: %s", j.NextRun.Format("2006-01-02 15:04 UTC"))
			}
			if j.LastRun != nil {
				fmt.Fprintf(&b, " | Last: %s", j.LastRun.Format("2006-01-02 15:04 UTC"))
			}
			fmt.Fprintln(&b)
		}

		return &tool.ToolResult{
			Output:   b.String(),
			Metadata: map[string]any{"count": len(jobs)},
		}, nil
	},
)

type deleteCronParams struct {
	ID   *string `json:"id" desc:"Cron job ID to delete"`
	Name *string `json:"name" desc:"Cron job name to delete (alternative to ID)"`
}

var deleteCron = tool.Define("cairn.deleteCron",
	"Delete a cron job by ID or name. Removes the job and all its execution history.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p deleteCronParams) (*tool.ToolResult, error) {
		if ctx.Crons == nil {
			return &tool.ToolResult{Error: "cron service not configured"}, nil
		}

		target := ""
		if p.ID != nil && *p.ID != "" {
			target = *p.ID
		} else if p.Name != nil && *p.Name != "" {
			target = *p.Name
		} else {
			return &tool.ToolResult{Error: "id or name is required"}, nil
		}

		if err := ctx.Crons.Delete(ctx.Cancel, target); err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to delete cron job: %v", err)}, nil
		}

		return &tool.ToolResult{
			Output: fmt.Sprintf("Cron job %q deleted.", target),
		}, nil
	},
)
