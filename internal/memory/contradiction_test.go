package memory

import (
	"context"
	"testing"

	"github.com/avifenesh/cairn/internal/llm"
)

func TestCheckContradiction_Contradicting(t *testing.T) {
	mock := &mockLLMProvider{response: "YES"}
	result, err := CheckContradiction(context.Background(),
		"User prefers dark mode in all editors",
		"User prefers light mode for better readability",
		mock, "mock")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result {
		t.Fatal("expected contradiction (YES), got false")
	}
}

func TestCheckContradiction_Compatible(t *testing.T) {
	mock := &mockLLMProvider{response: "NO"}
	result, err := CheckContradiction(context.Background(),
		"Project uses Go for backend",
		"Project uses Go 1.25 for backend development",
		mock, "mock")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result {
		t.Fatal("expected compatible (NO), got true")
	}
}

func TestCheckContradiction_Different(t *testing.T) {
	mock := &mockLLMProvider{response: "NO"}
	result, err := CheckContradiction(context.Background(),
		"User prefers dark mode",
		"Always create feature branches",
		mock, "mock")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result {
		t.Fatal("expected different topics (NO), got true")
	}
}

func TestCheckContradiction_LLMError(t *testing.T) {
	// Stream error — should return false (safe default).
	mock := &mockLLMErrorProvider{}
	result, err := CheckContradiction(context.Background(),
		"fact A", "fact B", mock, "mock")
	if err == nil {
		t.Fatal("expected error")
	}
	if result {
		t.Fatal("expected false on error (safe default)")
	}
}

func TestCheckContradiction_NilProvider(t *testing.T) {
	result, err := CheckContradiction(context.Background(),
		"fact A", "fact B", nil, "mock")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result {
		t.Fatal("expected false for nil provider")
	}
}

// mockLLMErrorProvider always returns a stream error.
type mockLLMErrorProvider struct{}

func (m *mockLLMErrorProvider) ID() string              { return "mock-error" }
func (m *mockLLMErrorProvider) Models() []llm.ModelInfo { return nil }
func (m *mockLLMErrorProvider) Stream(_ context.Context, _ *llm.Request) (<-chan llm.Event, error) {
	ch := make(chan llm.Event, 1)
	ch <- llm.StreamError{Err: context.DeadlineExceeded}
	close(ch)
	return ch, nil
}
