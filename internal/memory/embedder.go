package memory

import "context"

// Embedder turns text into dense vector representations for semantic search.
// Actual providers (OpenAI, local models) implement this interface; for now
// NoopEmbedder allows the system to work with keyword search only.
type Embedder interface {
	// Embed produces one embedding vector per input text.
	Embed(ctx context.Context, texts []string) ([][]float32, error)

	// Dimensions returns the dimensionality of the embedding vectors.
	// Returns 0 for NoopEmbedder (no embeddings available).
	Dimensions() int
}

// NoopEmbedder returns nil embeddings — keyword-only fallback.
type NoopEmbedder struct{}

// Embed returns a slice of nil embeddings matching the input length.
func (NoopEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	return make([][]float32, len(texts)), nil
}

// Dimensions returns 0 (no embedding support).
func (NoopEmbedder) Dimensions() int { return 0 }
