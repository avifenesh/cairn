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
		if len([]rune(c)) > 100 {
			t.Fatalf("chunk %d: %d runes exceeds limit", i, len([]rune(c)))
		}
	}
	// Exact preservation: no data loss from splitting.
	reassembled := strings.Join(chunks, "")
	if reassembled != text {
		t.Fatalf("reassembled text does not match original: got len=%d, want len=%d", len(reassembled), len(text))
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
		if len([]rune(c)) > 1000 {
			t.Fatalf("chunk %d: %d runes exceeds limit", i, len([]rune(c)))
		}
	}
	// Exact preservation: no data loss from splitting.
	reassembled := strings.Join(chunks, "")
	if reassembled != text {
		t.Fatalf("reassembled text does not match original: got len=%d, want len=%d", len(reassembled), len(text))
	}
}

func TestSplitMessageEmpty(t *testing.T) {
	chunks := splitMessage("", 100)
	if len(chunks) != 1 || chunks[0] != "" {
		t.Fatal("expected single empty chunk")
	}
}

func TestSplitMessageThreeChunks(t *testing.T) {
	// 9000 chars → should produce 3+ chunks with raw chunk limit.
	text := strings.Repeat("word ", 1800) // ~9000 chars
	limit := tgRawChunkLimit
	chunks := splitMessage(text, limit)
	if len(chunks) < 3 {
		t.Fatalf("expected 3+ chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len([]rune(c)) > limit {
			t.Fatalf("chunk %d: %d runes exceeds limit %d", i, len([]rune(c)), limit)
		}
	}
}

func TestSplitMessageMultibyteUnicode(t *testing.T) {
	// Multi-byte runes (emoji, CJK) near chunk boundary.
	// Each 🧪 is 4 bytes but 1 rune. If splitting is byte-based, it could split mid-rune.
	text := strings.Repeat("🧪", 250) // 250 runes, 1000 bytes
	chunks := splitMessage(text, 100)
	if len(chunks) != 3 { // 100, 100, 50
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		runes := []rune(c)
		if len(runes) > 100 {
			t.Fatalf("chunk %d: %d runes exceeds limit", i, len(runes))
		}
		// Verify no corrupted runes — all runes should be valid emoji.
		for _, r := range runes {
			if r != '🧪' {
				t.Fatalf("chunk %d: corrupted rune %U", i, r)
			}
		}
	}
	// Exact preservation.
	reassembled := strings.Join(chunks, "")
	if reassembled != text {
		t.Fatalf("reassembled text does not match original")
	}
}

func TestSplitMessageCJKNearBoundary(t *testing.T) {
	// CJK characters are 3 bytes each. Test splitting near boundary.
	text := strings.Repeat("漢", 150) // 150 runes, 450 bytes
	chunks := splitMessage(text, 100)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	reassembled := strings.Join(chunks, "")
	if reassembled != text {
		t.Fatalf("reassembled text does not match original")
	}
	for i, c := range chunks {
		runes := []rune(c)
		if len(runes) > 100 {
			t.Fatalf("chunk %d: %d runes exceeds limit", i, len(runes))
		}
		for _, r := range runes {
			if r != '漢' {
				t.Fatalf("chunk %d: corrupted rune %U", i, r)
			}
		}
	}
}

func TestNormalizePerChunkMarkdownV2(t *testing.T) {
	// Verify that escaping each chunk independently produces valid MarkdownV2
	// with no dangling escape sequences at chunk boundaries.
	// Build a raw text with lots of special chars that get escaped.
	words := []string{"hello_world", "1. item", "2. item", "a-b+c", "foo.bar", "[test](http://example.com)"}
	repeatCount := 200
	var raw strings.Builder
	for i := 0; i < repeatCount; i++ {
		for _, w := range words {
			raw.WriteString(w)
			raw.WriteString(" ")
		}
	}
	rawText := raw.String()

	// Split raw text, then escape each chunk.
	limit := tgRawChunkLimit
	chunks := splitMessage(rawText, limit)

	if len(chunks) < 2 {
		t.Fatalf("expected 2+ chunks, got %d", len(chunks))
	}

	for i, chunk := range chunks {
		escaped := Normalize(chunk, "telegram")

		// Each escaped chunk must fit within Telegram limit.
		runes := []rune(escaped)
		if len(runes) > tgMaxMessageChars {
			t.Fatalf("chunk %d: escaped %d runes exceeds limit %d", i, len(runes), tgMaxMessageChars)
		}

		// Verify no dangling backslash at end (would indicate mid-escape split).
		if strings.HasSuffix(escaped, "\\") {
			t.Fatalf("chunk %d: escaped text ends with dangling backslash", i)
		}

		// Verify no unbalanced links in escaped text.
		openLinks := strings.Count(escaped, "[")
		closeLinks := strings.Count(escaped, "]")
		if openLinks != closeLinks {
			t.Fatalf("chunk %d: unbalanced links: %d open, %d close", i, openLinks, closeLinks)
		}
	}

	// With header overhead, verify total with header still fits.
	for _, chunk := range chunks {
		escaped := Normalize(chunk, "telegram")
		header := "─── Part 1/2 ───\n"
		full := header + escaped
		if len([]rune(full)) > tgMaxMessageChars {
			t.Fatalf("escaped chunk with header exceeds limit: %d runes", len([]rune(full)))
		}
	}
}
