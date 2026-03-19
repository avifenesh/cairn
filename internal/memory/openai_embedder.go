package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"
)

const maxEmbedInputLen = 8192

// OpenAIEmbedder implements Embedder using the OpenAI-compatible /embeddings API.
// Works with Z.ai (GLM), OpenAI, Ollama, vLLM, and any compatible provider.
type OpenAIEmbedder struct {
	apiKey     string
	baseURL    string // e.g. "https://api.z.ai/api/coding/paas/v4"
	model      string // e.g. "embedding-3"
	dimensions int
	client     *http.Client
}

// NewOpenAIEmbedder creates an embedder for any OpenAI-compatible API.
func NewOpenAIEmbedder(apiKey, baseURL, model string, dimensions int) *OpenAIEmbedder {
	return &OpenAIEmbedder{
		apiKey:     apiKey,
		baseURL:    strings.TrimRight(baseURL, "/"),
		model:      model,
		dimensions: dimensions,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *OpenAIEmbedder) Dimensions() int { return e.dimensions }

// Embed sends texts to the /embeddings endpoint and returns one vector per input.
func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// Truncate long texts (rune-safe to avoid splitting UTF-8 characters).
	truncated := make([]string, len(texts))
	for i, t := range texts {
		if len(t) > maxEmbedInputLen {
			runes := []rune(t)
			if len(runes) > maxEmbedInputLen {
				runes = runes[:maxEmbedInputLen]
			}
			truncated[i] = string(runes)
		} else {
			truncated[i] = t
		}
	}

	body, err := json.Marshal(embeddingRequest{
		Model: e.model,
		Input: truncated,
	})
	if err != nil {
		return nil, fmt.Errorf("embedding: marshal request: %w", err)
	}

	url := e.baseURL + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embedding: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("embedding: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding: HTTP %d: %s", resp.StatusCode, truncateStr(string(respBody), 200))
	}

	var result embeddingResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("embedding: decode response: %w", err)
	}

	if len(result.Data) != len(texts) {
		return nil, fmt.Errorf("embedding: expected %d results, got %d", len(texts), len(result.Data))
	}

	// Sort by index (API may return out of order).
	sort.Slice(result.Data, func(i, j int) bool {
		return result.Data[i].Index < result.Data[j].Index
	})

	// Warn if API returns different dimensions than configured.
	if len(result.Data) > 0 && len(result.Data[0].Embedding) > 0 &&
		len(result.Data[0].Embedding) != e.dimensions {
		slog.Warn("embedding: dimension mismatch",
			"configured", e.dimensions,
			"actual", len(result.Data[0].Embedding),
			"model", e.model,
		)
	}

	vecs := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		vecs[i] = d.Embedding
	}
	return vecs, nil
}

type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingResponse struct {
	Data []embeddingData `json:"data"`
}

type embeddingData struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
