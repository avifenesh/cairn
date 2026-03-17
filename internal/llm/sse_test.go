package llm

import (
	"context"
	"strings"
	"testing"
	"time"
)

// collectSSEEvents reads all SSEEvent values from the channel into a slice.
func collectSSEEvents(ch <-chan SSEEvent) []SSEEvent {
	var events []SSEEvent
	for ev := range ch {
		events = append(events, ev)
	}
	return events
}

func TestParseSSE_BasicData(t *testing.T) {
	input := "data: hello\n\n"
	ctx := context.Background()

	ch := ParseSSE(ctx, strings.NewReader(input))
	events := collectSSEEvents(ch)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Data != "hello" {
		t.Errorf("expected data 'hello', got %q", events[0].Data)
	}
	if events[0].Event != "" {
		t.Errorf("expected empty event type, got %q", events[0].Event)
	}
	if events[0].Err != nil {
		t.Errorf("unexpected error: %v", events[0].Err)
	}
}

func TestParseSSE_MultiLineData(t *testing.T) {
	input := "data: line one\ndata: line two\n\n"
	ctx := context.Background()

	ch := ParseSSE(ctx, strings.NewReader(input))
	events := collectSSEEvents(ch)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	expected := "line one\nline two"
	if events[0].Data != expected {
		t.Errorf("expected data %q, got %q", expected, events[0].Data)
	}
}

func TestParseSSE_EventType(t *testing.T) {
	input := "event: done\ndata: {}\n\n"
	ctx := context.Background()

	ch := ParseSSE(ctx, strings.NewReader(input))
	events := collectSSEEvents(ch)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Event != "done" {
		t.Errorf("expected event type 'done', got %q", events[0].Event)
	}
	if events[0].Data != "{}" {
		t.Errorf("expected data '{}', got %q", events[0].Data)
	}
}

func TestParseSSE_CommentIgnored(t *testing.T) {
	input := ": this is a comment\ndata: actual\n\n"
	ctx := context.Background()

	ch := ParseSSE(ctx, strings.NewReader(input))
	events := collectSSEEvents(ch)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Data != "actual" {
		t.Errorf("expected data 'actual', got %q", events[0].Data)
	}
}

func TestParseSSE_DoneSentinel(t *testing.T) {
	// [DONE] should end the stream; data after it should not be emitted.
	input := "data: first\n\ndata: [DONE]\n\ndata: should not see\n\n"
	ctx := context.Background()

	ch := ParseSSE(ctx, strings.NewReader(input))
	events := collectSSEEvents(ch)

	// Should get only the first event; [DONE] stops the stream.
	if len(events) != 1 {
		t.Fatalf("expected 1 event (before [DONE]), got %d: %+v", len(events), events)
	}
	if events[0].Data != "first" {
		t.Errorf("expected data 'first', got %q", events[0].Data)
	}
}

func TestParseSSE_ContextCancellation(t *testing.T) {
	// Use a shared context so that when we cancel, the blocking reader
	// also unblocks (scanner.Scan returns when Read returns an error).
	ctx, cancel := context.WithCancel(context.Background())
	reader := &slowReader{ctx: ctx}

	ch := ParseSSE(ctx, reader)

	// Cancel after a short delay.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	var events []SSEEvent
	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				goto done
			}
			events = append(events, ev)
		case <-timeout:
			t.Fatal("test timed out waiting for channel close")
			return
		}
	}
done:

	// The scanner sees an error from the reader (context.Canceled) and
	// emits it, or the goroutine checks ctx.Done and emits the error.
	// Either way the channel closes. We just need it to not hang.
	// If we got an error event, great. If the channel closed cleanly
	// (scanner.Err returned the error from Read, emitted via SSEEvent.Err),
	// that's also valid.
	if len(events) > 0 {
		last := events[len(events)-1]
		if last.Err == nil {
			t.Error("expected last event to carry a cancellation error")
		}
	}
	// If len(events)==0, the goroutine exited cleanly after scanner.Err
	// returned context.Canceled but didn't emit it as an SSEEvent because
	// the channel send raced with close. That's acceptable — the key
	// invariant is that the channel closed promptly.
}

func TestParseSSE_EventID(t *testing.T) {
	input := "id: 42\ndata: payload\n\n"
	ctx := context.Background()

	ch := ParseSSE(ctx, strings.NewReader(input))
	events := collectSSEEvents(ch)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ID != "42" {
		t.Errorf("expected id '42', got %q", events[0].ID)
	}
}

func TestParseSSE_MultipleEvents(t *testing.T) {
	input := "data: one\n\ndata: two\n\ndata: three\n\n"
	ctx := context.Background()

	ch := ParseSSE(ctx, strings.NewReader(input))
	events := collectSSEEvents(ch)

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	expected := []string{"one", "two", "three"}
	for i, e := range events {
		if e.Data != expected[i] {
			t.Errorf("event %d: expected %q, got %q", i, expected[i], e.Data)
		}
	}
}

func TestParseSSE_NoSpaceAfterColon(t *testing.T) {
	// Per SSE spec, only a single leading space is stripped.
	input := "data:nospace\n\n"
	ctx := context.Background()

	ch := ParseSSE(ctx, strings.NewReader(input))
	events := collectSSEEvents(ch)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Data != "nospace" {
		t.Errorf("expected data 'nospace', got %q", events[0].Data)
	}
}

func TestParseSSE_EmptyDataLine(t *testing.T) {
	// An empty data field should still dispatch an event with empty data.
	input := "data:\n\n"
	ctx := context.Background()

	ch := ParseSSE(ctx, strings.NewReader(input))
	events := collectSSEEvents(ch)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Data != "" {
		t.Errorf("expected empty data, got %q", events[0].Data)
	}
}

// slowReader blocks on Read until its context is cancelled.
type slowReader struct {
	ctx context.Context
}

func (r *slowReader) Read(p []byte) (int, error) {
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}
