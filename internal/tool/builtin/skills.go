package builtin

import (
	"fmt"
	"strings"

	"github.com/avifenesh/cairn/internal/tool"
)

// loadSkillParams are the parameters for cairn.loadSkill.
type loadSkillParams struct {
	Name string `json:"name" desc:"Skill name to load"`
}

var loadSkill = tool.Define("cairn.loadSkill",
	"Load a skill by name. Returns the skill content for context injection.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p loadSkillParams) (*tool.ToolResult, error) {
		if ctx.Skills == nil {
			return &tool.ToolResult{Error: "skill service not available"}, nil
		}
		if p.Name == "" {
			return &tool.ToolResult{Error: "name is required"}, nil
		}

		sk := ctx.Skills.Get(p.Name)
		if sk == nil {
			return &tool.ToolResult{Error: fmt.Sprintf("skill %q not found", p.Name)}, nil
		}

		return &tool.ToolResult{
			Output: sk.Content,
			Metadata: map[string]any{
				"name":        sk.Name,
				"description": sk.Description,
				"inclusion":   sk.Inclusion,
			},
		}, nil
	},
)

// listSkillsParams has no required inputs.
type listSkillsParams struct{}

var listSkills = tool.Define("cairn.listSkills",
	"List all available skills with their names, descriptions, and inclusion type.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, _ listSkillsParams) (*tool.ToolResult, error) {
		if ctx.Skills == nil {
			return &tool.ToolResult{Error: "skill service not available"}, nil
		}

		skills := ctx.Skills.List()
		if len(skills) == 0 {
			return &tool.ToolResult{Output: "No skills available."}, nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Skills: %d available\n\n", len(skills))
		for i, sk := range skills {
			fmt.Fprintf(&b, "%d. %s [%s]\n   %s\n\n",
				i+1, sk.Name, sk.Inclusion, sk.Description)
		}

		return &tool.ToolResult{
			Output:   b.String(),
			Metadata: map[string]any{"count": len(skills)},
		}, nil
	},
)
