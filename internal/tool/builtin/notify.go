package builtin

import (
	"fmt"
	"strings"

	"github.com/avifenesh/cairn/internal/tool"
)

type notifyParams struct {
	Message  string  `json:"message" desc:"Notification text (markdown)"`
	Priority *string `json:"priority" desc:"Priority: low, medium, high, critical (default: medium)"`
	Channel  *string `json:"channel" desc:"Target channel: telegram, discord, slack, broadcast. Omit for default routing."`
	Action   *string `json:"action" desc:"Optional action: 'flush' to send queued digest immediately"`
}

var notify = tool.Define("cairn.notify",
	"Send a notification to configured channels. Routes based on priority and quiet hours. Use action='flush' to deliver queued digest. Use channel param to target a specific channel (telegram, discord, slack) or broadcast to all.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p notifyParams) (*tool.ToolResult, error) {
		if ctx.Notifier == nil {
			return &tool.ToolResult{Error: "notification service not configured"}, nil
		}

		// Handle flush action.
		if p.Action != nil && strings.EqualFold(*p.Action, "flush") {
			count := ctx.Notifier.FlushDigest(ctx.Cancel)
			if count == 0 {
				return &tool.ToolResult{Output: "Digest queue is empty, nothing to flush."}, nil
			}
			return &tool.ToolResult{
				Output:   fmt.Sprintf("Flushed %d queued notifications as digest.", count),
				Metadata: map[string]any{"flushed": count},
			}, nil
		}

		if strings.TrimSpace(p.Message) == "" {
			return &tool.ToolResult{Error: "message is required"}, nil
		}

		// Parse priority.
		priority := 1 // default: medium
		priStr := ""
		if p.Priority != nil {
			priStr = *p.Priority
		}
		switch strings.ToLower(priStr) {
		case "low", "0":
			priority = 0
		case "medium", "1", "":
			priority = 1
		case "high", "2":
			priority = 2
		case "critical", "3":
			priority = 3
		default:
			return &tool.ToolResult{Error: fmt.Sprintf("unknown priority %q (use: low, medium, high, critical)", priStr)}, nil
		}

		// Validate channel if specified.
		if p.Channel != nil {
			ch := strings.ToLower(*p.Channel)
			switch ch {
			case "telegram", "discord", "slack":
				// Valid single-channel target.
				ctx.Notifier.SendToChannel(ctx.Cancel, ch, p.Message, priority)
				labels := []string{"low", "medium", "high", "critical"}
				return &tool.ToolResult{
					Output: fmt.Sprintf("Notification sent to %s (priority: %s): %s", ch, labels[priority], truncateStr(p.Message, 100)),
					Metadata: map[string]any{
						"priority": labels[priority],
						"channel":  ch,
					},
				}, nil
			case "broadcast":
				// Force broadcast regardless of priority routing.
				priority = 3 // critical triggers broadcast
				ctx.Notifier.Notify(ctx.Cancel, p.Message, priority)
				return &tool.ToolResult{
					Output:   fmt.Sprintf("Notification broadcast to all channels (priority: critical): %s", truncateStr(p.Message, 100)),
					Metadata: map[string]any{"priority": "critical", "channel": "broadcast"},
				}, nil
			default:
				return &tool.ToolResult{Error: fmt.Sprintf("unknown channel %q (use: telegram, discord, slack, broadcast)", *p.Channel)}, nil
			}
		}

		// Default routing (no channel specified).
		ctx.Notifier.Notify(ctx.Cancel, p.Message, priority)

		labels := []string{"low", "medium", "high", "critical"}
		action := "sent"
		if priority == 0 {
			action = "queued for digest"
		}
		return &tool.ToolResult{
			Output: fmt.Sprintf("Notification %s (priority: %s): %s", action, labels[priority], truncateStr(p.Message, 100)),
			Metadata: map[string]any{
				"priority":    labels[priority],
				"digestQueue": ctx.Notifier.DigestLen(),
			},
		}, nil
	},
)

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
