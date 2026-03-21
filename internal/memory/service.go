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

// Update persists changes to a memory, re-embedding content if the embedder is active.
func (s *Service) Update(ctx context.Context, m *Memory) error {
	if s.embedder.Dimensions() > 0 {
		vecs, err := s.embedder.Embed(ctx, []string{m.Content})
		if err != nil {
			slog.Warn("memory: re-embedding failed on update", "id", m.ID, "error", err)
		} else if len(vecs) > 0 && vecs[0] != nil {
			m.Embedding = vecs[0]
		}
	}
	return s.store.Update(ctx, m)
}

// BackfillEmbeddings computes embeddings for accepted memories that don't have one.
// Runs in batches with rate limiting. Designed to run in a background goroutine.
func (s *Service) BackfillEmbeddings(ctx context.Context) error {
	if s.embedder.Dimensions() == 0 {
		return nil
	}

	memories, err := s.store.AllAcceptedWithoutEmbeddings(ctx)
	if err != nil {
		return err
	}
	if len(memories) == 0 {
		return nil
	}

	slog.Info("memory: backfilling embeddings", "count", len(memories))

	const batchSize = 20
	embedded := 0
	for i := 0; i < len(memories); i += batchSize {
		end := i + batchSize
		if end > len(memories) {
			end = len(memories)
		}
		batch := memories[i:end]

		texts := make([]string, len(batch))
		for j, m := range batch {
			texts[j] = m.Content
		}

		vecs, err := s.embedder.Embed(ctx, texts)
		if err != nil {
			slog.Warn("memory: backfill batch failed", "offset", i, "error", err)
			continue
		}

		for j, vec := range vecs {
			if vec == nil {
				continue
			}
			if err := s.store.UpdateEmbedding(ctx, batch[j].ID, vec); err != nil {
				slog.Warn("memory: backfill update failed", "id", batch[j].ID, "error", err)
			} else {
				embedded++
			}
		}

		// Rate limit between batches.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}

	slog.Info("memory: backfill complete", "embedded", embedded, "total", len(memories))
	return nil
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

// SemanticDedup finds near-duplicate accepted memories using embedding cosine
// similarity and removes the lower-confidence (or newer) duplicate.
// Returns the number of duplicates removed.
func (s *Service) SemanticDedup(ctx context.Context, threshold float64) (int, error) {
	if threshold <= 0 || threshold > 1 {
		threshold = 0.92
	}

	mems, err := s.store.AllAcceptedWithEmbeddings(ctx)
	if err != nil {
		return 0, err
	}
	if len(mems) < 2 {
		return 0, nil
	}

	// Mark which memories to delete (keep the better one from each pair).
	toDelete := make(map[string]bool)
	removed := 0

	for i := 0; i < len(mems); i++ {
		if toDelete[mems[i].ID] {
			continue
		}
		for j := i + 1; j < len(mems); j++ {
			if toDelete[mems[j].ID] {
				continue
			}
			// Only compare within the same category.
			if mems[i].Category != mems[j].Category {
				continue
			}
			sim := cosineSimilarity(mems[i].Embedding, mems[j].Embedding)
			if sim >= threshold {
				// Keep the one with higher confidence, or older if tied.
				victim := mems[j]
				if mems[j].Confidence > mems[i].Confidence ||
					(mems[j].Confidence == mems[i].Confidence && mems[j].CreatedAt.Before(mems[i].CreatedAt)) {
					victim = mems[i]
				}
				toDelete[victim.ID] = true
			}
		}
	}

	for id := range toDelete {
		if err := s.store.Delete(ctx, id); err == nil {
			removed++
		}
	}

	if removed > 0 {
		slog.Info("semantic dedup complete", "removed", removed, "threshold", threshold, "scanned", len(mems))
	}
	return removed, nil
}
