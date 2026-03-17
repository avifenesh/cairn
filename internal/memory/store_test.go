package memory

import (
	"context"
	"testing"

	"github.com/avifenesh/cairn/internal/db"
)

// openTestDB opens an in-memory database and runs migrations.
func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:): %v", err)
	}
	if err := d.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestStore_CreateAndGet(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	ctx := context.Background()

	m := &Memory{
		Content:    "User prefers dark mode",
		Category:   CatPreference,
		Scope:      ScopePersonal,
		Status:     StatusProposed,
		Confidence: 0.8,
		Source:     "agent",
	}

	if err := store.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if m.ID == "" {
		t.Fatal("expected non-empty ID after Create")
	}

	got, err := store.Get(ctx, m.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.Content != "User prefers dark mode" {
		t.Errorf("content: got %q, want %q", got.Content, "User prefers dark mode")
	}
	if got.Category != CatPreference {
		t.Errorf("category: got %q, want %q", got.Category, CatPreference)
	}
	if got.Scope != ScopePersonal {
		t.Errorf("scope: got %q, want %q", got.Scope, ScopePersonal)
	}
	if got.Status != StatusProposed {
		t.Errorf("status: got %q, want %q", got.Status, StatusProposed)
	}
	if got.Confidence != 0.8 {
		t.Errorf("confidence: got %f, want 0.8", got.Confidence)
	}
	if got.UseCount != 0 {
		t.Errorf("use count: got %d, want 0", got.UseCount)
	}
	if got.CreatedAt.IsZero() {
		t.Error("created_at should not be zero")
	}
}

func TestStore_List(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	ctx := context.Background()

	// Create 5 memories with varying attributes.
	mems := []*Memory{
		{Content: "fact one", Category: CatFact, Status: StatusAccepted},
		{Content: "fact two", Category: CatFact, Status: StatusAccepted},
		{Content: "pref one", Category: CatPreference, Status: StatusAccepted},
		{Content: "rule one", Category: CatHardRule, Status: StatusProposed},
		{Content: "decision one", Category: CatDecision, Status: StatusRejected},
	}
	for _, m := range mems {
		if err := store.Create(ctx, m); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	// List all.
	all, err := store.List(ctx, ListOpts{})
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 5 {
		t.Errorf("List all: got %d, want 5", len(all))
	}

	// Filter by status=accepted.
	accepted, err := store.List(ctx, ListOpts{Status: StatusAccepted})
	if err != nil {
		t.Fatalf("List accepted: %v", err)
	}
	if len(accepted) != 3 {
		t.Errorf("List accepted: got %d, want 3", len(accepted))
	}

	// Filter by category=fact.
	facts, err := store.List(ctx, ListOpts{Category: CatFact})
	if err != nil {
		t.Fatalf("List facts: %v", err)
	}
	if len(facts) != 2 {
		t.Errorf("List facts: got %d, want 2", len(facts))
	}

	// Limit.
	limited, err := store.List(ctx, ListOpts{Limit: 2})
	if err != nil {
		t.Fatalf("List limited: %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("List limited: got %d, want 2", len(limited))
	}
}

func TestStore_UpdateStatus(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	ctx := context.Background()

	m := &Memory{Content: "test memory", Status: StatusProposed}
	if err := store.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Accept the memory.
	if err := store.UpdateStatus(ctx, m.ID, StatusAccepted); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	got, err := store.Get(ctx, m.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusAccepted {
		t.Errorf("status: got %q, want %q", got.Status, StatusAccepted)
	}

	// Reject the memory.
	if err := store.UpdateStatus(ctx, m.ID, StatusRejected); err != nil {
		t.Fatalf("UpdateStatus rejected: %v", err)
	}

	got, err = store.Get(ctx, m.ID)
	if err != nil {
		t.Fatalf("Get after reject: %v", err)
	}
	if got.Status != StatusRejected {
		t.Errorf("status: got %q, want %q", got.Status, StatusRejected)
	}
}

func TestStore_SearchByKeyword(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	ctx := context.Background()

	// Create accepted memories with known text.
	memories := []*Memory{
		{Content: "Go is a compiled language", Category: CatFact, Status: StatusAccepted},
		{Content: "User likes Rust for systems programming", Category: CatPreference, Status: StatusAccepted},
		{Content: "Python is great for data science", Category: CatFact, Status: StatusAccepted},
		{Content: "This is proposed and should not match", Category: CatFact, Status: StatusProposed},
	}
	for _, m := range memories {
		if err := store.Create(ctx, m); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	// Search for "Rust" — should find 1 result.
	results, err := store.SearchByKeyword(ctx, "Rust", 10)
	if err != nil {
		t.Fatalf("SearchByKeyword: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("SearchByKeyword 'Rust': got %d results, want 1", len(results))
	}
	if results[0].Content != "User likes Rust for systems programming" {
		t.Errorf("unexpected content: %q", results[0].Content)
	}

	// Search for "language" — should find 1 (only accepted).
	results, err = store.SearchByKeyword(ctx, "language", 10)
	if err != nil {
		t.Fatalf("SearchByKeyword 'language': %v", err)
	}
	if len(results) != 1 {
		t.Errorf("SearchByKeyword 'language': got %d results, want 1", len(results))
	}

	// Search for "proposed" — should find 0 (only accepted memories searched).
	results, err = store.SearchByKeyword(ctx, "proposed", 10)
	if err != nil {
		t.Fatalf("SearchByKeyword 'proposed': %v", err)
	}
	if len(results) != 0 {
		t.Errorf("SearchByKeyword 'proposed': got %d results, want 0", len(results))
	}
}

func TestStore_IncrementUseCount(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	ctx := context.Background()

	m := &Memory{Content: "frequently used fact", Status: StatusAccepted}
	if err := store.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Increment 3 times.
	for i := 0; i < 3; i++ {
		if err := store.IncrementUseCount(ctx, m.ID); err != nil {
			t.Fatalf("IncrementUseCount (iteration %d): %v", i, err)
		}
	}

	got, err := store.Get(ctx, m.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.UseCount != 3 {
		t.Errorf("use count: got %d, want 3", got.UseCount)
	}
	if got.LastUsedAt == nil {
		t.Error("last_used_at should be set after increment")
	}
}

func TestStore_Delete(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	ctx := context.Background()

	m := &Memory{Content: "delete me"}
	if err := store.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := store.Delete(ctx, m.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := store.Get(ctx, m.ID)
	if err == nil {
		t.Fatal("expected error getting deleted memory")
	}
}

func TestStore_Update(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	ctx := context.Background()

	m := &Memory{Content: "original content", Category: CatFact}
	if err := store.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}

	m.Content = "updated content"
	m.Category = CatDecision
	if err := store.Update(ctx, m); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := store.Get(ctx, m.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Content != "updated content" {
		t.Errorf("content: got %q, want %q", got.Content, "updated content")
	}
	if got.Category != CatDecision {
		t.Errorf("category: got %q, want %q", got.Category, CatDecision)
	}
}

func TestStore_EmbeddingRoundTrip(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	ctx := context.Background()

	embedding := []float32{0.1, 0.2, 0.3, -0.5, 1.0}
	m := &Memory{
		Content:   "memory with embedding",
		Status:    StatusAccepted,
		Embedding: embedding,
	}
	if err := store.Create(ctx, m); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.Get(ctx, m.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if len(got.Embedding) != len(embedding) {
		t.Fatalf("embedding length: got %d, want %d", len(got.Embedding), len(embedding))
	}
	for i := range embedding {
		if got.Embedding[i] != embedding[i] {
			t.Errorf("embedding[%d]: got %f, want %f", i, got.Embedding[i], embedding[i])
		}
	}
}
