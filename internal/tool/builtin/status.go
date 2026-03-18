package builtin

import (
	"fmt"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/tool"
)

// composeParams are the parameters for cairn.compose.
type composeParams struct {
	Title    string `json:"title" desc:"Title of the message"`
	Body     string `json:"body" desc:"Body content of the message"`
	Priority string `json:"priority,omitempty" desc:"Priority: low, medium, high (default: medium)"`
}

var compose = tool.Define("cairn.compose",
	"Compose a message and add it to the feed as an agent-authored event.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p composeParams) (*tool.ToolResult, error) {
		if ctx.Events == nil {
			return &tool.ToolResult{Error: "event service not available"}, nil
		}
		if p.Title == "" {
			return &tool.ToolResult{Error: "title is required"}, nil
		}
		if p.Body == "" {
			return &tool.ToolResult{Error: "body is required"}, nil
		}

		priority := "medium"
		if p.Priority != "" {
			validPriorities := map[string]bool{"low": true, "medium": true, "high": true}
			if !validPriorities[p.Priority] {
				return &tool.ToolResult{Error: fmt.Sprintf("invalid priority %q, must be one of: low, medium, high", p.Priority)}, nil
			}
			priority = p.Priority
		}

		// Create an agent-authored event in the feed.
		events, err := ctx.Events.Ingest(ctx.Cancel, []*tool.IngestEvent{{
			Source:     "agent",
			SourceID:   fmt.Sprintf("compose-%d", time.Now().UnixNano()),
			Kind:       "message",
			Title:      p.Title,
			Body:       p.Body,
			Actor:      "cairn",
			OccurredAt: time.Now(),
			Metadata:   map[string]any{"priority": priority},
		}})
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to compose message: %v", err)}, nil
		}

		count := len(events)
		return &tool.ToolResult{
			Output:   fmt.Sprintf("Message composed: %q (priority: %s)", p.Title, priority),
			Metadata: map[string]any{"count": count, "priority": priority},
		}, nil
	},
)

// getStatusParams has no inputs.
type getStatusParams struct{}

var getStatus = tool.Define("cairn.getStatus",
	"Get system status: uptime, active tasks, unread events, memory count, poller status.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, _ getStatusParams) (*tool.ToolResult, error) {
		if ctx.Status == nil {
			return &tool.ToolResult{Error: "status service not available"}, nil
		}

		status, err := ctx.Status.GetStatus(ctx.Cancel)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to get status: %v", err)}, nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "System Status\n")
		fmt.Fprintf(&b, "  Uptime:        %s\n", status.Uptime)
		fmt.Fprintf(&b, "  Active tasks:  %d\n", status.ActiveTasks)
		fmt.Fprintf(&b, "  Unread events: %d\n", status.UnreadEvents)
		fmt.Fprintf(&b, "  Memories:      %d\n", status.MemoryCount)

		if len(status.PollerStatus) > 0 {
			b.WriteString("  Pollers:\n")
			for _, p := range status.PollerStatus {
				activeStr := "active"
				if !p.Active {
					activeStr = "inactive"
				}
				fmt.Fprintf(&b, "    - %s: %s\n", p.Source, activeStr)
			}
		}

		return &tool.ToolResult{
			Output: b.String(),
			Metadata: map[string]any{
				"activeTasks":  status.ActiveTasks,
				"unreadEvents": status.UnreadEvents,
				"memoryCount":  status.MemoryCount,
			},
		}, nil
	},
)
