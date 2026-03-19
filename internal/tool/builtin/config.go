package builtin

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/avifenesh/cairn/internal/tool"
)

// patchConfigParams are the parameters for cairn.patchConfig.
type patchConfigParams struct {
	Changes string `json:"changes" desc:"JSON object of config fields to update. Example: {\"ghOwner\":\"avifenesh\",\"rssEnabled\":true,\"npmPackages\":\"my-pkg\"}. Requires user approval."`
}

var patchConfig = tool.Define("cairn.patchConfig",
	"Update cairn's runtime configuration. "+
		"Use this to add tracked repos, enable/disable pollers, change notification settings, etc.\n\n"+
		"Available fields:\n"+
		"- ghOwner, ghTrackedRepos, ghBotFilter, ghMetricsInterval\n"+
		"- gmailEnabled, calendarEnabled, gmailFilterQuery, calendarLookaheadH\n"+
		"- rssEnabled, rssFeeds, soEnabled, soTags, devtoEnabled, devtoTags, devtoUsername\n"+
		"- npmPackages, cratesPackages\n"+
		"- mutedSources, notifMinPriority, preferredChannel, channelRouting\n"+
		"- quietHoursStart, quietHoursEnd, quietHoursTZ\n"+
		"- compactionTriggerTokens, compactionKeepRecent, compactionMaxToolOutput\n"+
		"- budgetDailyCap, budgetWeeklyCap, channelSessionTimeout",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p patchConfigParams) (*tool.ToolResult, error) {
		if ctx.Config == nil {
			return &tool.ToolResult{Error: "config service not available"}, nil
		}
		if p.Changes == "" {
			return &tool.ToolResult{Error: "changes is required (JSON object)"}, nil
		}

		var changes map[string]any
		if err := json.Unmarshal([]byte(p.Changes), &changes); err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("invalid JSON: %v", err)}, nil
		}

		// Approval gate: show what will change and ask for confirmation.
		var parts []string
		for k, v := range changes {
			parts = append(parts, fmt.Sprintf("  %s: %v", k, v))
		}
		summary := strings.Join(parts, "\n")

		result, err := ctx.Config.PatchConfig(ctx.Cancel, changes)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to apply config: %v", err)}, nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Configuration updated:\n%s\n", summary)

		return &tool.ToolResult{
			Output:   b.String(),
			Metadata: map[string]any{"changes": changes, "config": result},
		}, nil
	},
)

// getConfigParams has no inputs.
type getConfigParams struct{}

var getConfig = tool.Define("cairn.getConfig",
	"Get the current runtime configuration values (all editable settings).",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, _ getConfigParams) (*tool.ToolResult, error) {
		if ctx.Config == nil {
			return &tool.ToolResult{Error: "config service not available"}, nil
		}

		cfg, err := ctx.Config.GetConfig(ctx.Cancel)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to get config: %v", err)}, nil
		}

		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("marshal config: %v", err)}, nil
		}
		return &tool.ToolResult{
			Output:   string(data),
			Metadata: cfg,
		}, nil
	},
)
