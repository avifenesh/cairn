package signal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/llm"
)

// DigestRunner generates summaries of unread events using an LLM.
type DigestRunner struct {
	store    *EventStore
	provider llm.Provider
	model    string
}

// Digest holds a generated event summary.
type Digest struct {
	Summary    string        `json:"summary"`
	Highlights []string      `json:"highlights"`
	Groups     []DigestGroup `json:"groups"`
	EventCount int           `json:"eventCount"`
	Period     TimeRange     `json:"period"`
}

// DigestGroup is a set of events grouped by source or entity.
type DigestGroup struct {
	Key    string `json:"key"`
	Count  int    `json:"count"`
	Sample string `json:"sample"` // representative title
}

// TimeRange represents a time window.
type TimeRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// NewDigestRunner creates a digest generator.
func NewDigestRunner(store *EventStore, provider llm.Provider, model string) *DigestRunner {
	return &DigestRunner{
		store:    store,
		provider: provider,
		model:    model,
	}
}

// Generate creates a digest of unread events. Groups events by source, builds
// a prompt, and calls the LLM to produce a summary.
func (d *DigestRunner) Generate(ctx context.Context) (*Digest, error) {
	events, err := d.store.List(ctx, EventFilter{UnreadOnly: true, Limit: 200})
	if err != nil {
		return nil, fmt.Errorf("digest: list events: %w", err)
	}
	if len(events) == 0 {
		return &Digest{Summary: "No new events."}, nil
	}

	// Group events by source.
	groups := map[string][]string{}
	var oldest, newest time.Time
	for _, ev := range events {
		groups[ev.Source] = append(groups[ev.Source], ev.Title)
		if oldest.IsZero() || ev.CreatedAt.Before(oldest) {
			oldest = ev.CreatedAt
		}
		if ev.CreatedAt.After(newest) {
			newest = ev.CreatedAt
		}
	}

	// Build digest groups.
	var digestGroups []DigestGroup
	for key, titles := range groups {
		sample := titles[0]
		if r := []rune(sample); len(r) > 100 {
			sample = string(r[:100]) + "..."
		}
		digestGroups = append(digestGroups, DigestGroup{
			Key:    key,
			Count:  len(titles),
			Sample: sample,
		})
	}

	// Build the prompt for LLM summarization.
	prompt := d.buildPrompt(events, groups)

	// Call LLM.
	summary, err := d.callLLM(ctx, prompt)
	if err != nil {
		// Return a basic digest without LLM summary on failure.
		return &Digest{
			Summary:    fmt.Sprintf("%d unread events from %d sources.", len(events), len(groups)),
			Groups:     digestGroups,
			EventCount: len(events),
			Period:     TimeRange{From: oldest, To: newest},
		}, nil
	}

	// Parse highlights from summary (lines starting with -).
	var highlights []string
	for _, line := range strings.Split(summary, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			highlights = append(highlights, strings.TrimLeft(line, "-* "))
		}
	}

	return &Digest{
		Summary:    summary,
		Highlights: highlights,
		Groups:     digestGroups,
		EventCount: len(events),
		Period:     TimeRange{From: oldest, To: newest},
	}, nil
}

func (d *DigestRunner) buildPrompt(events []*StoredEvent, groups map[string][]string) string {
	var b strings.Builder
	b.WriteString("Summarize these notifications concisely. Group by source. Highlight the most important items. Use bullet points for highlights.\n\n")

	const maxTitlesPerSource = 15
	for source, titles := range groups {
		fmt.Fprintf(&b, "## %s (%d events)\n", source, len(titles))
		limit := maxTitlesPerSource
		if len(titles) < limit {
			limit = len(titles)
		}
		for _, t := range titles[:limit] {
			fmt.Fprintf(&b, "- %s\n", t)
		}
		if len(titles) > maxTitlesPerSource {
			fmt.Fprintf(&b, "- ... and %d more\n", len(titles)-maxTitlesPerSource)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (d *DigestRunner) callLLM(ctx context.Context, prompt string) (string, error) {
	if d.provider == nil {
		return "", fmt.Errorf("digest: no LLM provider configured")
	}

	req := &llm.Request{
		Model: d.model,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock{Text: prompt}}},
		},
		MaxTokens: 1024,
	}

	ch, err := d.provider.Stream(ctx, req)
	if err != nil {
		return "", fmt.Errorf("digest: llm stream: %w", err)
	}

	var result strings.Builder
	for ev := range ch {
		switch e := ev.(type) {
		case llm.TextDelta:
			result.WriteString(e.Text)
		case llm.StreamError:
			return result.String(), fmt.Errorf("digest: llm error: %w", e.Err)
		}
	}
	return result.String(), nil
}
