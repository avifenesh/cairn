package agent

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/avifenesh/cairn/internal/tool"
)

// mockConfigService implements tool.ConfigService for testing.
type mockConfigService struct {
	config map[string]any
	err    error
}

func (m *mockConfigService) PatchConfig(_ context.Context, changes map[string]any) (map[string]any, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.config == nil {
		m.config = make(map[string]any)
	}
	for k, v := range changes {
		m.config[k] = v
	}
	return m.config, nil
}

func (m *mockConfigService) GetConfig(_ context.Context) (map[string]any, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.config, nil
}

// discardWriter is a no-op io.Writer for test loggers.
type discardWriter struct{}

func (d *discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&discardWriter{}, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestIsValidGitHubOwner(t *testing.T) {
	tests := []struct {
		owner   string
		want    bool
		comment string
	}{
		{"avifenesh", true, "normal alphanumeric owner"},
		{"avifenesh-cairn", true, "owner with single hyphen"},
		{"a-b-c", true, "owner with multiple hyphens"},
		{"A", true, "single char"},
		{"a1-b2-c3", true, "alphanumeric with hyphens"},
		{"12345", true, "numeric owner"},
		{"", false, "empty string"},
		{"  ", false, "whitespace only"},
		{"has spaces", false, "spaces in middle"},
		{" has-hyphens ", false, "leading/trailing spaces"},
		{"-has-leading-hyphen", false, "leading hyphen"},
		{"has-trailing-hyphen-", false, "trailing hyphen"},
		{"has--double-hyphen", false, "double hyphen disallowed by GitHub"},
		{"a/b", false, "slash not allowed"},
		{"user@example", false, "special chars not allowed"},
		{"<script>alert(1)</script>", false, "HTML injection attempt"},
		{"evil\nowner", false, "newline in owner"},
		{"evil\rowner", false, "carriage return in owner"},
		{"evil\towner", false, "tab in owner"},
		{strings.Repeat("a", 40), false, "40 chars exceeds max of 39"},
		{strings.Repeat("a", 39), true, "39 chars at the limit"},
		{"a" + strings.Repeat("b", 38), true, "39 chars with leading 'a'"},
	}

	for _, tt := range tests {
		t.Run(tt.comment, func(t *testing.T) {
			got := isValidGitHubOwner(tt.owner)
			if got != tt.want {
				t.Errorf("isValidGitHubOwner(%q) = %v, want %v", tt.owner, got, tt.want)
			}
		})
	}
}

func TestSubagentIdentityInjection(t *testing.T) {
	ctx := context.Background()

	t.Run("normal case - valid owner injected", func(t *testing.T) {
		runner := &SubagentRunner{
			logger:     newTestLogger(),
			toolConfig: &mockConfigService{config: map[string]any{"ghOwner": "avifenesh"}},
		}
		hint := runner.buildSystemHintForTest(ctx, "You are a coder agent.")
		if hint == "" {
			t.Fatal("expected non-empty hint")
		}
		if !strings.Contains(hint, "Canonical Identity") {
			t.Errorf("hint missing Canonical Identity section\n got: %s", hint)
		}
		if !strings.Contains(hint, "avifenesh") {
			t.Errorf("hint missing owner name\n got: %s", hint)
		}
	})

	t.Run("empty ghOwner - no injection", func(t *testing.T) {
		runner := &SubagentRunner{
			logger:     newTestLogger(),
			toolConfig: &mockConfigService{config: map[string]any{"ghOwner": ""}},
		}
		hint := runner.buildSystemHintForTest(ctx, "You are a coder agent.")
		if strings.Contains(hint, "Canonical Identity") {
			t.Errorf("hint should not contain Canonical Identity for empty owner\n got: %s", hint)
		}
	})

	t.Run("missing ghOwner key - no injection", func(t *testing.T) {
		runner := &SubagentRunner{
			logger:     newTestLogger(),
			toolConfig: &mockConfigService{config: map[string]any{}},
		}
		hint := runner.buildSystemHintForTest(ctx, "You are a coder agent.")
		if strings.Contains(hint, "Canonical Identity") {
			t.Errorf("hint should not contain Canonical Identity for missing key\n got: %s", hint)
		}
	})

	t.Run("invalid ghOwner with spaces - no injection", func(t *testing.T) {
		runner := &SubagentRunner{
			logger:     newTestLogger(),
			toolConfig: &mockConfigService{config: map[string]any{"ghOwner": "has spaces in it"}},
		}
		hint := runner.buildSystemHintForTest(ctx, "You are a coder agent.")
		if strings.Contains(hint, "Canonical Identity") {
			t.Errorf("hint should not contain Canonical Identity for invalid owner\n got: %s", hint)
		}
	})

	t.Run("invalid ghOwner with special chars - no injection", func(t *testing.T) {
		runner := &SubagentRunner{
			logger:     newTestLogger(),
			toolConfig: &mockConfigService{config: map[string]any{"ghOwner": "<script>alert(1)</script>"}},
		}
		hint := runner.buildSystemHintForTest(ctx, "You are a coder agent.")
		if strings.Contains(hint, "Canonical Identity") {
			t.Errorf("hint should not contain Canonical Identity for XSS attempt\n got: %s", hint)
		}
	})

	t.Run("invalid ghOwner with newline - no injection", func(t *testing.T) {
		runner := &SubagentRunner{
			logger:     newTestLogger(),
			toolConfig: &mockConfigService{config: map[string]any{"ghOwner": "evil\nowner"}},
		}
		hint := runner.buildSystemHintForTest(ctx, "You are a coder agent.")
		if strings.Contains(hint, "Canonical Identity") {
			t.Errorf("hint should not contain Canonical Identity for owner with newline\n got: %s", hint)
		}
	})

	t.Run("invalid ghOwner too long - no injection", func(t *testing.T) {
		longOwner := strings.Repeat("a", 40)
		runner := &SubagentRunner{
			logger:     newTestLogger(),
			toolConfig: &mockConfigService{config: map[string]any{"ghOwner": longOwner}},
		}
		hint := runner.buildSystemHintForTest(ctx, "You are a coder agent.")
		if strings.Contains(hint, "Canonical Identity") {
			t.Errorf("hint should not contain Canonical Identity for too-long owner\n got: %s", hint)
		}
	})

	t.Run("GetConfig failure - no injection, no panic", func(t *testing.T) {
		runner := &SubagentRunner{
			logger:     newTestLogger(),
			toolConfig: &mockConfigService{err: errors.New("config service unavailable")},
		}
		hint := runner.buildSystemHintForTest(ctx, "You are a coder agent.")
		if hint == "" {
			t.Fatal("expected base prompt even when config fails")
		}
		if strings.Contains(hint, "Canonical Identity") {
			t.Errorf("hint should not contain Canonical Identity when config fails\n got: %s", hint)
		}
	})

	t.Run("nil toolConfig - no injection", func(t *testing.T) {
		runner := &SubagentRunner{
			logger:     newTestLogger(),
			toolConfig: nil,
		}
		hint := runner.buildSystemHintForTest(ctx, "You are a coder agent.")
		if hint != "You are a coder agent." {
			t.Errorf("expected base prompt unchanged, got: %s", hint)
		}
	})

	t.Run("ghOwner wrong type - no injection", func(t *testing.T) {
		runner := &SubagentRunner{
			logger:     newTestLogger(),
			toolConfig: &mockConfigService{config: map[string]any{"ghOwner": 12345}},
		}
		hint := runner.buildSystemHintForTest(ctx, "You are a coder agent.")
		if strings.Contains(hint, "Canonical Identity") {
			t.Errorf("hint should not contain Canonical Identity for non-string owner\n got: %s", hint)
		}
	})

	t.Run("nil logger with invalid owner - no panic", func(t *testing.T) {
		// This tests the real production scenario where NewSubagentRunner
		// provides a default logger, but someone might construct directly.
		runner := &SubagentRunner{
			logger:     nil,
			toolConfig: &mockConfigService{config: map[string]any{"ghOwner": "has spaces"}},
		}
		// Should not panic — buildSystemHint should handle nil logger gracefully
		hint := runner.buildSystemHintForTest(ctx, "You are a coder agent.")
		if strings.Contains(hint, "Canonical Identity") {
			t.Errorf("nil logger: hint should not contain Canonical Identity for invalid owner\n got: %s", hint)
		}
	})
}

// buildSystemHintForTest delegates to the real system-hint builder used in production
// so the tests exercise the actual identity-injection logic instead of re-implementing it.
func (r *SubagentRunner) buildSystemHintForTest(ctx context.Context, basePrompt string) string {
	return r.buildSystemHint(ctx, basePrompt)
}

// Verify that mockConfigService satisfies the tool.ConfigService interface.
var _ tool.ConfigService = (*mockConfigService)(nil)
