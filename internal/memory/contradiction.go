package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/avifenesh/cairn/internal/llm"
)

const contradictionPrompt = `Do these two statements contradict each other?

Existing memory: "%s"
New information: "%s"

Respond with ONLY "YES" or "NO".
- YES: They are mutually exclusive or conflicting (e.g. "prefers dark mode" vs "prefers light mode")
- NO: They are compatible, the new one refines/extends the old, or they are about different things`

// CheckContradiction asks the LLM whether two statements contradict each other.
// Returns true if they conflict, false if compatible/refinement.
// On error, returns false (safe default — don't reject on failure).
func CheckContradiction(ctx context.Context, existing, proposed string, provider llm.Provider, model string) (bool, error) {
	if provider == nil {
		return false, nil
	}

	prompt := fmt.Sprintf(contradictionPrompt, existing, proposed)

	req := &llm.Request{
		Model: model,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock{Text: prompt}}},
		},
		MaxTokens: 5,
	}

	ch, err := provider.Stream(ctx, req)
	if err != nil {
		return false, fmt.Errorf("contradiction check stream: %w", err)
	}

	var result strings.Builder
	for ev := range ch {
		switch e := ev.(type) {
		case llm.TextDelta:
			result.WriteString(e.Text)
		case llm.StreamError:
			return false, fmt.Errorf("contradiction check error: %w", e.Err)
		}
	}

	answer := strings.TrimSpace(strings.ToUpper(result.String()))
	return strings.HasPrefix(answer, "YES"), nil
}
