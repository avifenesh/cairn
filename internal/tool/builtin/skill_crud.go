package builtin

import (
	"fmt"
	"strings"

	"github.com/avifenesh/cairn/internal/tool"
)

type createSkillParams struct {
	Name         string `json:"name" desc:"Skill name (lowercase, hyphens, e.g. 'my-skill')"`
	Description  string `json:"description" desc:"When to use this skill (trigger description)"`
	Content      string `json:"content" desc:"Skill body (Markdown instructions for the agent)"`
	Inclusion    string `json:"inclusion" desc:"'always' or 'on-demand' (default: on-demand)"`
	AllowedTools string `json:"allowedTools" desc:"Comma-separated tool names (e.g. 'cairn.shell,cairn.readFile'). Empty = all tools."`
}

var createSkill = tool.Define("cairn.createSkill",
	"Create a new skill. Skills are SKILL.md files that teach cairn workflows and scope tool access.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p createSkillParams) (*tool.ToolResult, error) {
		if ctx.Skills == nil {
			return &tool.ToolResult{Error: "skill service not available"}, nil
		}
		if p.Name == "" || p.Description == "" || p.Content == "" {
			return &tool.ToolResult{Error: "name, description, and content are required"}, nil
		}
		if p.Inclusion != "" && p.Inclusion != "always" && p.Inclusion != "on-demand" {
			return &tool.ToolResult{Error: "inclusion must be 'always' or 'on-demand'"}, nil
		}

		var tools []string
		if p.AllowedTools != "" {
			for _, t := range strings.Split(p.AllowedTools, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					tools = append(tools, t)
				}
			}
		}

		if err := ctx.Skills.Create(p.Name, p.Description, p.Content, p.Inclusion, tools); err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("create failed: %v", err)}, nil
		}

		return &tool.ToolResult{
			Output:   fmt.Sprintf("Skill %q created successfully.", p.Name),
			Metadata: map[string]any{"name": p.Name},
		}, nil
	},
)

type editSkillParams struct {
	Name         string `json:"name" desc:"Skill name to edit"`
	Description  string `json:"description" desc:"New description (empty = keep existing)"`
	Content      string `json:"content" desc:"New content (empty = keep existing)"`
	Inclusion    string `json:"inclusion" desc:"New inclusion type (empty = keep existing)"`
	AllowedTools string `json:"allowedTools" desc:"New allowed tools, comma-separated (empty = keep existing)"`
}

var editSkill = tool.Define("cairn.editSkill",
	"Edit an existing skill's description, content, inclusion type, or allowed tools.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p editSkillParams) (*tool.ToolResult, error) {
		if ctx.Skills == nil {
			return &tool.ToolResult{Error: "skill service not available"}, nil
		}
		if p.Name == "" {
			return &tool.ToolResult{Error: "name is required"}, nil
		}

		if p.Inclusion != "" && p.Inclusion != "always" && p.Inclusion != "on-demand" {
			return &tool.ToolResult{Error: "inclusion must be 'always' or 'on-demand'"}, nil
		}

		var tools []string
		if p.AllowedTools != "" {
			for _, t := range strings.Split(p.AllowedTools, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					tools = append(tools, t)
				}
			}
		}

		if err := ctx.Skills.Update(p.Name, p.Description, p.Content, p.Inclusion, tools); err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("edit failed: %v", err)}, nil
		}

		return &tool.ToolResult{
			Output:   fmt.Sprintf("Skill %q updated.", p.Name),
			Metadata: map[string]any{"name": p.Name},
		}, nil
	},
)

type deleteSkillParams struct {
	Name string `json:"name" desc:"Skill name to delete"`
}

var deleteSkill = tool.Define("cairn.deleteSkill",
	"Delete a skill permanently. Removes the skill directory and SKILL.md file.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p deleteSkillParams) (*tool.ToolResult, error) {
		if ctx.Skills == nil {
			return &tool.ToolResult{Error: "skill service not available"}, nil
		}
		if p.Name == "" {
			return &tool.ToolResult{Error: "name is required"}, nil
		}

		if err := ctx.Skills.Delete(p.Name); err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("delete failed: %v", err)}, nil
		}

		return &tool.ToolResult{
			Output:   fmt.Sprintf("Skill %q deleted.", p.Name),
			Metadata: map[string]any{"name": p.Name},
		}, nil
	},
)
