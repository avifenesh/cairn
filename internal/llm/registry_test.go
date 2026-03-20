package llm

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestRegistry_RegisterAndResolve(t *testing.T) {
	r := NewRegistry(nil)

	p := NewOpenAIProvider("key", "http://localhost", "gpt-4o")
	r.Register(p)

	provider, model, err := r.Resolve("gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.ID() != "openai" {
		t.Errorf("expected provider 'openai', got %q", provider.ID())
	}
	if model != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %q", model)
	}
}

func TestRegistry_DefaultProvider(t *testing.T) {
	r := NewRegistry(nil)

	glm := NewGLMProvider("key", "http://localhost", "glm-5-turbo")
	oai := NewOpenAIProvider("key", "http://localhost", "gpt-4o")

	r.Register(glm)
	r.Register(oai)

	// First registered becomes default.
	provider, model, err := r.Default()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.ID() != "glm" {
		t.Errorf("expected default provider 'glm', got %q", provider.ID())
	}
	if model != "glm-5-turbo" {
		t.Errorf("expected default model 'glm-5-turbo', got %q", model)
	}
}

func TestRegistry_SetDefault(t *testing.T) {
	r := NewRegistry(nil)

	glm := NewGLMProvider("key", "http://localhost", "glm-5-turbo")
	oai := NewOpenAIProvider("key", "http://localhost", "gpt-4o")

	r.Register(glm)
	r.Register(oai)
	r.SetDefault("openai", "gpt-4o")

	provider, model, err := r.Default()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.ID() != "openai" {
		t.Errorf("expected 'openai', got %q", provider.ID())
	}
	if model != "gpt-4o" {
		t.Errorf("expected 'gpt-4o', got %q", model)
	}
}

func TestRegistry_UnknownModel(t *testing.T) {
	r := NewRegistry(nil)
	_, _, err := r.Resolve("nonexistent")
	if err == nil {
		t.Error("expected error for unknown model")
	}
}

func TestRegistry_Fallback(t *testing.T) {
	r := NewRegistry(nil)

	glm := NewGLMProvider("key", "http://localhost", "glm-5-turbo")
	r.Register(glm)

	// GLM registers both glm-5-turbo and glm-4.7.
	r.SetFallback("glm-5-turbo", "glm-4.7")

	fb, model, ok := r.FallbackFor("glm-5-turbo")
	if !ok {
		t.Fatal("expected fallback to exist")
	}
	if fb.ID() != "glm" {
		t.Errorf("expected fallback provider 'glm', got %q", fb.ID())
	}
	if model != "glm-4.7" {
		t.Errorf("expected fallback model 'glm-4.7', got %q", model)
	}
}

func TestRegistry_NoFallback(t *testing.T) {
	r := NewRegistry(nil)
	_, _, ok := r.FallbackFor("anything")
	if ok {
		t.Error("expected no fallback")
	}
}

func TestRegistry_RegisterFromConfig(t *testing.T) {
	r := NewRegistry(nil)

	err := r.RegisterFromConfig(ProviderConfig{
		Type:    "openai",
		APIKey:  "key",
		BaseURL: "http://localhost",
		Model:   "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	provider, model, err := r.Resolve("gpt-4o-mini")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.ID() != "openai" {
		t.Errorf("expected 'openai', got %q", provider.ID())
	}
	if model != "gpt-4o-mini" {
		t.Errorf("expected 'gpt-4o-mini', got %q", model)
	}
}

func TestRegistry_RegisterFromConfig_GLM(t *testing.T) {
	r := NewRegistry(nil)

	err := r.RegisterFromConfig(ProviderConfig{
		Type:   "glm",
		APIKey: "id.secret",
		Model:  "glm-5-turbo",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	provider, _, err := r.Resolve("glm-5-turbo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.ID() != "glm" {
		t.Errorf("expected 'glm', got %q", provider.ID())
	}
}

func TestRegistry_RegisterFromConfig_Unknown(t *testing.T) {
	r := NewRegistry(nil)
	err := r.RegisterFromConfig(ProviderConfig{Type: "claude"})
	if err == nil {
		t.Error("expected error for unknown type")
	}
}

func TestRegistry_ListProviders(t *testing.T) {
	r := NewRegistry(nil)
	r.Register(NewGLMProvider("key", "http://localhost", "glm-5-turbo"))
	r.Register(NewOpenAIProvider("key", "http://localhost", "gpt-4o"))

	providers := r.ListProviders()
	if len(providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(providers))
	}
}

func TestRegistry_FallbackChain(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	r := NewRegistry(logger)
	r.Register(NewGLMProvider("key", "http://localhost", "glm-5-turbo"))

	// Set up a 3-level chain: glm-5-turbo -> glm-5 -> glm-4.7
	r.SetFallback("glm-5-turbo", "glm-5")
	r.SetFallback("glm-5", "glm-4.7")

	provider, model, err := r.WithRetryAndFallback("glm-5-turbo", DefaultRetryConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "glm-5-turbo" {
		t.Errorf("expected 'glm-5-turbo', got %q", model)
	}

	// The wrapped provider should be a retryProvider chain.
	rp, ok := provider.(*retryProvider)
	if !ok {
		t.Fatalf("expected *retryProvider, got %T", provider)
	}
	// Primary should be glm (for glm-5-turbo).
	if rp.primary.ID() != "glm" {
		t.Errorf("primary provider = %q, want glm", rp.primary.ID())
	}
	// Fallback should itself be a retryProvider (for glm-5 -> glm-4.7).
	fbRp, ok := rp.fallback.(*retryProvider)
	if !ok {
		t.Fatalf("expected fallback to be *retryProvider, got %T", rp.fallback)
	}
	if fbRp.primary.ID() != "glm" {
		t.Errorf("fallback primary = %q, want glm", fbRp.primary.ID())
	}
	// Second-level fallback should be a retryProvider wrapping glm-4.7 with no further fallback.
	fb2Rp, ok := fbRp.fallback.(*retryProvider)
	if !ok {
		t.Fatalf("expected second-level fallback to be *retryProvider, got %T", fbRp.fallback)
	}
	if fb2Rp.fallback != nil {
		t.Error("expected glm-4.7 (tail) to have no further fallback")
	}
}

func TestRegistry_FallbackChain_CyclePrevention(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	r := NewRegistry(logger)
	r.Register(NewOpenAIProvider("key", "http://localhost", "model-a"))

	// Create a cycle: model-a -> model-a
	r.SetFallback("model-a", "model-a")

	provider, _, err := r.WithRetryAndFallback("model-a", DefaultRetryConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should not infinite loop - the chain should have exactly 1 level (no fallback).
	rp, ok := provider.(*retryProvider)
	if !ok {
		t.Fatalf("expected *retryProvider, got %T", provider)
	}
	if rp.fallback != nil {
		t.Error("expected cycle to be broken - no fallback should be set")
	}
}

func TestRegistry_WithRetryAndFallback(t *testing.T) {
	// Set up a mock server that always succeeds.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"id":"1","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	r := NewRegistry(logger)
	r.Register(NewOpenAIProvider("key", server.URL, "test-model"))
	r.SetFallback("test-model", "test-model") // self-fallback for test simplicity

	provider, model, err := r.WithRetryAndFallback("test-model", DefaultRetryConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "test-model" {
		t.Errorf("expected 'test-model', got %q", model)
	}

	// Actually stream to verify the wrapped provider works.
	ch, err := provider.Stream(context.Background(), &Request{
		Messages: []Message{{Role: RoleUser, Content: []ContentBlock{TextBlock{Text: "hi"}}}},
	})
	if err != nil {
		t.Fatalf("stream error: %v", err)
	}

	var text string
	for ev := range ch {
		if td, ok := ev.(TextDelta); ok {
			text += td.Text
		}
	}
	if text != "ok" {
		t.Errorf("expected 'ok', got %q", text)
	}
}
