package channel

import (
	"strings"
	"testing"
)

func TestNormalize_Telegram(t *testing.T) {
	input := "**hello** world"
	got := Normalize(input, "telegram")
	// Bold should become single * in TelegramV2
	if !strings.Contains(got, "*hello*") {
		t.Fatalf("expected telegram bold, got: %s", got)
	}
	// "world" should have escaped dot-like chars if present
}

func TestNormalize_Slack(t *testing.T) {
	input := "**hello** [click here](https://example.com)"
	got := Normalize(input, "slack")
	if !strings.Contains(got, "*hello*") {
		t.Fatalf("expected slack bold, got: %s", got)
	}
	if !strings.Contains(got, "<https://example.com|click here>") {
		t.Fatalf("expected slack link, got: %s", got)
	}
}

func TestNormalize_Matrix(t *testing.T) {
	input := "**bold** and `code`"
	got := Normalize(input, "matrix")
	if !strings.Contains(got, "<strong>bold</strong>") {
		t.Fatalf("expected HTML bold, got: %s", got)
	}
	if !strings.Contains(got, "<code>code</code>") {
		t.Fatalf("expected HTML code, got: %s", got)
	}
}

func TestNormalize_Discord(t *testing.T) {
	input := "**bold** `code`"
	got := Normalize(input, "discord")
	// Discord uses standard markdown, should be unchanged
	if got != input {
		t.Fatalf("expected passthrough for discord, got: %s", got)
	}
}

func TestNormalize_Plain(t *testing.T) {
	input := "**bold** `code` [link](https://example.com)"
	got := Normalize(input, "plain")
	if strings.Contains(got, "*") || strings.Contains(got, "`") || strings.Contains(got, "[") {
		t.Fatalf("expected stripped markdown, got: %s", got)
	}
	if !strings.Contains(got, "link") {
		t.Fatalf("expected link text preserved, got: %s", got)
	}
}

func TestConvertLinks(t *testing.T) {
	input := "See [docs](https://example.com) and [more](https://other.com)"
	got := convertLinks(input, func(text, url string) string {
		return "<" + url + "|" + text + ">"
	})
	if !strings.Contains(got, "<https://example.com|docs>") {
		t.Fatalf("expected first link converted, got: %s", got)
	}
	if !strings.Contains(got, "<https://other.com|more>") {
		t.Fatalf("expected second link converted, got: %s", got)
	}
}

func TestNormalize_Unknown(t *testing.T) {
	input := "**bold**"
	got := Normalize(input, "unknown")
	if got != input {
		t.Fatalf("expected passthrough for unknown target, got: %s", got)
	}
}
