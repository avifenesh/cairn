package builtin

import (
	"encoding/json"
	"fmt"

	"github.com/avifenesh/cairn/internal/tool"
)

type spawnSubagentParams struct {
	Type        string  `json:"type"        desc:"Subagent type: researcher (read-only search), coder (worktree-isolated coding), reviewer (code analysis), executor (shell commands)"`
	Instruction string  `json:"instruction" desc:"What the subagent should accomplish. Include file paths, success criteria, and expected output format."`
	Context     *string `json:"context"     desc:"Optional summary of parent context to pass to the child agent"`
	ExecMode    *string `json:"execMode"    desc:"foreground (default, blocks until done) or background (returns immediately, check via cairn.listTasks)"`
	MaxRounds   *int    `json:"maxRounds"   desc:"Max reasoning rounds. 0 or omitted = type default (researcher:15, coder:50, reviewer:10, executor:10)"`
}

var spawnSubagent = tool.Define("cairn.spawnSubagent",
	"Spawn a child agent to handle a sub-task independently. The child runs in its own context "+
		"and returns a condensed summary. Types: researcher (read-only web/file search), "+
		"coder (implements code in isolated worktree), reviewer (analyzes code quality), "+
		"executor (runs shell commands). Children cannot spawn their own children.",
	[]tool.Mode{tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p spawnSubagentParams) (*tool.ToolResult, error) {
		if ctx.Subagents == nil {
			return &tool.ToolResult{Error: "subagent spawning not available in this context"}, nil
		}
		if p.Instruction == "" {
			return &tool.ToolResult{Error: "instruction is required"}, nil
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

		result, err := ctx.Subagents.Spawn(ctx.Cancel, ctx.TaskID, req)
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

		metadata := map[string]any{
			"subagentId": result.TaskID,
			"sessionId":  result.SessionID,
			"status":     result.Status,
			"type":       p.Type,
		}
		if p.ExecMode != nil {
			metadata["execMode"] = *p.ExecMode
		}

		// Include instruction in metadata for frontend SubagentCard rendering.
		metadata["instruction"] = p.Instruction

		raw, _ := json.Marshal(metadata)
		var metadataMap map[string]any
		json.Unmarshal(raw, &metadataMap)

		return &tool.ToolResult{
			Output:   out,
			Metadata: metadataMap,
		}, nil
	},
)
