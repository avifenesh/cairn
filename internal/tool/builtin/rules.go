package builtin

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/avifenesh/cairn/internal/rules"
	"github.com/avifenesh/cairn/internal/signal"
	"github.com/avifenesh/cairn/internal/tool"
)

// rulesEnabled tracks whether the automation rules engine is active.
var rulesEnabled atomic.Bool

// SetRulesEnabled enables/disables the rules tools.
func SetRulesEnabled(enabled bool) {
	rulesEnabled.Store(enabled)
}

// RulesEnabled returns true if the automation rules engine is configured.
func RulesEnabled() bool {
	return rulesEnabled.Load()
}

type createRuleParams struct {
	Name        string `json:"name" desc:"Unique name for the rule (e.g. notify-on-github-pr)"`
	Description string `json:"description" desc:"Human-readable description of what this rule does"`
	TriggerType string `json:"triggerType" desc:"Trigger type: 'event' (bus event) or 'cron' (scheduled)"`
	EventType   string `json:"eventType" desc:"Bus event type to match (e.g. EventIngested, TaskFailed). Required when triggerType=event"`
	Filter      string `json:"filter" desc:"JSON object of key=value pairs for pre-filtering events (e.g. {\"sourceType\":\"github\"})"`
	Schedule    string `json:"schedule" desc:"Cron expression for scheduled triggers (e.g. '0 9 * * *'). Required when triggerType=cron"`
	Condition   string `json:"condition" desc:"expr-lang expression evaluated against event data. Empty means always true"`
	Actions     string `json:"actions" desc:"JSON array of actions, each with type and params (e.g. [{\"type\":\"notify\",\"params\":{\"message\":\"New PR: {{.title}}\"}}])"`
	ThrottleMs  *int64 `json:"throttleMs" desc:"Minimum milliseconds between rule fires. Default: 0 (no throttle)"`
}

var createRule = tool.Define("cairn.createRule",
	"Create an automation rule. Rules fire when a trigger matches and condition is true, executing the specified actions.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p createRuleParams) (*tool.ToolResult, error) {
		if ctx.Rules == nil {
			return &tool.ToolResult{Error: "rules service not configured"}, nil
		}

		if strings.TrimSpace(p.Name) == "" {
			return &tool.ToolResult{Error: "name is required"}, nil
		}
		if strings.TrimSpace(p.TriggerType) == "" {
			return &tool.ToolResult{Error: "triggerType is required (event or cron)"}, nil
		}
		if strings.TrimSpace(p.Actions) == "" {
			return &tool.ToolResult{Error: "actions is required (JSON array)"}, nil
		}

		// Build trigger JSON from individual params using json.Marshal for safety.
		triggerObj := map[string]any{"type": p.TriggerType}
		if p.EventType != "" {
			triggerObj["eventType"] = p.EventType
		}
		if p.Filter != "" {
			var filterMap map[string]string
			if err := json.Unmarshal([]byte(p.Filter), &filterMap); err != nil {
				return &tool.ToolResult{Error: fmt.Sprintf("invalid filter JSON: %v", err)}, nil
			}
			triggerObj["filter"] = filterMap
		}
		if p.Schedule != "" {
			triggerObj["schedule"] = p.Schedule
		}
		triggerBytes, err := json.Marshal(triggerObj)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to build trigger: %v", err)}, nil
		}
		trigger := string(triggerBytes)

		var throttle int64
		if p.ThrottleMs != nil {
			throttle = *p.ThrottleMs
		}

		id, err := ctx.Rules.Create(ctx.Cancel, p.Name, p.Description, trigger, p.Condition, p.Actions, throttle)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to create rule: %v", err)}, nil
		}

		return &tool.ToolResult{
			Output: fmt.Sprintf("Rule %q created (ID: %s).", p.Name, id),
			Metadata: map[string]any{
				"id":   id,
				"name": p.Name,
			},
		}, nil
	},
)

var listRules = tool.Define("cairn.listRules",
	"List all automation rules with their status, triggers, and last fire time.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p struct{}) (*tool.ToolResult, error) {
		if ctx.Rules == nil {
			return &tool.ToolResult{Error: "rules service not configured"}, nil
		}

		output, err := ctx.Rules.List(ctx.Cancel)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to list rules: %v", err)}, nil
		}

		return &tool.ToolResult{Output: output}, nil
	},
)

type deleteRuleParams struct {
	ID   *string `json:"id" desc:"Rule ID to delete"`
	Name *string `json:"name" desc:"Rule name to delete (alternative to ID)"`
}

var deleteRule = tool.Define("cairn.deleteRule",
	"Delete an automation rule by ID or name.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p deleteRuleParams) (*tool.ToolResult, error) {
		if ctx.Rules == nil {
			return &tool.ToolResult{Error: "rules service not configured"}, nil
		}

		target := ""
		if p.ID != nil && *p.ID != "" {
			target = *p.ID
		} else if p.Name != nil && *p.Name != "" {
			target = *p.Name
		} else {
			return &tool.ToolResult{Error: "id or name is required"}, nil
		}

		if err := ctx.Rules.Delete(ctx.Cancel, target); err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to delete rule: %v", err)}, nil
		}

		return &tool.ToolResult{
			Output: fmt.Sprintf("Rule %q deleted.", target),
		}, nil
	},
)

type toggleRuleParams struct {
	ID      string `json:"id" desc:"Rule ID to toggle"`
	Enabled bool   `json:"enabled" desc:"Set to true to enable, false to disable"`
}

var toggleRule = tool.Define("cairn.toggleRule",
	"Enable or disable an automation rule.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p toggleRuleParams) (*tool.ToolResult, error) {
		if ctx.Rules == nil {
			return &tool.ToolResult{Error: "rules service not configured"}, nil
		}

		if strings.TrimSpace(p.ID) == "" {
			return &tool.ToolResult{Error: "id is required"}, nil
		}

		if err := ctx.Rules.Toggle(ctx.Cancel, p.ID, p.Enabled); err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to toggle rule: %v", err)}, nil
		}

		status := "disabled"
		if p.Enabled {
			status = "enabled"
		}
		return &tool.ToolResult{
			Output: fmt.Sprintf("Rule %s %s.", p.ID, status),
		}, nil
	},
)

// --- Source and template tools ---

var listSources = tool.Define("cairn.listSources",
	"List active signal sources with their event kinds and filterable fields.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork, tool.ModeCoding},
	func(_ *tool.ToolContext, _ struct{}) (*tool.ToolResult, error) {
		sources := signal.AllSourceInfo()
		if len(sources) == 0 {
			return &tool.ToolResult{Output: "No signal sources configured."}, nil
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "## Signal Sources (%d)\n\n", len(sources))
		for _, src := range sources {
			fmt.Fprintf(&sb, "- **%s** (%s): kinds=%s\n", src.Label, src.Name, strings.Join(src.Kinds, ", "))
		}
		return &tool.ToolResult{Output: sb.String()}, nil
	},
)

var listRuleTemplates = tool.Define("cairn.listRuleTemplates",
	"List available rule templates that can be instantiated with cairn.createRuleFromTemplate.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork, tool.ModeCoding},
	func(_ *tool.ToolContext, _ struct{}) (*tool.ToolResult, error) {
		templates := rules.ListTemplates()
		if len(templates) == 0 {
			return &tool.ToolResult{Output: "No rule templates available."}, nil
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "## Rule Templates (%d)\n\n", len(templates))
		for _, t := range templates {
			fmt.Fprintf(&sb, "- **%s** (id: `%s`, category: %s)", t.Name, t.ID, t.Category)
			if t.Source != "" {
				fmt.Fprintf(&sb, " [%s]", t.Source)
			}
			sb.WriteString("\n")
			fmt.Fprintf(&sb, "  %s\n", t.Description)
			if len(t.Params) > 0 {
				params := make([]string, 0, len(t.Params))
				for _, p := range t.Params {
					s := p.Key
					if p.Required {
						s += " (required)"
					}
					if p.Default != "" {
						s += fmt.Sprintf(" [default: %s]", p.Default)
					}
					params = append(params, s)
				}
				fmt.Fprintf(&sb, "  Params: %s\n", strings.Join(params, ", "))
			}
		}
		return &tool.ToolResult{Output: sb.String()}, nil
	},
)

type createRuleFromTemplateParams struct {
	TemplateID string            `json:"templateId" desc:"Template ID from cairn.listRuleTemplates (e.g. notify-github-pr)"`
	Params     map[string]string `json:"params" desc:"Template parameters as key-value pairs"`
}

var createRuleFromTemplate = tool.Define("cairn.createRuleFromTemplate",
	"Create a rule from a bundled template. Use cairn.listRuleTemplates to see available templates and their parameters.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p createRuleFromTemplateParams) (*tool.ToolResult, error) {
		if ctx.Rules == nil {
			return &tool.ToolResult{Error: "rules service not configured"}, nil
		}

		if strings.TrimSpace(p.TemplateID) == "" {
			return &tool.ToolResult{Error: "templateId is required"}, nil
		}

		rule, err := rules.Instantiate(p.TemplateID, p.Params)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("template instantiation failed: %v", err)}, nil
		}

		// Marshal trigger and actions to JSON for the RulesService.Create interface.
		triggerJSON, err := json.Marshal(rule.Trigger)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to marshal trigger: %v", err)}, nil
		}
		actionsJSON, err := json.Marshal(rule.Actions)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to marshal actions: %v", err)}, nil
		}

		id, err := ctx.Rules.Create(ctx.Cancel, rule.Name, rule.Description, string(triggerJSON), rule.Condition, string(actionsJSON), rule.ThrottleMs)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to create rule: %v", err)}, nil
		}

		return &tool.ToolResult{
			Output: fmt.Sprintf("Rule %q created from template %q (ID: %s).", rule.Name, p.TemplateID, id),
			Metadata: map[string]any{
				"id":         id,
				"name":       rule.Name,
				"templateId": p.TemplateID,
			},
		}, nil
	},
)
