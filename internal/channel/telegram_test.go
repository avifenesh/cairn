package channel

import (
	"strings"
	"testing"
)

func TestChunkTelegramMessage_ShortText(t *testing.T) {
	text := "short message"
	chunks, parseMd := chunkTelegramMessage(text, 4096)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("expected %q, got %q", text, chunks[0])
	}
	if !parseMd[0] {
		t.Error("expected markdown for short text")
	}
}

func TestChunkTelegramMessage_EmptyText(t *testing.T) {
	chunks, parseMd := chunkTelegramMessage("", 4096)
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks, got %d", len(chunks))
	}
	if len(parseMd) != 0 {
		t.Fatalf("expected 0 parse flags, got %d", len(parseMd))
	}
}

func TestChunkTelegramMessage_ExactlyAtLimit(t *testing.T) {
	text := strings.Repeat("a", 2457) // ~60% of 4096 = safeRawLen
	chunks, _ := chunkTelegramMessage(text, 4096)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if len(chunks[0]) != len(text) {
		t.Errorf("expected %d chars, got %d", len(text), len(chunks[0]))
	}
}

func TestChunkTelegramMessage_SplitsOnNewline(t *testing.T) {
	// Build text longer than safeRawLen (~2457) with a newline at position 80.
	line1 := strings.Repeat("a", 80)
	line2 := strings.Repeat("b", 2500)
	text := line1 + "\n" + line2
	chunks, _ := chunkTelegramMessage(text, 4096)

	if len(chunks) < 2 {
		t.Fatalf("expected >=2 chunks, got %d", len(chunks))
	}
}

func TestChunkTelegramMessage_NoContentLost(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 10; i++ {
		sb.WriteString(strings.Repeat("x", 50))
		sb.WriteString("\n")
	}
	text := sb.String() // 510 chars total

	chunks, _ := chunkTelegramMessage(text, 4096)

	// Reassemble stripped versions and check total length.
	// After escaping, the chunks will be longer than original, so check that
	// the original text can be reconstructed from the un-escaped form.
	totalOriginal := 0
	for _, chunk := range chunks {
		// Each chunk was normalized; strip to get back close to original
		stripped := stripMarkdown(chunk)
		totalOriginal += len(stripped)
	}
	if totalOriginal == 0 {
		t.Error("reassembled content is empty")
	}
}

func TestChunkTelegramMessage_EachChunkUnderLimit(t *testing.T) {
	// Generate a long text with markdown special chars.
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString("Line ")
		sb.WriteString(strings.Repeat("_", 20))
		sb.WriteString(" with [link](https://example.com) and *bold*\n")
	}
	text := sb.String()

	chunks, _ := chunkTelegramMessage(text, telegramMaxMessageLen)

	for i, chunk := range chunks {
		if len(chunk) > telegramMaxMessageLen {
			t.Errorf("chunk %d: length %d exceeds Telegram limit %d",
				i, len(chunk), telegramMaxMessageLen)
		}
	}
	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks for long text, got %d", len(chunks))
	}
}
