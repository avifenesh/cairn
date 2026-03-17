package memory

import (
	"context"
	"log/slog"
	"time"

	"github.com/avifenesh/cairn/internal/eventbus"
)

// Service wires the memory store, embedder, and event bus into a
// high-level API for the rest of the system.
type Service struct {
	store    *Store
	embedder Embedder
	bus      *eventbus.Bus
}

// NewService creates a Service. The bus may be nil if event emission
// is not needed (e.g. in tests).
func NewService(store *Store, embedder Embedder, bus *eventbus.Bus) *Service {
	return &Service{
		store:    store,
		embedder: embedder,
		bus:      bus,
	}
}

// Create persists a new memory, computes its embedding, and emits
// a MemoryProposed event.
func (s *Service) Create(ctx context.Context, m *Memory) error {
	// Generate embedding for the content.
	if s.embedder.Dimensions() > 0 {
		vecs, err := s.embedder.Embed(ctx, []string{m.Content})
		if err != nil {
			slog.Warn("memory: embedding failed, proceeding without", "error", err)
		} else if len(vecs) > 0 && vecs[0] != nil {
			m.Embedding = vecs[0]
		}
	}

	if err := s.store.Create(ctx, m); err != nil {
		return err
	}

	if s.bus != nil {
		eventbus.Publish(s.bus, eventbus.MemoryProposed{
			EventMeta: eventbus.NewMeta("memory"),
			MemoryID:  m.ID,
			Content:   m.Content,
		})
	}

	return nil
}

// Accept transitions a memory from proposed to accepted and emits
// a MemoryAccepted event.
func (s *Service) Accept(ctx context.Context, id string) error {
	if err := s.store.UpdateStatus(ctx, id, StatusAccepted); err != nil {
		return err
	}

	if s.bus != nil {
		eventbus.Publish(s.bus, eventbus.MemoryAccepted{
			EventMeta: eventbus.NewMeta("memory"),
			MemoryID:  id,
		})
	}

	return nil
}

// Reject transitions a memory from proposed to rejected and emits
// a MemoryRejected event.
func (s *Service) Reject(ctx context.Context, id string) error {
	if err := s.store.UpdateStatus(ctx, id, StatusRejected); err != nil {
		return err
	}

	if s.bus != nil {
		eventbus.Publish(s.bus, eventbus.MemoryRejected{
			EventMeta: eventbus.NewMeta("memory"),
			MemoryID:  id,
		})
	}

	return nil
}

// Search performs hybrid keyword + vector search with MMR re-ranking.
func (s *Service) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	return Search(ctx, s.store, s.embedder, query, limit)
}

// Get retrieves a single memory by ID.
func (s *Service) Get(ctx context.Context, id string) (*Memory, error) {
	return s.store.Get(ctx, id)
}

// List returns memories matching the given filters.
func (s *Service) List(ctx context.Context, opts ListOpts) ([]*Memory, error) {
	return s.store.List(ctx, opts)
}

// Delete removes a memory by ID.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.store.Delete(ctx, id)
}

// Compact decays old unused memories and auto-rejects those below threshold.
//
// Memories with UseCount=0 and age > 30 days get Confidence *= 0.8.
// If Confidence drops below 0.1, the memory is auto-rejected.
func (s *Service) Compact(ctx context.Context) error {
	const decayAge = 30 * 24 * time.Hour
	const decayFactor = 0.8
	const rejectThreshold = 0.1

	old, err := s.store.OldUnusedMemories(ctx, decayAge)
	if err != nil {
		return err
	}

	decayed, rejected := 0, 0
	for _, m := range old {
		newConf := m.Confidence * decayFactor
		if newConf < rejectThreshold {
			if err := s.store.UpdateStatus(ctx, m.ID, StatusRejected); err != nil {
				slog.Warn("memory: compact reject failed", "id", m.ID, "error", err)
				continue
			}
			rejected++
		} else {
			if err := s.store.UpdateConfidence(ctx, m.ID, newConf); err != nil {
				slog.Warn("memory: compact decay failed", "id", m.ID, "error", err)
				continue
			}
			decayed++
		}
	}

	if decayed > 0 || rejected > 0 {
		slog.Info("memory compaction complete", "decayed", decayed, "rejected", rejected)
	}
	return nil
}
