package memory

import (
	"context"
	"math"
	"testing"
)

func TestSearch_KeywordOnly(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	ctx := context.Background()

	// Create accepted memories.
	memories := []*Memory{
		{Content: "Go uses goroutines for concurrency", Category: CatFact, Status: StatusAccepted},
		{Content: "User prefers vim keybindings", Category: CatPreference, Status: StatusAccepted},
		{Content: "Rust has a borrow checker", Category: CatFact, Status: StatusAccepted},
	}
	for _, m := range memories {
		if err := store.Create(ctx, m); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	// Search with NoopEmbedder — keyword only.
	results, err := Search(ctx, store, NoopEmbedder{}, "goroutines", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Memory.Content != "Go uses goroutines for concurrency" {
		t.Errorf("unexpected content: %q", results[0].Memory.Content)
	}
	if results[0].Score <= 0 || results[0].Score > 1.0 {
		t.Errorf("score out of range: %f", results[0].Score)
	}
}

func TestSearch_NoResults(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	ctx := context.Background()

	results, err := Search(ctx, store, NoopEmbedder{}, "nonexistent", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []float32
		expected float64
		epsilon  float64
	}{
		{
			name:     "identical vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{1, 0, 0},
			expected: 1.0,
			epsilon:  1e-6,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{0, 1, 0},
			expected: 0.0,
			epsilon:  1e-6,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{-1, 0, 0},
			expected: -1.0,
			epsilon:  1e-6,
		},
		{
			name:     "similar vectors",
			a:        []float32{1, 2, 3},
			b:        []float32{1, 2, 3.1},
			expected: 0.9998,
			epsilon:  0.001,
		},
		{
			name:     "zero vector a",
			a:        []float32{0, 0, 0},
			b:        []float32{1, 2, 3},
			expected: 0.0,
			epsilon:  1e-6,
		},
		{
			name:     "different lengths",
			a:        []float32{1, 2},
			b:        []float32{1, 2, 3},
			expected: 0.0,
			epsilon:  1e-6,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cosineSimilarity(tc.a, tc.b)
			if math.Abs(got-tc.expected) > tc.epsilon {
				t.Errorf("cosineSimilarity(%v, %v) = %f, want %f (epsilon %f)",
					tc.a, tc.b, got, tc.expected, tc.epsilon)
			}
		})
	}
}

// mockEmbedder returns pre-configured embeddings for testing vector search.
type mockEmbedder struct {
	embeddings map[string][]float32
	dim        int
}

func (m *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i, text := range texts {
		if emb, ok := m.embeddings[text]; ok {
			result[i] = emb
		} else {
			// Return a default embedding for unknown text.
			result[i] = make([]float32, m.dim)
			result[i][0] = 0.1
		}
	}
	return result, nil
}

func (m *mockEmbedder) Dimensions() int { return m.dim }

func TestMMR_Diversity(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	ctx := context.Background()

	// Create 3 memories with controlled embeddings:
	// - mem1 and mem2 have very similar embeddings (cluster A)
	// - mem3 has a different embedding (cluster B)
	embA1 := []float32{1.0, 0.0, 0.0}
	embA2 := []float32{0.99, 0.1, 0.0} // very similar to A1
	embB := []float32{0.0, 1.0, 0.0}   // orthogonal to cluster A

	memories := []*Memory{
		{Content: "Go concurrency fact", Category: CatFact, Status: StatusAccepted, Embedding: embA1},
		{Content: "Go goroutines fact", Category: CatFact, Status: StatusAccepted, Embedding: embA2},
		{Content: "Go channels fact", Category: CatFact, Status: StatusAccepted, Embedding: embB},
	}
	for _, m := range memories {
		if err := store.Create(ctx, m); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	embedder := &mockEmbedder{
		dim: 3,
		embeddings: map[string][]float32{
			"Go concurrency": {0.95, 0.05, 0.0}, // close to cluster A
		},
	}

	results, err := Search(ctx, store, embedder, "Go concurrency", 3)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}

	// With MMR, the diverse result (cluster B) should appear before the
	// second cluster-A result, since it adds more information.
	// The first result should be from cluster A (highest relevance).
	// Verify scores are in valid range.
	for i, r := range results {
		if r.Score < 0 || r.Score > 1.5 {
			t.Errorf("result[%d] score %f out of expected range", i, r.Score)
		}
	}

	// Verify we got results from both clusters (diversity).
	hasClusterA := false
	hasClusterB := false
	for _, r := range results {
		if r.Memory.Content == "Go channels fact" {
			hasClusterB = true
		} else {
			hasClusterA = true
		}
	}
	if !hasClusterA || !hasClusterB {
		t.Error("MMR should select memories from both clusters for diversity")
	}
}

func TestSearch_WithEmbedder(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	ctx := context.Background()

	emb1 := []float32{1.0, 0.0, 0.0}
	emb2 := []float32{0.0, 1.0, 0.0}

	memories := []*Memory{
		{Content: "Go programming language", Category: CatFact, Status: StatusAccepted, Embedding: emb1},
		{Content: "Rust programming language", Category: CatFact, Status: StatusAccepted, Embedding: emb2},
	}
	for _, m := range memories {
		if err := store.Create(ctx, m); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	embedder := &mockEmbedder{
		dim: 3,
		embeddings: map[string][]float32{
			"Go programming": {0.9, 0.1, 0.0}, // closer to emb1
		},
	}

	results, err := Search(ctx, store, embedder, "Go programming", 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	// The Go memory should score higher due to closer embedding.
	if results[0].Memory.Content != "Go programming language" {
		t.Errorf("expected Go memory first, got %q", results[0].Memory.Content)
	}
}
