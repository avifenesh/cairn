package channel

import (
	"strings"
	"testing"
)

func TestSplitMessage_Short(t *testing.T) {
	chunks := splitMessage("hello", 100)
	if len(chunks) != 1 || chunks[0] != "hello" {
		t.Errorf("expected [hello], got %v", chunks)
	}
}

func TestSplitMessage_ExactLimit(t *testing.T) {
	text := strings.Repeat("a", 100)
	chunks := splitMessage(text, 100)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(chunks))
	}
}

func TestSplitMessage_SplitsOnNewline(t *testing.T) {
	text := "line1\nline2\nline3\nline4"
	chunks := splitMessage(text, 12)
	// "line1\nline2\n" = 12 chars, should split there
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d: %v", len(chunks), chunks)
	}
	if !strings.Contains(chunks[0], "line1") {
		t.Errorf("first chunk should contain line1, got %q", chunks[0])
	}
}

func TestSplitMessage_SplitsOnSpace(t *testing.T) {
	// No newlines, but has spaces - should split at space boundary.
	text := "word1 word2 word3 word4 word5"
	chunks := splitMessage(text, 12)
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d: %v", len(chunks), chunks)
	}
	// Each chunk should not exceed the limit.
	for i, c := range chunks {
		if len(c) > 12 {
			t.Errorf("chunk %d exceeds limit: len=%d", i, len(c))
		}
	}
}

func TestSplitMessage_HardCut(t *testing.T) {
	// No newlines or spaces - must hard cut.
	text := strings.Repeat("x", 30)
	chunks := splitMessage(text, 10)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d: %v", len(chunks), chunks)
	}
	for i, c := range chunks {
		if len(c) > 10 {
			t.Errorf("chunk %d exceeds limit: len=%d", i, len(c))
		}
	}
}

func TestSplitMessage_Empty(t *testing.T) {
	chunks := splitMessage("", 100)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk for empty string, got %d", len(chunks))
	}
}

func TestSplitMessage_TelegramLimits(t *testing.T) {
	// Simulate a 10000-char message split at Telegram's markdown limit.
	text := strings.Repeat("Hello world. ", 800) // ~10400 chars
	chunks := splitMessage(text, telegramMaxMarkdown)
	for i, c := range chunks {
		if len([]rune(c)) > telegramMaxMarkdown {
			t.Errorf("chunk %d exceeds telegram limit: runes=%d", i, len([]rune(c)))
		}
	}
	// Verify all content is preserved by joining chunks.
	joined := strings.Join(chunks, "")
	if joined != text {
		t.Errorf("content not preserved: original len=%d, joined len=%d", len(text), len(joined))
	}
}
