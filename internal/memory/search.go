package memory

import (
	"context"
	"math"
	"sort"
)

// searchCandidate is used internally by the hybrid search pipeline.
type searchCandidate struct {
	memory       *Memory
	keywordScore float64
	vectorScore  float64
	blended      float64
}

// Search performs hybrid keyword + vector search with MMR re-ranking.
//
// When the embedder is a NoopEmbedder (Dimensions() == 0), only keyword
// search is used. Otherwise, scores are blended: 0.3 keyword + 0.7 vector.
// MMR (Maximal Marginal Relevance) re-ranking promotes diversity in results.
func Search(ctx context.Context, store *Store, embedder Embedder, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	// Step 1: keyword matches.
	keywordMatches, err := store.SearchByKeyword(ctx, query, limit*3)
	if err != nil {
		return nil, err
	}

	useVectors := embedder.Dimensions() > 0

	if !useVectors {
		// Keyword-only mode: score by position (first result = highest score).
		results := make([]SearchResult, len(keywordMatches))
		for i, m := range keywordMatches {
			results[i] = SearchResult{
				Memory: m,
				Score:  1.0 - float64(i)*0.05,
			}
			if results[i].Score < 0.1 {
				results[i].Score = 0.1
			}
		}
		if len(results) > limit {
			results = results[:limit]
		}
		return results, nil
	}

	// Step 2: embed the query.
	queryEmbeddings, err := embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	queryVec := queryEmbeddings[0]
	if queryVec == nil {
		// Embedder returned nil despite Dimensions() > 0 — fall back to keyword.
		results := make([]SearchResult, len(keywordMatches))
		for i, m := range keywordMatches {
			results[i] = SearchResult{Memory: m, Score: 1.0 - float64(i)*0.05}
			if results[i].Score < 0.1 {
				results[i].Score = 0.1
			}
		}
		if len(results) > limit {
			results = results[:limit]
		}
		return results, nil
	}

	// Step 3: vector search over all accepted memories with embeddings.
	vectorMemories, err := store.AllAcceptedWithEmbeddings(ctx)
	if err != nil {
		return nil, err
	}

	// Index keyword matches by ID for quick lookup.
	keywordScores := make(map[string]float64, len(keywordMatches))
	for i, m := range keywordMatches {
		score := 1.0 - float64(i)*0.05
		if score < 0.1 {
			score = 0.1
		}
		keywordScores[m.ID] = score
	}

	// Score all vector memories.
	vectorScores := make(map[string]float64, len(vectorMemories))
	for _, m := range vectorMemories {
		vectorScores[m.ID] = cosineSimilarity(queryVec, m.Embedding)
	}

	// Merge candidates from both sets.
	seen := make(map[string]bool)
	var candidates []searchCandidate

	for _, m := range keywordMatches {
		seen[m.ID] = true
		ks := keywordScores[m.ID]
		vs := vectorScores[m.ID] // 0 if not in vector set
		candidates = append(candidates, searchCandidate{
			memory:       m,
			keywordScore: ks,
			vectorScore:  vs,
			blended:      0.3*ks + 0.7*vs,
		})
	}
	for _, m := range vectorMemories {
		if seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		vs := vectorScores[m.ID]
		candidates = append(candidates, searchCandidate{
			memory:       m,
			keywordScore: 0,
			vectorScore:  vs,
			blended:      0.7 * vs,
		})
	}

	// Sort by blended score descending for MMR input.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].blended > candidates[j].blended
	})

	// Step 4: MMR re-ranking.
	results := mmrRerank(candidates, limit, 0.7)

	return results, nil
}

// mmrRerank applies Maximal Marginal Relevance to select diverse results.
// lambda controls the relevance-diversity trade-off (1.0 = pure relevance).
func mmrRerank(candidates []searchCandidate, limit int, lambda float64) []SearchResult {
	if len(candidates) == 0 {
		return nil
	}

	selected := make([]SearchResult, 0, limit)
	used := make([]bool, len(candidates))

	for len(selected) < limit && len(selected) < len(candidates) {
		bestIdx := -1
		bestMMR := math.Inf(-1)

		for i, c := range candidates {
			if used[i] {
				continue
			}

			// Relevance component.
			relevance := c.blended

			// Diversity: max similarity to any already-selected memory.
			maxSim := 0.0
			for _, s := range selected {
				if len(c.memory.Embedding) > 0 && len(s.Memory.Embedding) > 0 {
					sim := cosineSimilarity(c.memory.Embedding, s.Memory.Embedding)
					if sim > maxSim {
						maxSim = sim
					}
				}
			}

			mmrScore := lambda*relevance - (1-lambda)*maxSim

			if mmrScore > bestMMR {
				bestMMR = mmrScore
				bestIdx = i
			}
		}

		if bestIdx < 0 {
			break
		}

		used[bestIdx] = true
		selected = append(selected, SearchResult{
			Memory: candidates[bestIdx].memory,
			Score:  candidates[bestIdx].blended,
		})
	}

	return selected
}

// cosineSimilarity computes the cosine similarity between two vectors.
// Returns 0 if either vector has zero magnitude.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
