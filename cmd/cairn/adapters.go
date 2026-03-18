package main

import (
	"context"
	"time"

	"github.com/avifenesh/cairn/internal/agent"
	"github.com/avifenesh/cairn/internal/memory"
	"github.com/avifenesh/cairn/internal/signal"
	"github.com/avifenesh/cairn/internal/tool"
)

// memoryAdapter bridges memory.Service to tool.MemoryService.
type memoryAdapter struct {
	svc *memory.Service
}

func (a *memoryAdapter) Create(ctx context.Context, m *tool.MemoryItem) error {
	mem := &memory.Memory{
		Content:  m.Content,
		Category: memory.Category(m.Category),
		Scope:    memory.Scope(m.Scope),
		Source:   m.Source,
	}
	if err := a.svc.Create(ctx, mem); err != nil {
		return err
	}
	m.ID = mem.ID // propagate generated ID back
	return nil
}

func (a *memoryAdapter) Search(ctx context.Context, query string, limit int) ([]tool.MemorySearchResult, error) {
	results, err := a.svc.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]tool.MemorySearchResult, len(results))
	for i, r := range results {
		out[i] = tool.MemorySearchResult{
			Memory: &tool.MemoryItem{
				ID:         r.Memory.ID,
				Content:    r.Memory.Content,
				Category:   string(r.Memory.Category),
				Scope:      string(r.Memory.Scope),
				Status:     string(r.Memory.Status),
				Confidence: r.Memory.Confidence,
				Source:     r.Memory.Source,
			},
			Score: r.Score,
		}
	}
	return out, nil
}

func (a *memoryAdapter) Get(ctx context.Context, id string) (*tool.MemoryItem, error) {
	m, err := a.svc.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &tool.MemoryItem{
		ID:         m.ID,
		Content:    m.Content,
		Category:   string(m.Category),
		Scope:      string(m.Scope),
		Status:     string(m.Status),
		Confidence: m.Confidence,
		Source:     m.Source,
	}, nil
}

func (a *memoryAdapter) Accept(ctx context.Context, id string) error {
	return a.svc.Accept(ctx, id)
}

func (a *memoryAdapter) Reject(ctx context.Context, id string) error {
	return a.svc.Reject(ctx, id)
}

func (a *memoryAdapter) Delete(ctx context.Context, id string) error {
	return a.svc.Delete(ctx, id)
}

// eventAdapter bridges signal.EventStore to tool.EventService.
type eventAdapter struct {
	store *signal.EventStore
}

func (a *eventAdapter) List(ctx context.Context, f tool.EventFilter) ([]*tool.StoredEvent, error) {
	events, err := a.store.List(ctx, signal.EventFilter{
		Source:     f.Source,
		Kind:       f.Kind,
		UnreadOnly: f.UnreadOnly,
		Limit:      f.Limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]*tool.StoredEvent, len(events))
	for i, ev := range events {
		out[i] = &tool.StoredEvent{
			ID:        ev.ID,
			Source:    ev.Source,
			Kind:      ev.Kind,
			Title:     ev.Title,
			Body:      ev.Body,
			URL:       ev.URL,
			Actor:     ev.Actor,
			CreatedAt: ev.CreatedAt,
			ReadAt:    ev.ReadAt,
		}
	}
	return out, nil
}

func (a *eventAdapter) MarkRead(ctx context.Context, id string) error {
	return a.store.MarkRead(ctx, id)
}

func (a *eventAdapter) MarkAllRead(ctx context.Context) (int, error) {
	return a.store.MarkAllRead(ctx)
}

// digestAdapter bridges signal.DigestRunner to tool.DigestService.
type digestAdapter struct {
	runner *signal.DigestRunner
}

func (a *digestAdapter) Generate(ctx context.Context) (*tool.DigestResult, error) {
	d, err := a.runner.Generate(ctx)
	if err != nil {
		return nil, err
	}
	return &tool.DigestResult{
		Summary:    d.Summary,
		Highlights: d.Highlights,
		EventCount: d.EventCount,
	}, nil
}

// journalAdapter bridges agent.JournalStore to tool.JournalService.
type journalAdapter struct {
	store *agent.JournalStore
}

func (a *journalAdapter) Recent(ctx context.Context, dur time.Duration) ([]*tool.JournalEntry, error) {
	entries, err := a.store.Recent(ctx, dur)
	if err != nil {
		return nil, err
	}
	out := make([]*tool.JournalEntry, len(entries))
	for i, e := range entries {
		out[i] = &tool.JournalEntry{
			ID:        e.ID,
			Summary:   e.Summary,
			Decisions: e.Decisions,
			Errors:    e.Errors,
			Learnings: e.Learnings,
			Mode:      e.Mode,
			CreatedAt: e.CreatedAt,
		}
	}
	return out, nil
}
