package builtin

import (
	"fmt"

	"github.com/avifenesh/cairn/internal/tool"
)

type updateIdentityParams struct {
	Target  string `json:"target" desc:"Which identity file to update: soul, user, agents, or memory"`
	Content string `json:"content" desc:"Markdown content to add"`
	Reason  string `json:"reason,omitempty" desc:"Why this change is needed (included in patch proposal)"`
}

var updateIdentity = tool.Define("cairn.updateIdentity",
	`Propose or apply a change to one of the 4 identity files.

- soul: Who the agent IS (identity, voice, values). Changes require human approval.
- user: Who the user IS (preferences, style, patterns). Changes applied directly.
- agents: How agents OPERATE (rules, permissions, discipline). Changes require human approval.
- memory: Pinned knowledge the agent must always know (canonical facts, conventions). Changes require human approval.

For soul/agents/memory, this creates a pending patch visible in the Identity UI.
The human reviews and approves or denies.
For user, the content is appended directly (low risk, user reviews in UI).`,
	[]tool.Mode{tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p updateIdentityParams) (*tool.ToolResult, error) {
		if ctx.Identity == nil {
			return &tool.ToolResult{Error: "identity service not available"}, nil
		}
		if p.Content == "" {
			return &tool.ToolResult{Error: "content is required"}, nil
		}

		validTargets := map[string]bool{"soul": true, "user": true, "agents": true, "memory": true}
		if !validTargets[p.Target] {
			return &tool.ToolResult{
				Error: fmt.Sprintf("invalid target %q, must be: soul, user, agents, or memory", p.Target),
			}, nil
		}

		source := "agent"
		if p.Reason != "" {
			source = "agent: " + p.Reason
		}

		result, err := ctx.Identity.UpdateIdentity(ctx.Cancel, p.Target, p.Content, source)
		if err != nil {
			return &tool.ToolResult{Error: err.Error()}, nil
		}

		return &tool.ToolResult{Output: result}, nil
	},
)
