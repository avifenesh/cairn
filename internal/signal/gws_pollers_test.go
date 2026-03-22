package signal

import (
	"testing"
	"time"
)

// --- Gmail helper tests ---

func TestGmailPoller_Source(t *testing.T) {
	p := NewGmailPoller(GmailConfig{GWSPath: "/usr/bin/false"})
	if p.Source() != SourceGmail {
		t.Errorf("source = %q, want %q", p.Source(), SourceGmail)
	}
}

func TestGmailPoller_ShouldAutoArchive(t *testing.T) {
	p := NewGmailPoller(GmailConfig{GWSPath: "/usr/bin/false"})
	tests := []struct {
		from    string
		archive bool
	}{
		{"notifications@github.com", true},
		{"noreply@github.com", true},
		{"GitHub <notifications@github.com>", true},
		{"NOTIFICATIONS@GITHUB.COM", true}, // case-insensitive
		{"alice@company.com", false},
		{"github@example.com", false},
		{"", false},
	}
	for _, tt := range tests {
		got := p.shouldAutoArchive(tt.from)
		if got != tt.archive {
			t.Errorf("shouldAutoArchive(%q) = %v, want %v", tt.from, got, tt.archive)
		}
	}
}

func TestParseEmailName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`John Doe <john@example.com>`, "John Doe"},
		{`"Alice Smith" <alice@example.com>`, "Alice Smith"},
		{`<bare@example.com>`, "bare@example.com"},
		{`plain@example.com`, "plain@example.com"},
		{``, ""},
		{`Bob <bob@test.com>`, "Bob"},
	}
	for _, tt := range tests {
		got := parseEmailName(tt.input)
		if got != tt.want {
			t.Errorf("parseEmailName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseEmailDate(t *testing.T) {
	tests := []struct {
		input string
		zero  bool
	}{
		// RFC1123Z
		{"Sat, 22 Mar 2026 10:00:00 +0000", false},
		// RFC1123
		{"Sat, 22 Mar 2026 10:00:00 UTC", false},
		// Custom: single-digit day
		{"Mon, 2 Jan 2006 15:04:05 -0700", false},
		// Custom: single-digit day with MST
		{"Mon, 2 Jan 2006 15:04:05 MST", false},
		// RFC3339
		{"2026-03-22T10:00:00Z", false},
		// No timezone variant
		{"2 Jan 2006 15:04:05 -0700", false},
		// Invalid
		{"not a date", true},
		{"", true},
	}
	for _, tt := range tests {
		got := parseEmailDate(tt.input)
		if tt.zero && !got.IsZero() {
			t.Errorf("parseEmailDate(%q) = %v, want zero", tt.input, got)
		}
		if !tt.zero && got.IsZero() {
			t.Errorf("parseEmailDate(%q) = zero, want non-zero", tt.input)
		}
	}
}

func TestGmailPoller_DefaultFilter(t *testing.T) {
	p := NewGmailPoller(GmailConfig{GWSPath: "/usr/bin/false"})
	if p.filterQuery != "-category:promotions -category:social -category:forums" {
		t.Errorf("filterQuery = %q, want default filter", p.filterQuery)
	}
}

func TestGmailPoller_CustomFilter(t *testing.T) {
	p := NewGmailPoller(GmailConfig{GWSPath: "/usr/bin/false", FilterQuery: "is:important"})
	if p.filterQuery != "is:important" {
		t.Errorf("filterQuery = %q, want 'is:important'", p.filterQuery)
	}
}

// --- Calendar helper tests ---

func TestCalendarPoller_Source(t *testing.T) {
	p := NewCalendarPoller(CalendarConfig{GWSPath: "/usr/bin/false"})
	if p.Source() != SourceCalendar {
		t.Errorf("source = %q, want %q", p.Source(), SourceCalendar)
	}
}

func TestParseCalTime(t *testing.T) {
	tests := []struct {
		input string
		zero  bool
	}{
		// RFC3339 datetime
		{"2026-03-22T14:30:00Z", false},
		{"2026-03-22T14:30:00+05:00", false},
		// Date-only (all-day events)
		{"2026-03-22", false},
		// Invalid
		{"not-a-time", true},
		{"", true},
	}
	for _, tt := range tests {
		got := parseCalTime(tt.input)
		if tt.zero && !got.IsZero() {
			t.Errorf("parseCalTime(%q) = %v, want zero", tt.input, got)
		}
		if !tt.zero && got.IsZero() {
			t.Errorf("parseCalTime(%q) = zero, want non-zero", tt.input)
		}
	}
}

func TestNewCalendarPoller_DefaultLookahead(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{0, 48},
		{-1, 48},
		{24, 24},
		{72, 72},
	}
	for _, tt := range tests {
		p := NewCalendarPoller(CalendarConfig{GWSPath: "/usr/bin/false", LookaheadH: tt.input})
		if p.lookaheadH != tt.want {
			t.Errorf("NewCalendarPoller(lookahead=%d).lookaheadH = %d, want %d", tt.input, p.lookaheadH, tt.want)
		}
	}
}

func TestParseCalTime_UTC(t *testing.T) {
	// Verify parseCalTime returns UTC.
	got := parseCalTime("2026-03-22T14:30:00+05:00")
	if got.Location() != time.UTC {
		t.Errorf("parseCalTime should return UTC, got %v", got.Location())
	}
	// 14:30+05:00 = 09:30 UTC
	if got.Hour() != 9 || got.Minute() != 30 {
		t.Errorf("parseCalTime(14:30+05:00) = %v, want 09:30 UTC", got)
	}
}
