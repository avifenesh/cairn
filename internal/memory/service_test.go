package memory

import (
	"context"
	"testing"
	"time"

	"github.com/avifenesh/cairn/internal/eventbus"
)

func TestService_CreateAndSearch(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	bus := eventbus.New()
	defer bus.Close()

	svc := NewService(store, NoopEmbedder{}, bus)
	ctx := context.Background()

	// Create 3 memories and accept them.
	memories := []*Memory{
		{Content: "Go is great for backend development", Category: CatFact},
		{Content: "User prefers dark mode in editors", Category: CatPreference},
		{Content: "Always run tests before committing", Category: CatHardRule},
	}
	for _, m := range memories {
		if err := svc.Create(ctx, m); err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := svc.Accept(ctx, m.ID); err != nil {
			t.Fatalf("Accept: %v", err)
		}
	}

	// Search for "dark mode".
	results, err := svc.Search(ctx, "dark mode", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'dark mode', got %d", len(results))
	}
	if results[0].Memory.Content != "User prefers dark mode in editors" {
		t.Errorf("unexpected content: %q", results[0].Memory.Content)
	}

	// Search for "Go" — should find the Go memory.
	results, err = svc.Search(ctx, "Go", 10)
	if err != nil {
		t.Fatalf("Search 'Go': %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result for 'Go'")
	}
}

func TestService_AcceptReject(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	bus := eventbus.New()
	defer bus.Close()

	// Track events.
	var proposedIDs, acceptedIDs, rejectedIDs []string

	eventbus.Subscribe(bus, func(e eventbus.MemoryProposed) {
		proposedIDs = append(proposedIDs, e.MemoryID)
	})
	eventbus.Subscribe(bus, func(e eventbus.MemoryAccepted) {
		acceptedIDs = append(acceptedIDs, e.MemoryID)
	})
	eventbus.Subscribe(bus, func(e eventbus.MemoryRejected) {
		rejectedIDs = append(rejectedIDs, e.MemoryID)
	})

	svc := NewService(store, NoopEmbedder{}, bus)
	ctx := context.Background()

	// Create a memory (should emit MemoryProposed).
	accepted := &Memory{Content: "accepted memory content"}
	if err := svc.Create(ctx, accepted); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Accept it — should be searchable.
	if err := svc.Accept(ctx, accepted.ID); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	results, err := svc.Search(ctx, "accepted memory", 10)
	if err != nil {
		t.Fatalf("Search after accept: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result after accept, got %d", len(results))
	}

	// Create and reject another memory — should NOT be searchable.
	rejected := &Memory{Content: "rejected memory content"}
	if err := svc.Create(ctx, rejected); err != nil {
		t.Fatalf("Create rejected: %v", err)
	}
	if err := svc.Reject(ctx, rejected.ID); err != nil {
		t.Fatalf("Reject: %v", err)
	}

	results, err = svc.Search(ctx, "rejected memory", 10)
	if err != nil {
		t.Fatalf("Search after reject: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for rejected memory, got %d", len(results))
	}

	// Verify events were emitted.
	if len(proposedIDs) != 2 {
		t.Errorf("expected 2 MemoryProposed events, got %d", len(proposedIDs))
	}
	if len(acceptedIDs) != 1 {
		t.Errorf("expected 1 MemoryAccepted event, got %d", len(acceptedIDs))
	}
	if len(rejectedIDs) != 1 {
		t.Errorf("expected 1 MemoryRejected event, got %d", len(rejectedIDs))
	}
}

func TestService_Compact(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	svc := NewService(store, NoopEmbedder{}, nil)
	ctx := context.Background()

	// Create a memory with low confidence and zero use count.
	m := &Memory{
		Content:    "old unused memory",
		Status:     StatusAccepted,
		Confidence: 0.12,
	}
	if err := store.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Manually backdate the created_at to > 30 days ago.
	oldDate := time.Now().UTC().Add(-31 * 24 * time.Hour).Format(timeFormat)
	_, err := d.ExecContext(ctx, "UPDATE memories SET created_at = ? WHERE id = ?", oldDate, m.ID)
	if err != nil {
		t.Fatalf("backdate: %v", err)
	}

	// Run compaction.
	if err := svc.Compact(ctx); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	// Memory should have decayed confidence: 0.12 * 0.8 = 0.096 < 0.1 → rejected.
	got, err := store.Get(ctx, m.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusRejected {
		t.Errorf("expected status %q after compact, got %q", StatusRejected, got.Status)
	}
}

func TestService_CompactDecay(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	svc := NewService(store, NoopEmbedder{}, nil)
	ctx := context.Background()

	// Create a memory with medium confidence — should decay but not reject.
	m := &Memory{
		Content:    "slightly old memory",
		Status:     StatusAccepted,
		Confidence: 0.5,
	}
	if err := store.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Backdate to > 30 days.
	oldDate := time.Now().UTC().Add(-35 * 24 * time.Hour).Format(timeFormat)
	_, err := d.ExecContext(ctx, "UPDATE memories SET created_at = ? WHERE id = ?", oldDate, m.ID)
	if err != nil {
		t.Fatalf("backdate: %v", err)
	}

	if err := svc.Compact(ctx); err != nil {
		t.Fatalf("Compact: %v", err)
	}

	got, err := store.Get(ctx, m.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Should still be accepted but with decayed confidence: 0.5 * 0.8 = 0.4.
	if got.Status != StatusAccepted {
		t.Errorf("expected status %q, got %q", StatusAccepted, got.Status)
	}
	if got.Confidence > 0.41 || got.Confidence < 0.39 {
		t.Errorf("expected confidence ~0.4, got %f", got.Confidence)
	}
}

func TestService_Delete(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	svc := NewService(store, NoopEmbedder{}, nil)
	ctx := context.Background()

	m := &Memory{Content: "to be deleted"}
	if err := svc.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.Delete(ctx, m.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := svc.Get(ctx, m.ID)
	if err == nil {
		t.Fatal("expected error getting deleted memory")
	}
}

func TestService_List(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	svc := NewService(store, NoopEmbedder{}, nil)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		m := &Memory{Content: "list test memory", Category: CatFact}
		if err := svc.Create(ctx, m); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	results, err := svc.List(ctx, ListOpts{Category: CatFact})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3, got %d", len(results))
	}
}

func TestService_NilBus(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	// nil bus should not panic.
	svc := NewService(store, NoopEmbedder{}, nil)
	ctx := context.Background()

	m := &Memory{Content: "no bus memory"}
	if err := svc.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Accept(ctx, m.ID); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if err := svc.Reject(ctx, m.ID); err != nil {
		t.Fatalf("Reject: %v", err)
	}
}

func TestService_BackfillEmbeddings(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)

	// Create memories without embeddings using NoopEmbedder.
	noopSvc := NewService(store, NoopEmbedder{}, nil)
	ctx := context.Background()

	m1 := &Memory{Content: "memory one", Status: StatusAccepted}
	m2 := &Memory{Content: "memory two", Status: StatusAccepted}
	for _, m := range []*Memory{m1, m2} {
		if err := store.Create(ctx, m); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	// Verify no embeddings.
	without, _ := store.AllAcceptedWithoutEmbeddings(ctx)
	if len(without) != 2 {
		t.Fatalf("expected 2 without embeddings, got %d", len(without))
	}

	// Noop backfill should be a no-op.
	if err := noopSvc.BackfillEmbeddings(ctx); err != nil {
		t.Fatalf("NoopEmbedder backfill: %v", err)
	}

	// Switch to mock embedder and backfill.
	mock := &testEmbedder{dims: 3, vec: []float32{0.1, 0.2, 0.3}}
	svc := NewService(store, mock, nil)
	if err := svc.BackfillEmbeddings(ctx); err != nil {
		t.Fatalf("BackfillEmbeddings: %v", err)
	}

	// Verify embeddings were stored.
	without, _ = store.AllAcceptedWithoutEmbeddings(ctx)
	if len(without) != 0 {
		t.Fatalf("expected 0 without embeddings after backfill, got %d", len(without))
	}

	got, _ := store.Get(ctx, m1.ID)
	if len(got.Embedding) != 3 {
		t.Fatalf("expected 3 dims after backfill, got %d", len(got.Embedding))
	}
}

func TestService_UpdateReembeds(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	mock := &testEmbedder{dims: 2, vec: []float32{0.5, 0.6}}
	svc := NewService(store, mock, nil)
	ctx := context.Background()

	m := &Memory{Content: "original content"}
	if err := svc.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Verify initial embedding.
	got, _ := store.Get(ctx, m.ID)
	if len(got.Embedding) != 2 {
		t.Fatalf("expected 2 dims, got %d", len(got.Embedding))
	}

	// Update content — should re-embed.
	mock.vec = []float32{0.9, 0.8}
	m.Content = "updated content"
	if err := svc.Update(ctx, m); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ = store.Get(ctx, m.ID)
	if got.Embedding[0] != 0.9 {
		t.Fatalf("expected re-embedded 0.9, got %f", got.Embedding[0])
	}
}

// testEmbedder is a simple mock that returns a fixed vector.
type testEmbedder struct {
	dims int
	vec  []float32
}

func (e *testEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	vecs := make([][]float32, len(texts))
	for i := range texts {
		v := make([]float32, len(e.vec))
		copy(v, e.vec)
		vecs[i] = v
	}
	return vecs, nil
}

func (e *testEmbedder) Dimensions() int { return e.dims }
