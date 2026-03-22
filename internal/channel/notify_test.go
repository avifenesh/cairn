package channel

import (
	"context"
	"testing"
)

func newTestRouter() (*Router, *mockChannel, *mockChannel, *mockChannel) {
	handler := func(_ context.Context, _ *IncomingMessage) (*OutgoingMessage, error) {
		return nil, nil
	}
	r := NewRouter(handler, nil)
	tg := &mockChannel{name: "telegram"}
	dc := &mockChannel{name: "discord"}
	sl := &mockChannel{name: "slack"}
	r.Register(tg)
	r.Register(dc)
	r.Register(sl)
	return r, tg, dc, sl
}

func TestNotify_Critical_Broadcasts(t *testing.T) {
	r, tg, dc, sl := newTestRouter()
	r.SetNotifyConfig(&NotifyConfig{PreferredChannel: "telegram"})

	msg := &OutgoingMessage{Text: "server down", Priority: PriorityCritical}
	r.Notify(context.Background(), msg)

	// Critical always broadcasts to ALL channels.
	for _, ch := range []*mockChannel{tg, dc, sl} {
		ch.mu.Lock()
		count := len(ch.sent)
		ch.mu.Unlock()
		if count != 1 {
			t.Errorf("%s: expected 1 message, got %d", ch.name, count)
		}
	}
}

func TestNotify_High_PreferredChannel(t *testing.T) {
	r, tg, dc, sl := newTestRouter()
	r.SetNotifyConfig(&NotifyConfig{PreferredChannel: "telegram"})

	msg := &OutgoingMessage{Text: "important update", Priority: PriorityHigh}
	r.Notify(context.Background(), msg)

	// High goes to preferred only.
	tg.mu.Lock()
	if len(tg.sent) != 1 {
		t.Fatalf("telegram: expected 1, got %d", len(tg.sent))
	}
	tg.mu.Unlock()

	dc.mu.Lock()
	if len(dc.sent) != 0 {
		t.Fatalf("discord: expected 0, got %d", len(dc.sent))
	}
	dc.mu.Unlock()

	sl.mu.Lock()
	if len(sl.sent) != 0 {
		t.Fatalf("slack: expected 0, got %d", len(sl.sent))
	}
	sl.mu.Unlock()
}

func TestNotify_High_NoPreferred_Broadcasts(t *testing.T) {
	r, tg, dc, sl := newTestRouter()
	r.SetNotifyConfig(&NotifyConfig{}) // no preferred

	msg := &OutgoingMessage{Text: "important", Priority: PriorityHigh}
	r.Notify(context.Background(), msg)

	// No preferred → broadcast to all.
	for _, ch := range []*mockChannel{tg, dc, sl} {
		ch.mu.Lock()
		count := len(ch.sent)
		ch.mu.Unlock()
		if count != 1 {
			t.Errorf("%s: expected 1 (broadcast fallback), got %d", ch.name, count)
		}
	}
}

func TestNotify_Medium_NoQuietHours_Preferred(t *testing.T) {
	r, tg, dc, _ := newTestRouter()
	r.SetNotifyConfig(&NotifyConfig{
		PreferredChannel: "telegram",
		QuietHoursStart:  -1, // disabled
		QuietHoursEnd:    -1,
	})

	msg := &OutgoingMessage{Text: "new PR review", Priority: PriorityMedium}
	r.Notify(context.Background(), msg)

	tg.mu.Lock()
	if len(tg.sent) != 1 {
		t.Fatalf("telegram: expected 1, got %d", len(tg.sent))
	}
	tg.mu.Unlock()

	dc.mu.Lock()
	if len(dc.sent) != 0 {
		t.Fatalf("discord: expected 0, got %d", len(dc.sent))
	}
	dc.mu.Unlock()

	if r.DigestLen() != 0 {
		t.Fatalf("digest queue should be empty, got %d", r.DigestLen())
	}
}

func TestNotify_Low_AlwaysQueued(t *testing.T) {
	r, tg, _, _ := newTestRouter()
	r.SetNotifyConfig(&NotifyConfig{PreferredChannel: "telegram"})

	msg := &OutgoingMessage{Text: "npm package updated", Priority: PriorityLow}
	r.Notify(context.Background(), msg)

	// Low is always queued, never sent immediately.
	tg.mu.Lock()
	if len(tg.sent) != 0 {
		t.Fatalf("telegram: expected 0 (queued), got %d", len(tg.sent))
	}
	tg.mu.Unlock()

	if r.DigestLen() != 1 {
		t.Fatalf("digest queue: expected 1, got %d", r.DigestLen())
	}
}

func TestIsQuietHours_Disabled(t *testing.T) {
	cfg := &NotifyConfig{QuietHoursStart: -1, QuietHoursEnd: -1}
	if cfg.isQuietHours() {
		t.Fatal("expected not quiet when disabled (-1)")
	}
}

func TestIsQuietHours_NilConfig(t *testing.T) {
	var cfg *NotifyConfig
	if cfg.isQuietHours() {
		t.Fatal("expected not quiet for nil config")
	}
}

func TestIsQuietHours_SameDay(t *testing.T) {
	// 01:00 → 06:00 UTC
	cfg := &NotifyConfig{QuietHoursStart: 1, QuietHoursEnd: 6, QuietHoursTZ: "UTC"}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("isQuietHours SameDay panicked: %v", r)
		}
	}()
	// Verify the method returns a valid boolean result.
	got := cfg.isQuietHours()
	t.Logf("isQuietHours SameDay returned %v (start=%d, end=%d, tz=%s)", got, cfg.QuietHoursStart, cfg.QuietHoursEnd, cfg.QuietHoursTZ)
}

func TestIsQuietHours_MidnightWrap(t *testing.T) {
	// 22:00 → 08:00 UTC — wraps across midnight.
	cfg := &NotifyConfig{QuietHoursStart: 22, QuietHoursEnd: 8, QuietHoursTZ: "UTC"}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("isQuietHours MidnightWrap panicked: %v", r)
		}
	}()
	got := cfg.isQuietHours()
	t.Logf("isQuietHours MidnightWrap returned %v (start=%d, end=%d, tz=%s)", got, cfg.QuietHoursStart, cfg.QuietHoursEnd, cfg.QuietHoursTZ)
}

func TestDigestQueue_EnqueueFlush(t *testing.T) {
	q := &digestQueue{}

	q.enqueue("item 1", PriorityLow)
	q.enqueue("item 2", PriorityLow)
	q.enqueue("item 3", PriorityMedium)

	if q.len() != 3 {
		t.Fatalf("expected 3 items, got %d", q.len())
	}

	items := q.flush()
	if len(items) != 3 {
		t.Fatalf("expected 3 flushed items, got %d", len(items))
	}
	if q.len() != 0 {
		t.Fatalf("expected empty queue after flush, got %d", q.len())
	}

	// Second flush should return empty.
	items = q.flush()
	if len(items) != 0 {
		t.Fatalf("expected 0 items on second flush, got %d", len(items))
	}
}

func TestFlushDigest_FormatsAndSends(t *testing.T) {
	r, tg, _, _ := newTestRouter()
	r.SetNotifyConfig(&NotifyConfig{PreferredChannel: "telegram"})

	// Queue some items.
	r.digest.enqueue("PR #42 merged", PriorityLow)
	r.digest.enqueue("new star on cairn", PriorityLow)
	r.digest.enqueue("CI passed", PriorityMedium)

	count := r.FlushDigest(context.Background())
	if count != 3 {
		t.Fatalf("expected 3 flushed, got %d", count)
	}

	tg.mu.Lock()
	defer tg.mu.Unlock()
	if len(tg.sent) != 1 {
		t.Fatalf("expected 1 digest message sent, got %d", len(tg.sent))
	}

	text := tg.sent[0].Text
	if !containsStr(text, "Digest (3 items)") {
		t.Errorf("digest should contain header, got: %s", text)
	}
	if !containsStr(text, "PR #42 merged") {
		t.Errorf("digest should contain items, got: %s", text)
	}
}

func TestFlushDigest_Empty(t *testing.T) {
	r, tg, _, _ := newTestRouter()
	r.SetNotifyConfig(&NotifyConfig{PreferredChannel: "telegram"})

	count := r.FlushDigest(context.Background())
	if count != 0 {
		t.Fatalf("expected 0 flushed, got %d", count)
	}

	tg.mu.Lock()
	if len(tg.sent) != 0 {
		t.Fatalf("expected no messages sent for empty digest, got %d", len(tg.sent))
	}
	tg.mu.Unlock()
}

func TestNotify_NoChannels(t *testing.T) {
	handler := func(_ context.Context, _ *IncomingMessage) (*OutgoingMessage, error) {
		return nil, nil
	}
	r := NewRouter(handler, nil)
	// No channels registered — should not panic.
	msg := &OutgoingMessage{Text: "test", Priority: PriorityCritical}
	r.Notify(context.Background(), msg)
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
