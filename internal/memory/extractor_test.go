package memory

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/avifenesh/cairn/internal/llm"
)

// mockLLMProvider returns a fixed response for extraction tests.
type mockLLMProvider struct {
	response string
}

func (m *mockLLMProvider) ID() string              { return "mock" }
func (m *mockLLMProvider) Models() []llm.ModelInfo { return nil }
func (m *mockLLMProvider) Stream(_ context.Context, _ *llm.Request) (<-chan llm.Event, error) {
	ch := make(chan llm.Event, 2)
	ch <- llm.TextDelta{Text: m.response}
	ch <- llm.MessageEnd{FinishReason: "stop"}
	close(ch)
	return ch, nil
}

func TestExtractor_ExtractsPreferences(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	mock := &testEmbedder{dims: 3, vec: []float32{0.1, 0.2, 0.3}}
	svc := NewService(store, mock, nil)

	facts := []extractedFact{
		{Content: "User prefers dark mode in all editors", Category: "preference"},
		{Content: "Always use feature branches", Category: "hard_rule"},
	}
	resp, _ := json.Marshal(facts)

	llmMock := &mockLLMProvider{response: string(resp)}
	ext := NewExtractor(svc, llmMock, "mock", slog.Default())

	ctx := context.Background()
	ext.Extract(ctx, "User: I always use dark mode\nAssistant: Noted!\nUser: And always branch\nAssistant: Got it")

	// Check that proposed memories were created.
	proposed, err := store.List(ctx, ListOpts{Status: StatusProposed})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposed) != 2 {
		t.Fatalf("expected 2 proposed memories, got %d", len(proposed))
	}

	// Verify categories.
	found := map[Category]bool{}
	for _, m := range proposed {
		found[m.Category] = true
		if m.Source != "auto-extract" {
			t.Fatalf("expected source 'auto-extract', got %q", m.Source)
		}
	}
	if !found[CatPreference] || !found[CatHardRule] {
		t.Fatalf("expected preference and hard_rule categories, got %v", found)
	}
}

func TestExtractor_SkipsShortTranscripts(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	svc := NewService(store, NoopEmbedder{}, nil)

	ext := NewExtractor(svc, &mockLLMProvider{response: "[]"}, "mock", slog.Default())
	ext.Extract(context.Background(), "") // Empty transcript

	proposed, _ := store.List(context.Background(), ListOpts{Status: StatusProposed})
	if len(proposed) != 0 {
		t.Fatalf("expected 0 proposed for empty transcript, got %d", len(proposed))
	}
}

func TestExtractor_ClassifySkipsDuplicates(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	// Same vector for everything → cosine similarity = 1.0.
	mock := &testEmbedder{dims: 3, vec: []float32{0.5, 0.5, 0.5}}
	svc := NewService(store, mock, nil)
	ctx := context.Background()

	// Create and accept existing memory.
	existing := &Memory{Content: "User prefers dark mode", Category: CatPreference}
	if err := svc.Create(ctx, existing); err != nil {
		t.Fatal(err)
	}
	if err := svc.Accept(ctx, existing.ID); err != nil {
		t.Fatal(err)
	}

	// classifyFact should detect duplicate via vector similarity.
	ext := NewExtractor(svc, &mockLLMProvider{response: "[]"}, "mock", slog.Default())
	action := ext.classifyFact(ctx, extractedFact{
		Content:  "User prefers dark mode",
		Category: "preference",
	})
	if action != "skip" {
		t.Fatalf("expected 'skip' for duplicate, got %q", action)
	}
}

func TestExtractor_InvalidJSON(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	svc := NewService(store, NoopEmbedder{}, nil)

	// LLM returns invalid JSON — should not crash.
	ext := NewExtractor(svc, &mockLLMProvider{response: "not valid json"}, "mock", slog.Default())
	ext.Extract(context.Background(), "User: test\nAssistant: reply\nUser: more\nAssistant: done")

	proposed, _ := store.List(context.Background(), ListOpts{Status: StatusProposed})
	if len(proposed) != 0 {
		t.Fatalf("expected 0 proposed for invalid JSON, got %d", len(proposed))
	}
}

func TestExtractor_EmptyArray(t *testing.T) {
	d := openTestDB(t)
	store := NewStore(d)
	svc := NewService(store, NoopEmbedder{}, nil)

	// LLM returns empty array — nothing to extract.
	ext := NewExtractor(svc, &mockLLMProvider{response: "[]"}, "mock", slog.Default())
	ext.Extract(context.Background(), "User: hi\nAssistant: hello\nUser: bye\nAssistant: goodbye")

	proposed, _ := store.List(context.Background(), ListOpts{Status: StatusProposed})
	if len(proposed) != 0 {
		t.Fatalf("expected 0 proposed for empty array, got %d", len(proposed))
	}
}
