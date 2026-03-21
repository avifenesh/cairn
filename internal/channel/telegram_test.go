package channel

import (
	"strings"
	"testing"
)

func TestSplitMessageShort(t *testing.T) {
	text := "short message"
	chunks := splitMessage(text, 100)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != text {
		t.Fatalf("chunk mismatch: got %q", chunks[0])
	}
}

func TestSplitMessageExactLimit(t *testing.T) {
	text := strings.Repeat("a", 100)
	chunks := splitMessage(text, 100)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for exact limit, got %d", len(chunks))
	}
	if len(chunks[0]) != 100 {
		t.Fatalf("expected 100 chars, got %d", len(chunks[0]))
	}
}

func TestSplitMessageOverLimit(t *testing.T) {
	text := strings.Repeat("a", 200)
	chunks := splitMessage(text, 100)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c) > 100 {
			t.Fatalf("chunk %d: %d chars exceeds limit", i, len(c))
		}
	}
	// Verify data is preserved (chunks may have leading whitespace trimmed).
	reassembled := strings.Join(chunks, " ")
	if len(reassembled) < len(text) {
		t.Fatalf("reassembled length %d < original length %d", len(reassembled), len(text))
	}
}

func TestSplitMessageAtNewline(t *testing.T) {
	// Build a text where splitting at newline should be preferred.
	line := strings.Repeat("a", 80)
	text := line + "\n" + strings.Repeat("b", 80)
	chunks := splitMessage(text, 100)

	// Should split at the newline, giving two clean chunks.
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks split at newline, got %d", len(chunks))
	}
	if !strings.HasPrefix(chunks[0], "aaa") {
		t.Fatalf("chunk 0 should start with 'a', got: %s", chunks[0][:10])
	}
	if !strings.HasPrefix(chunks[1], "bbb") {
		t.Fatalf("chunk 1 should start with 'b', got: %s", chunks[1][:10])
	}
}

func TestSplitMessageNoGoodBreakpoint(t *testing.T) {
	// Long string with no newlines or spaces — should hard-split.
	text := strings.Repeat("abcdefgh", 500) // 4000 chars, no whitespace
	chunks := splitMessage(text, 1000)
	for i, c := range chunks {
		if len(c) > 1000 {
			t.Fatalf("chunk %d: %d chars exceeds limit", i, len(c))
		}
	}
	// Verify data is preserved.
	reassembled := strings.Join(chunks, " ")
	if len(reassembled) < len(text) {
		t.Fatalf("reassembled length %d < original length %d", len(reassembled), len(text))
	}
}

func TestSplitMessageEmpty(t *testing.T) {
	chunks := splitMessage("", 100)
	if len(chunks) != 1 || chunks[0] != "" {
		t.Fatal("expected single empty chunk")
	}
}

func TestSplitMessageThreeChunks(t *testing.T) {
	// 9000 chars → should produce 3+ chunks with 3500 limit.
	text := strings.Repeat("word ", 1800) // ~9000 chars
	limit := tgMarkdownV2Limit
	chunks := splitMessage(text, limit)
	if len(chunks) < 3 {
		t.Fatalf("expected 3+ chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c) > limit {
			t.Fatalf("chunk %d: %d chars exceeds limit %d", i, len(c), limit)
		}
	}
}
