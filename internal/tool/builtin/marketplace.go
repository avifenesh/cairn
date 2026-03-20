package builtin

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/avifenesh/cairn/internal/skill"
	"github.com/avifenesh/cairn/internal/tool"
)

var (
	mpClient *skill.MarketplaceClient
	mpOnce   sync.Once
)

// SetMarketplaceConfig initializes the marketplace client for agent tools.
func SetMarketplaceConfig(baseURL string, logger *slog.Logger) {
	mpOnce = sync.Once{} // allow re-init for tests
	mpOnce.Do(func() {
		mpClient = skill.NewMarketplaceClient(baseURL, logger)
	})
}

func getMarketplaceClient() *skill.MarketplaceClient {
	if mpClient == nil {
		// Lazy init with defaults if not explicitly configured.
		mpClient = skill.NewMarketplaceClient("", slog.Default())
	}
	return mpClient
}

// --- cairn.searchSkills ---

type searchSkillsParams struct {
	Query string `json:"query" desc:"Search query for ClawHub skill marketplace"`
	Limit int    `json:"limit" desc:"Max results (default 10, max 20)"`
}

var searchSkillsMarketplace = tool.Define("cairn.searchSkills",
	"Search ClawHub marketplace for skills. Returns matching skills with name, description, version, downloads, and stars.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p searchSkillsParams) (*tool.ToolResult, error) {
		if p.Query == "" {
			return &tool.ToolResult{Error: "query is required"}, nil
		}
		if p.Limit <= 0 {
			p.Limit = 10
		}

		mc := getMarketplaceClient()
		results, err := mc.Search(ctx.Cancel, p.Query, p.Limit)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("search failed: %v", err)}, nil
		}

		if len(results) == 0 {
			return &tool.ToolResult{
				Output:   fmt.Sprintf("No skills found for %q on ClawHub.", p.Query),
				Metadata: map[string]any{"count": 0},
			}, nil
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Found %d skills on ClawHub for %q:\n\n", len(results), p.Query)
		for i, r := range results {
			fmt.Fprintf(&sb, "%d. **%s** (`%s`) v%s\n", i+1, r.DisplayName, r.Slug, r.Version)
			fmt.Fprintf(&sb, "   %s\n", r.Summary)
			fmt.Fprintf(&sb, "   Score: %.1f\n\n", r.Score)
		}
		sb.WriteString("Use `cairn.skillInfo` to preview a skill or `cairn.installSkill` to install one.")

		return &tool.ToolResult{
			Output:   sb.String(),
			Metadata: map[string]any{"count": len(results), "source": "clawhub"},
		}, nil
	},
)

// --- cairn.skillInfo ---

type skillInfoParams struct {
	Slug string `json:"slug" desc:"ClawHub skill slug (e.g. 'git-essentials')"`
}

var skillInfoMarketplace = tool.Define("cairn.skillInfo",
	"Get detailed information about a ClawHub marketplace skill, including stats, author, and SKILL.md preview.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p skillInfoParams) (*tool.ToolResult, error) {
		if p.Slug == "" {
			return &tool.ToolResult{Error: "slug is required"}, nil
		}

		mc := getMarketplaceClient()

		detail, err := mc.Detail(ctx.Cancel, p.Slug)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("detail failed: %v", err)}, nil
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "# %s (`%s`)\n\n", detail.DisplayName, detail.Slug)
		fmt.Fprintf(&sb, "%s\n\n", detail.Summary)
		fmt.Fprintf(&sb, "- **Version**: %s\n", detail.LatestVersion.Version)
		fmt.Fprintf(&sb, "- **Downloads**: %d\n", detail.Stats.Downloads)
		fmt.Fprintf(&sb, "- **Stars**: %d\n", detail.Stats.Stars)
		if detail.Owner.Handle != "" {
			fmt.Fprintf(&sb, "- **Author**: %s\n", detail.Owner.Handle)
		}

		// Try to fetch SKILL.md preview.
		preview, previewErr := mc.Preview(ctx.Cancel, p.Slug)
		if previewErr == nil && preview != "" {
			sb.WriteString("\n## SKILL.md Preview\n\n")
			if len(preview) > 2000 {
				sb.WriteString(preview[:2000])
				sb.WriteString("\n\n... (truncated)")
			} else {
				sb.WriteString(preview)
			}
		}

		// Check if already installed locally.
		installed := false
		if ctx.Skills != nil {
			if existing := ctx.Skills.Get(p.Slug); existing != nil {
				installed = true
				sb.WriteString("\n\n> This skill is already installed locally.")
			}
		}

		return &tool.ToolResult{
			Output: sb.String(),
			Metadata: map[string]any{
				"slug":      detail.Slug,
				"downloads": detail.Stats.Downloads,
				"stars":     detail.Stats.Stars,
				"version":   detail.LatestVersion.Version,
				"installed": installed,
			},
		}, nil
	},
)

// --- cairn.installSkill ---

type installSkillParams struct {
	Slug string `json:"slug" desc:"ClawHub skill slug to install (e.g. 'git-essentials')"`
}

var installSkillMarketplace = tool.Define("cairn.installSkill",
	"Install a skill from ClawHub marketplace. Downloads the skill package and makes it immediately available.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p installSkillParams) (*tool.ToolResult, error) {
		if p.Slug == "" {
			return &tool.ToolResult{Error: "slug is required"}, nil
		}
		if ctx.Skills == nil {
			return &tool.ToolResult{Error: "skill service not available"}, nil
		}

		// Check for name collision.
		if existing := ctx.Skills.Get(p.Slug); existing != nil {
			return &tool.ToolResult{
				Error: fmt.Sprintf("skill %q already exists locally at %s", p.Slug, existing.Location),
			}, nil
		}

		targetDir := ctx.Skills.InstallDir()
		if targetDir == "" {
			return &tool.ToolResult{Error: "no skill install directory configured"}, nil
		}

		mc := getMarketplaceClient()
		prov, err := mc.Install(ctx.Cancel, p.Slug, targetDir)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("install failed: %v", err)}, nil
		}

		// Re-discover skills so the new one is immediately available.
		if refreshErr := ctx.Skills.Refresh(); refreshErr != nil {
			return &tool.ToolResult{
				Output: fmt.Sprintf("Skill %q installed (v%s) but re-discovery failed: %v. It will be available after the next poll cycle.", p.Slug, prov.Version, refreshErr),
			}, nil
		}

		return &tool.ToolResult{
			Output: fmt.Sprintf("Skill %q (v%s) installed from ClawHub and is now available.", p.Slug, prov.Version),
			Metadata: map[string]any{
				"slug":    prov.Slug,
				"version": prov.Version,
				"source":  "clawhub",
			},
		}, nil
	},
)
