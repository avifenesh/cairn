package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIEmbedder_SingleText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/embeddings") {
			t.Fatalf("expected /embeddings path, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("missing or wrong auth header")
		}

		var req embeddingRequest
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.Input) != 1 {
			t.Fatalf("expected 1 input, got %d", len(req.Input))
		}
		if req.Model != "embedding-3" {
			t.Fatalf("expected model embedding-3, got %s", req.Model)
		}

		json.NewEncoder(w).Encode(embeddingResponse{
			Data: []embeddingData{
				{Embedding: []float32{0.1, 0.2, 0.3}, Index: 0},
			},
		})
	}))
	defer srv.Close()

	e := NewOpenAIEmbedder("test-key", srv.URL, "embedding-3", 3)
	vecs, err := e.Embed(context.Background(), []string{"hello world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vecs) != 1 {
		t.Fatalf("expected 1 vector, got %d", len(vecs))
	}
	if len(vecs[0]) != 3 {
		t.Fatalf("expected 3 dimensions, got %d", len(vecs[0]))
	}
	if vecs[0][0] != 0.1 || vecs[0][1] != 0.2 || vecs[0][2] != 0.3 {
		t.Fatalf("unexpected embedding values: %v", vecs[0])
	}
}

func TestOpenAIEmbedder_BatchTexts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req embeddingRequest
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.Input) != 3 {
			t.Fatalf("expected 3 inputs, got %d", len(req.Input))
		}

		// Return out of order to test sorting.
		json.NewEncoder(w).Encode(embeddingResponse{
			Data: []embeddingData{
				{Embedding: []float32{0.3}, Index: 2},
				{Embedding: []float32{0.1}, Index: 0},
				{Embedding: []float32{0.2}, Index: 1},
			},
		})
	}))
	defer srv.Close()

	e := NewOpenAIEmbedder("key", srv.URL, "embedding-3", 1)
	vecs, err := e.Embed(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vecs) != 3 {
		t.Fatalf("expected 3 vectors, got %d", len(vecs))
	}
	// Verify order matches input order (sorted by index).
	if vecs[0][0] != 0.1 || vecs[1][0] != 0.2 || vecs[2][0] != 0.3 {
		t.Fatalf("unexpected order: %v, %v, %v", vecs[0], vecs[1], vecs[2])
	}
}

func TestOpenAIEmbedder_Dimensions(t *testing.T) {
	e := NewOpenAIEmbedder("key", "http://localhost", "m", 2048)
	if e.Dimensions() != 2048 {
		t.Fatalf("expected 2048, got %d", e.Dimensions())
	}
}

func TestOpenAIEmbedder_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error": "rate limited"}`))
	}))
	defer srv.Close()

	e := NewOpenAIEmbedder("key", srv.URL, "m", 3)
	_, err := e.Embed(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error for 429")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("expected 429 in error, got: %v", err)
	}
}

func TestOpenAIEmbedder_EmptyInput(t *testing.T) {
	e := NewOpenAIEmbedder("key", "http://localhost", "m", 3)
	vecs, err := e.Embed(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vecs != nil {
		t.Fatalf("expected nil for empty input, got %v", vecs)
	}
}

func TestOpenAIEmbedder_TextTruncation(t *testing.T) {
	var receivedInput string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req embeddingRequest
		json.NewDecoder(r.Body).Decode(&req)
		receivedInput = req.Input[0]
		json.NewEncoder(w).Encode(embeddingResponse{
			Data: []embeddingData{
				{Embedding: []float32{0.1}, Index: 0},
			},
		})
	}))
	defer srv.Close()

	longText := strings.Repeat("x", maxEmbedInputLen+1000)
	e := NewOpenAIEmbedder("key", srv.URL, "m", 1)
	_, err := e.Embed(context.Background(), []string{longText})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(receivedInput) != maxEmbedInputLen {
		t.Fatalf("expected truncated to %d, got %d", maxEmbedInputLen, len(receivedInput))
	}
}

func TestOpenAIEmbedder_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response — context should cancel before we respond.
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	e := NewOpenAIEmbedder("key", srv.URL, "m", 3)
	_, err := e.Embed(ctx, []string{"test"})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
