package builtin

import (
	"fmt"
	"strings"

	"github.com/avifenesh/cairn/internal/tool"
)

// createMemoryParams are the parameters for cairn.createMemory.
type createMemoryParams struct {
	Content  string `json:"content" desc:"The knowledge to remember"`
	Category string `json:"category" desc:"Category: fact, preference, hard_rule, decision, or writing_style"`
	Scope    string `json:"scope,omitempty" desc:"Scope: personal, project, or global (default: global)"`
}

var createMemory = tool.Define("cairn.createMemory",
	"Create a new memory. The memory is proposed and can be accepted or rejected later.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p createMemoryParams) (*tool.ToolResult, error) {
		if ctx.Memories == nil {
			return &tool.ToolResult{Error: "memory service not available"}, nil
		}
		if p.Content == "" {
			return &tool.ToolResult{Error: "content is required"}, nil
		}

		validCategories := map[string]bool{
			"fact": true, "preference": true, "hard_rule": true,
			"decision": true, "writing_style": true,
		}
		if p.Category == "" {
			p.Category = "fact"
		}
		if !validCategories[p.Category] {
			return &tool.ToolResult{Error: fmt.Sprintf("invalid category %q, must be one of: fact, preference, hard_rule, decision, writing_style", p.Category)}, nil
		}

		if p.Scope == "" {
			p.Scope = "global"
		}
		validScopes := map[string]bool{"personal": true, "project": true, "global": true}
		if !validScopes[p.Scope] {
			return &tool.ToolResult{Error: fmt.Sprintf("invalid scope %q, must be one of: personal, project, global", p.Scope)}, nil
		}

		m := &tool.MemoryItem{
			Content:  p.Content,
			Category: p.Category,
			Scope:    p.Scope,
			Source:   "agent",
		}

		if err := ctx.Memories.Create(ctx.Cancel, m); err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to create memory: %v", err)}, nil
		}

		return &tool.ToolResult{
			Output: fmt.Sprintf("Memory created (id: %s, status: proposed, category: %s)", m.ID, p.Category),
			Metadata: map[string]any{
				"id":       m.ID,
				"category": p.Category,
				"scope":    p.Scope,
			},
		}, nil
	},
)

// searchMemoryParams are the parameters for cairn.searchMemory.
type searchMemoryParams struct {
	Query string `json:"query" desc:"Search query for memories"`
	Limit *int   `json:"limit,omitempty" desc:"Maximum results to return (default 10)"`
}

var searchMemory = tool.Define("cairn.searchMemory",
	"Search memories by keyword or semantic similarity. Returns memories with relevance scores.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p searchMemoryParams) (*tool.ToolResult, error) {
		if ctx.Memories == nil {
			return &tool.ToolResult{Error: "memory service not available"}, nil
		}
		if p.Query == "" {
			return &tool.ToolResult{Error: "query is required"}, nil
		}

		limit := 10
		if p.Limit != nil && *p.Limit > 0 {
			limit = *p.Limit
		}

		results, err := ctx.Memories.Search(ctx.Cancel, p.Query, limit)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("search failed: %v", err)}, nil
		}

		if len(results) == 0 {
			return &tool.ToolResult{Output: "No memories found."}, nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Found %d memories:\n\n", len(results))
		for i, r := range results {
			fmt.Fprintf(&b, "%d. [%s] (score: %.2f, id: %s)\n   %s\n\n",
				i+1, r.Memory.Category, r.Score, r.Memory.ID, r.Memory.Content)
		}

		return &tool.ToolResult{
			Output:   b.String(),
			Metadata: map[string]any{"count": len(results)},
		}, nil
	},
)

// manageMemoryParams are the parameters for cairn.manageMemory.
type manageMemoryParams struct {
	ID     string `json:"id" desc:"Memory ID to manage"`
	Action string `json:"action" desc:"Action: accept, reject, or delete"`
}

var manageMemory = tool.Define("cairn.manageMemory",
	"Manage a memory: accept, reject, or delete it.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p manageMemoryParams) (*tool.ToolResult, error) {
		if ctx.Memories == nil {
			return &tool.ToolResult{Error: "memory service not available"}, nil
		}
		if p.ID == "" {
			return &tool.ToolResult{Error: "id is required"}, nil
		}

		var err error
		switch p.Action {
		case "accept":
			err = ctx.Memories.Accept(ctx.Cancel, p.ID)
		case "reject":
			err = ctx.Memories.Reject(ctx.Cancel, p.ID)
		case "delete":
			err = ctx.Memories.Delete(ctx.Cancel, p.ID)
		default:
			return &tool.ToolResult{Error: fmt.Sprintf("invalid action %q, must be one of: accept, reject, delete", p.Action)}, nil
		}

		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to %s memory: %v", p.Action, err)}, nil
		}

		pastTense := map[string]string{"accept": "accepted", "reject": "rejected", "delete": "deleted"}
		return &tool.ToolResult{
			Output: fmt.Sprintf("Memory %s: %s", p.ID, pastTense[p.Action]),
			Metadata: map[string]any{
				"id":     p.ID,
				"action": p.Action,
			},
		}, nil
	},
)
