package builtin

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/avifenesh/cairn/internal/tool"
)

// loadSkillParams are the parameters for cairn.loadSkill.
type loadSkillParams struct {
	Name string `json:"name" desc:"Skill name to load and activate for this session"`
}

var loadSkill = tool.Define("cairn.loadSkill",
	"Load and activate a skill. Injects the skill's instructions into the session and scopes available tools to the skill's allowed-tools.",
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

		// Enforce disable-model-invocation gate.
		if sk.DisableModel {
			return &tool.ToolResult{
				Error: fmt.Sprintf("skill %q requires approval (disable-model-invocation). Ask the user to approve activation.", p.Name),
			}, nil
		}

		// Activate the skill in the session (if callback is set).
		activated := false
		if ctx.ActivateSkill != nil {
			ctx.ActivateSkill(sk.Name, sk.Content, sk.AllowedTools)
			activated = true
		}

		// Build output with skill content and bundled files.
		var b strings.Builder
		fmt.Fprintf(&b, "<skill_content name=%q>\n", sk.Name)
		fmt.Fprintf(&b, "# Skill: %s\n\n", sk.Name)
		b.WriteString(strings.TrimSpace(sk.Content))
		b.WriteString("\n")

		// List bundled files in the skill directory.
		if sk.Location != "" {
			files := listSkillFiles(sk.Location, 10)
			if len(files) > 0 {
				b.WriteString("\n<skill_files>\n")
				for _, f := range files {
					fmt.Fprintf(&b, "  <file>%s</file>\n", f)
				}
				b.WriteString("</skill_files>\n")
			}
		}

		b.WriteString("</skill_content>")

		meta := map[string]any{
			"name":        sk.Name,
			"description": sk.Description,
			"inclusion":   sk.Inclusion,
			"activated":   activated,
		}
		if len(sk.AllowedTools) > 0 {
			meta["allowedTools"] = sk.AllowedTools
		}

		return &tool.ToolResult{
			Output:   b.String(),
			Metadata: meta,
		}, nil
	},
)

// listSkillFiles returns up to limit filenames in a skill directory, excluding SKILL.md.
// Returns relative names only (no full paths) to avoid leaking host directory structure.
func listSkillFiles(dir string, limit int) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.EqualFold(name, "SKILL.md") {
			continue
		}
		files = append(files, name)
		if len(files) >= limit {
			break
		}
	}
	return files
}

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

		// Sort by name for deterministic output.
		sort.Slice(skills, func(i, j int) bool {
			return skills[i].Name < skills[j].Name
		})

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
