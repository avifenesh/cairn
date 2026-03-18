package builtin

import (
	"fmt"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/tool"
)

// journalSearchParams are the parameters for cairn.journalSearch.
type journalSearchParams struct {
	Query *string `json:"query,omitempty" desc:"Optional text to search for in journal entries"`
	Hours *int    `json:"hours,omitempty" desc:"How many hours of history to search (default 48)"`
}

var journalSearch = tool.Define("cairn.journalSearch",
	"Search the session journal for recent episodic memories. Returns summaries of past agent sessions.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p journalSearchParams) (*tool.ToolResult, error) {
		if ctx.Journal == nil {
			return &tool.ToolResult{Error: "journal service not available"}, nil
		}

		hours := 48
		if p.Hours != nil && *p.Hours > 0 {
			hours = *p.Hours
		}

		entries, err := ctx.Journal.Recent(ctx.Cancel, time.Duration(hours)*time.Hour)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("journal search failed: %v", err)}, nil
		}

		// Filter by query if provided.
		if p.Query != nil && *p.Query != "" {
			query := strings.ToLower(*p.Query)
			var filtered []*tool.JournalEntry
			for _, e := range entries {
				if matchesJournal(e, query) {
					filtered = append(filtered, e)
				}
			}
			entries = filtered
		}

		if len(entries) == 0 {
			return &tool.ToolResult{Output: "No journal entries found."}, nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Journal: %d entries (last %dh)\n\n", len(entries), hours)
		for i, e := range entries {
			fmt.Fprintf(&b, "%d. [%s] %s (at %s)\n",
				i+1, e.Mode, e.Summary, e.CreatedAt.Format("2006-01-02 15:04"))
			if len(e.Decisions) > 0 {
				fmt.Fprintf(&b, "   Decisions: %s\n", strings.Join(e.Decisions, "; "))
			}
			if len(e.Learnings) > 0 {
				fmt.Fprintf(&b, "   Learnings: %s\n", strings.Join(e.Learnings, "; "))
			}
			if len(e.Errors) > 0 {
				fmt.Fprintf(&b, "   Errors: %s\n", strings.Join(e.Errors, "; "))
			}
			b.WriteString("\n")
		}

		return &tool.ToolResult{
			Output:   b.String(),
			Metadata: map[string]any{"count": len(entries)},
		}, nil
	},
)

// matchesJournal returns true if the journal entry contains the query in any field.
func matchesJournal(e *tool.JournalEntry, query string) bool {
	if strings.Contains(strings.ToLower(e.Summary), query) {
		return true
	}
	for _, d := range e.Decisions {
		if strings.Contains(strings.ToLower(d), query) {
			return true
		}
	}
	for _, l := range e.Learnings {
		if strings.Contains(strings.ToLower(l), query) {
			return true
		}
	}
	for _, err := range e.Errors {
		if strings.Contains(strings.ToLower(err), query) {
			return true
		}
	}
	return false
}
