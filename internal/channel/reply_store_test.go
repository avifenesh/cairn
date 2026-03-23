package channel

import (
	"testing"
	"time"
	"unicode/utf8"
)

func TestReplyStore_SaveAndLookup(t *testing.T) {
	rs := NewReplyStore(0)
	defer rs.Close()

	// Basic save and lookup.
	rs.Save("telegram", "12345", "100", "Hello from cairn")
	got := rs.Lookup("telegram", "12345", "100")
	if got != "Hello from cairn" {
		t.Errorf("Lookup = %q, want %q", got, "Hello from cairn")
	}

	// Different channel should not match.
	got = rs.Lookup("discord", "12345", "100")
	if got != "" {
		t.Errorf("cross-channel Lookup = %q, want empty", got)
	}

	// Different chatID should not match.
	got = rs.Lookup("telegram", "99999", "100")
	if got != "" {
		t.Errorf("cross-chat Lookup = %q, want empty", got)
	}

	// Different messageID should not match.
	got = rs.Lookup("telegram", "12345", "200")
	if got != "" {
		t.Errorf("cross-message Lookup = %q, want empty", got)
	}
}

func TestReplyStore_EmptyMessageID(t *testing.T) {
	rs := NewReplyStore(0)
	defer rs.Close()

	rs.Save("telegram", "12345", "", "should not save")
	got := rs.Lookup("telegram", "12345", "")
	if got != "" {
		t.Errorf("empty messageID Lookup = %q, want empty", got)
	}
}

func TestReplyStore_TTLExpiry(t *testing.T) {
	// Use a very short TTL to test expiry (actual sleep required since we
	// don't inject a clock — acceptable for a store-level unit test).
	rs := NewReplyStore(50 * time.Millisecond)
	defer rs.Close()

	rs.Save("telegram", "1", "10", "expires soon")
	got := rs.Lookup("telegram", "1", "10")
	if got != "expires soon" {
		t.Errorf("Lookup before expiry = %q, want %q", got, "expires soon")
	}

	// Wait for expiry, polling to avoid flakiness on slow/contended runners.
	deadline := time.Now().Add(2 * time.Second)
	for {
		got = rs.Lookup("telegram", "1", "10")
		if got == "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Lookup after expiry timeout = %q, want empty", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestReplyStore_Overwrite(t *testing.T) {
	rs := NewReplyStore(0)
	defer rs.Close()

	rs.Save("telegram", "1", "10", "first")
	rs.Save("telegram", "1", "10", "second")
	got := rs.Lookup("telegram", "1", "10")
	if got != "second" {
		t.Errorf("Lookup after overwrite = %q, want %q", got, "second")
	}
}

func TestReplyStore_KeyFormat(t *testing.T) {
	rs := NewReplyStore(0)
	defer rs.Close()

	// Verify that key format separates components correctly.
	// channel:chatID:messageID
	rs.Save("tg", "chat1", "msg1", "value1")
	rs.Save("tg", "chat1msg1", "", "value2") // empty messageID — should not save

	got := rs.Lookup("tg", "chat1", "msg1")
	if got != "value1" {
		t.Errorf("Lookup = %q, want %q", got, "value1")
	}
}

func TestReplyStore_MultipleEntries(t *testing.T) {
	rs := NewReplyStore(0)
	defer rs.Close()

	// Save multiple entries and verify each.
	entries := []struct {
		channel, chat, msg, content string
	}{
		{"telegram", "100", "1", "hello"},
		{"telegram", "100", "2", "world"},
		{"telegram", "200", "1", "foo"},
		{"discord", "100", "1", "bar"},
	}

	for _, e := range entries {
		rs.Save(e.channel, e.chat, e.msg, e.content)
	}

	for _, e := range entries {
		got := rs.Lookup(e.channel, e.chat, e.msg)
		if got != e.content {
			t.Errorf("Lookup(%s, %s, %s) = %q, want %q", e.channel, e.chat, e.msg, got, e.content)
		}
	}
}

func TestTruncateRune(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		max    int
		want   string
	}{
		{"short", "hello", 10, "hello"},
		{"exact", "hello", 5, "hello"},
		{"truncate", "hello world", 5, "hello..."},
		{"multibyte ascii", "hello", 3, "hel..."},
		{"multibyte emoji", "👋🌍🚀", 2, "👋🌍..."},
		{"mixed", "abc你好世界", 5, "abc你好..."},
		{"empty", "", 5, ""},
		{"zero max", "hello", 0, "..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateRune(tt.input, tt.max)
			if got != tt.want {
				t.Errorf("TruncateRune(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
			}
			// Verify result is valid UTF-8.
			if !utf8.ValidString(got) {
				t.Errorf("TruncateRune(%q, %d) produced invalid UTF-8: %q", tt.input, tt.max, got)
			}
		})
	}
}

func TestReplyStore_OpportunisticExpiry(t *testing.T) {
	rs := NewReplyStore(50 * time.Millisecond)
	defer rs.Close()

	rs.Save("telegram", "1", "10", "expires soon")
	got := rs.Lookup("telegram", "1", "10")
	if got != "expires soon" {
		t.Fatalf("Lookup before expiry = %q, want %q", got, "expires soon")
	}

	// Wait for expiry, then verify Lookup cleans up.
	deadline := time.Now().Add(2 * time.Second)
	for {
		got = rs.Lookup("telegram", "1", "10")
		if got == "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Lookup after expiry timeout = %q, want empty", got)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Verify the entry was actually deleted from the map (not just skipped).
	rs.mu.RLock()
	_, exists := rs.store[key("telegram", "1", "10")]
	rs.mu.RUnlock()
	if exists {
		t.Error("expired entry still exists in store after Lookup")
	}
}
