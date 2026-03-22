package builtin

import (
	"fmt"
	"strings"

	"github.com/avifenesh/cairn/internal/tool"
)

// readFeedParams are the parameters for cairn.readFeed.
type readFeedParams struct {
	Source          string `json:"source,omitempty" desc:"Filter by source (e.g. github, hn, reddit, npm, crates)"`
	Limit           *int   `json:"limit,omitempty" desc:"Maximum events to return (default 20)"`
	UnreadOnly      *bool  `json:"unreadOnly,omitempty" desc:"Only return unread events (default true)"`
	IncludeArchived *bool  `json:"includeArchived,omitempty" desc:"Include archived events (default false)"`
}

var readFeed = tool.Define("cairn.readFeed",
	"Read feed events from signal sources. Returns recent notifications and updates.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p readFeedParams) (*tool.ToolResult, error) {
		if ctx.Events == nil {
			return &tool.ToolResult{Error: "event service not available"}, nil
		}

		limit := 20
		if p.Limit != nil && *p.Limit > 0 {
			limit = *p.Limit
		}

		unreadOnly := true
		if p.UnreadOnly != nil {
			unreadOnly = *p.UnreadOnly
		}

		excludeArchived := true
		if p.IncludeArchived != nil && *p.IncludeArchived {
			excludeArchived = false
			// When showing archived items, also include read ones —
			// an archived item is almost always already read, so
			// unreadOnly=true would hide everything.
			if p.UnreadOnly == nil {
				unreadOnly = false
			}
		}

		events, err := ctx.Events.List(ctx.Cancel, tool.EventFilter{
			Source:          p.Source,
			UnreadOnly:      unreadOnly,
			ExcludeArchived: excludeArchived,
			Limit:           limit,
		})
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to read feed: %v", err)}, nil
		}

		if len(events) == 0 {
			return &tool.ToolResult{Output: "No events found."}, nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Feed: %d events\n\n", len(events))
		for i, ev := range events {
			readMarker := " "
			if ev.ReadAt != nil {
				readMarker = "R"
			}
			fmt.Fprintf(&b, "%d. [%s] [%s/%s] %s\n", i+1, readMarker, ev.Source, ev.Kind, ev.Title)
			if ev.URL != "" {
				fmt.Fprintf(&b, "   %s\n", ev.URL)
			}
			if ev.Actor != "" {
				fmt.Fprintf(&b, "   by %s at %s\n", ev.Actor, ev.CreatedAt.Format(displayTimeFormat))
			} else {
				fmt.Fprintf(&b, "   at %s\n", ev.CreatedAt.Format(displayTimeFormat))
			}
			fmt.Fprintf(&b, "   id: %s\n\n", ev.ID)
		}

		return &tool.ToolResult{
			Output:   b.String(),
			Metadata: map[string]any{"count": len(events)},
		}, nil
	},
)

// markReadParams are the parameters for cairn.markRead.
type markReadParams struct {
	ID string `json:"id" desc:"Event ID to mark as read, or 'all' to mark all events read"`
}

var markRead = tool.Define("cairn.markRead",
	"Mark feed events as read. Pass an event ID or 'all' to mark all unread events.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p markReadParams) (*tool.ToolResult, error) {
		if ctx.Events == nil {
			return &tool.ToolResult{Error: "event service not available"}, nil
		}
		if p.ID == "" {
			return &tool.ToolResult{Error: "id is required (pass an event ID or 'all')"}, nil
		}

		if p.ID == "all" {
			count, err := ctx.Events.MarkAllRead(ctx.Cancel)
			if err != nil {
				return &tool.ToolResult{Error: fmt.Sprintf("failed to mark all read: %v", err)}, nil
			}
			return &tool.ToolResult{
				Output:   fmt.Sprintf("Marked %d events as read.", count),
				Metadata: map[string]any{"count": count},
			}, nil
		}

		if err := ctx.Events.MarkRead(ctx.Cancel, p.ID); err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to mark read: %v", err)}, nil
		}

		return &tool.ToolResult{
			Output:   fmt.Sprintf("Event %s marked as read.", p.ID),
			Metadata: map[string]any{"id": p.ID},
		}, nil
	},
)

// archiveFeedItemParams are the parameters for cairn.archiveFeedItem.
type archiveFeedItemParams struct {
	ID string `json:"id" desc:"Event ID to archive"`
}

var archiveFeedItem = tool.Define("cairn.archiveFeedItem",
	"Archive a feed event by ID. Archived events are hidden from the default feed view.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p archiveFeedItemParams) (*tool.ToolResult, error) {
		if ctx.Events == nil {
			return &tool.ToolResult{Error: "event service not available"}, nil
		}
		if p.ID == "" {
			return &tool.ToolResult{Error: "id is required"}, nil
		}
		if err := ctx.Events.Archive(ctx.Cancel, p.ID); err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to archive: %v", err)}, nil
		}
		return &tool.ToolResult{
			Output:   fmt.Sprintf("Event %s archived.", p.ID),
			Metadata: map[string]any{"id": p.ID},
		}, nil
	},
)

// deleteFeedItemParams are the parameters for cairn.deleteFeedItem.
type deleteFeedItemParams struct {
	ID string `json:"id" desc:"Event ID to delete permanently"`
}

var deleteFeedItem = tool.Define("cairn.deleteFeedItem",
	"Permanently delete a feed event by ID. This cannot be undone.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p deleteFeedItemParams) (*tool.ToolResult, error) {
		if ctx.Events == nil {
			return &tool.ToolResult{Error: "event service not available"}, nil
		}
		if p.ID == "" {
			return &tool.ToolResult{Error: "id is required"}, nil
		}
		if err := ctx.Events.DeleteByID(ctx.Cancel, p.ID); err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("failed to delete: %v", err)}, nil
		}
		return &tool.ToolResult{
			Output:   fmt.Sprintf("Event %s deleted.", p.ID),
			Metadata: map[string]any{"id": p.ID},
		}, nil
	},
)

// digestParams has no inputs.
type digestParams struct{}

var digest = tool.Define("cairn.digest",
	"Generate a digest summary of unread feed events. Groups by source and highlights important items.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, _ digestParams) (*tool.ToolResult, error) {
		if ctx.Digest == nil {
			return &tool.ToolResult{Error: "digest service not available"}, nil
		}

		result, err := ctx.Digest.Generate(ctx.Cancel)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("digest generation failed: %v", err)}, nil
		}

		var b strings.Builder
		b.WriteString(result.Summary)
		b.WriteString("\n")

		if len(result.Highlights) > 0 {
			b.WriteString("\nHighlights:\n")
			for _, h := range result.Highlights {
				fmt.Fprintf(&b, "- %s\n", h)
			}
		}

		return &tool.ToolResult{
			Output:   b.String(),
			Metadata: map[string]any{"eventCount": result.EventCount},
		}, nil
	},
)
