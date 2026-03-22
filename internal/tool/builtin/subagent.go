package builtin

import (
	"fmt"

	"github.com/avifenesh/cairn/internal/tool"
)

type spawnSubagentParams struct {
	Type        string  `json:"type"        desc:"Subagent type: built-in types (researcher, coder, reviewer, executor) or custom AGENT.md types"`
	Instruction string  `json:"instruction" desc:"What the subagent should accomplish. Include file paths, success criteria, and expected output format."`
	Context     *string `json:"context"     desc:"Optional summary of parent context to pass to the child agent"`
	ExecMode    *string `json:"execMode"    desc:"foreground (default, blocks until done) or background (returns immediately, check via cairn.listTasks)"`
	MaxRounds   *int    `json:"maxRounds"   desc:"Max reasoning rounds. 0 or omitted = type default from AGENT.md"`
}

var spawnSubagent = tool.Define("cairn.spawnSubagent",
	"Spawn a child agent to handle a sub-task independently. The child runs in its own context "+
		"and returns a condensed summary. Built-in types: researcher (read-only search), "+
		"coder (worktree-isolated coding), reviewer (code quality), executor (shell commands). "+
		"Custom types can be defined via AGENT.md files. Children cannot spawn their own children.",
	[]tool.Mode{tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p spawnSubagentParams) (*tool.ToolResult, error) {
		if ctx.Subagents == nil {
			return &tool.ToolResult{Error: "subagent spawning not available in this context"}, nil
		}
		if p.Instruction == "" {
			return &tool.ToolResult{Error: "instruction is required"}, nil
		}

		// Use TaskID as parent reference; fall back to SessionID for HTTP chat path
		// where taskIDFromSession may return empty.
		parentID := ctx.TaskID
		if parentID == "" {
			parentID = ctx.SessionID
		}

		req := &tool.SubagentSpawnRequest{
			Type:        p.Type,
			Instruction: p.Instruction,
		}
		if p.Context != nil {
			req.Context = *p.Context
		}
		if p.ExecMode != nil {
			req.ExecMode = *p.ExecMode
		}
		if p.MaxRounds != nil {
			req.MaxRounds = *p.MaxRounds
		}

		result, err := ctx.Subagents.Spawn(ctx.Cancel, parentID, req)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("subagent spawn failed: %v", err)}, nil
		}

		// Format result for parent model.
		out := fmt.Sprintf("[subagent:%s] Status: %s", p.Type, result.Status)
		if result.Summary != "" {
			out += "\n\n" + result.Summary
		}
		if result.Error != "" {
			out += "\n\nError: " + result.Error
		}
		if result.Rounds > 0 {
			out += fmt.Sprintf("\n\n(%d rounds, %d tool calls, %dms)", result.Rounds, result.ToolCalls, result.DurationMs)
		}

		// Metadata is already map[string]any - no need for JSON round-trip.
		metadata := map[string]any{
			"subagentId":  result.TaskID,
			"sessionId":   result.SessionID,
			"status":      result.Status,
			"type":        p.Type,
			"instruction": p.Instruction,
		}
		if p.ExecMode != nil {
			metadata["execMode"] = *p.ExecMode
		}

		return &tool.ToolResult{
			Output:   out,
			Metadata: metadata,
		}, nil
	},
)
