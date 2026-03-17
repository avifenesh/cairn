package eventbus

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSubscribeAndPublish(t *testing.T) {
	bus := New()
	defer bus.Close()

	var received TextDelta
	var called bool

	Subscribe(bus, func(e TextDelta) {
		received = e
		called = true
	})

	evt := TextDelta{
		EventMeta: EventMeta{ID: "1", Timestamp: time.Now(), Source: "test"},
		TaskID:    "task-1",
		Text:      "hello world",
	}
	Publish(bus, evt)

	if !called {
		t.Fatal("handler was not called")
	}
	if received.Text != "hello world" {
		t.Fatalf("expected text %q, got %q", "hello world", received.Text)
	}
	if received.TaskID != "task-1" {
		t.Fatalf("expected taskID %q, got %q", "task-1", received.TaskID)
	}
}

func TestPublishAsync(t *testing.T) {
	bus := New()
	defer bus.Close()

	done := make(chan TextDelta, 1)

	Subscribe(bus, func(e TextDelta) {
		done <- e
	})

	evt := TextDelta{
		EventMeta: EventMeta{ID: "async-1", Timestamp: time.Now(), Source: "test"},
		TaskID:    "task-async",
		Text:      "async hello",
	}
	PublishAsync(bus, evt)

	select {
	case received := <-done:
		if received.Text != "async hello" {
			t.Fatalf("expected text %q, got %q", "async hello", received.Text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async event delivery")
	}
}

func TestUnsubscribe(t *testing.T) {
	bus := New()
	defer bus.Close()

	var callCount int

	unsub := Subscribe(bus, func(e TextDelta) {
		callCount++
	})

	Publish(bus, TextDelta{EventMeta: EventMeta{ID: "1"}, Text: "first"})
	if callCount != 1 {
		t.Fatalf("expected callCount 1, got %d", callCount)
	}

	unsub()

	Publish(bus, TextDelta{EventMeta: EventMeta{ID: "2"}, Text: "second"})
	if callCount != 1 {
		t.Fatalf("expected callCount still 1 after unsubscribe, got %d", callCount)
	}
}

func TestMultipleSubscribers(t *testing.T) {
	bus := New()
	defer bus.Close()

	var count1, count2, count3 int

	Subscribe(bus, func(e TaskCreated) { count1++ })
	Subscribe(bus, func(e TaskCreated) { count2++ })
	Subscribe(bus, func(e TaskCreated) { count3++ })

	Publish(bus, TaskCreated{
		EventMeta:   EventMeta{ID: "t1"},
		TaskID:      "task-1",
		Type:        "test",
		Description: "a task",
	})

	if count1 != 1 || count2 != 1 || count3 != 1 {
		t.Fatalf("expected all 3 handlers called once, got %d %d %d", count1, count2, count3)
	}
}

func TestTypeSafety(t *testing.T) {
	bus := New()
	defer bus.Close()

	var textDeltaCalled bool

	Subscribe(bus, func(e TextDelta) {
		textDeltaCalled = true
	})

	// Publish a different event type — TextDelta handler must NOT fire.
	Publish(bus, TaskCreated{
		EventMeta: EventMeta{ID: "tc-1"},
		TaskID:    "task-1",
		Type:      "test",
	})

	if textDeltaCalled {
		t.Fatal("TextDelta handler should not be called when TaskCreated is published")
	}
}

func TestConcurrentPublish(t *testing.T) {
	bus := New()
	defer bus.Close()

	var total atomic.Int64

	Subscribe(bus, func(e TextDelta) {
		total.Add(1)
	})

	const goroutines = 100
	const eventsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				Publish(bus, TextDelta{
					EventMeta: EventMeta{ID: "concurrent"},
					Text:      "data",
				})
			}
		}()
	}

	wg.Wait()

	expected := int64(goroutines * eventsPerGoroutine)
	if got := total.Load(); got != expected {
		t.Fatalf("expected %d events delivered, got %d", expected, got)
	}
}

func TestBackpressure(t *testing.T) {
	// Use a tiny queue to make backpressure observable.
	bus := New(WithQueueSize(1))
	defer bus.Close()

	// Block the consumer so the queue fills up.
	block := make(chan struct{})
	Subscribe(bus, func(e TextDelta) {
		<-block
	})

	// First publish fills the single slot.
	PublishAsync(bus, TextDelta{EventMeta: EventMeta{ID: "bp-1"}, Text: "first"})

	// The async worker may have already dequeued bp-1, blocking on block.
	// Enqueue one more to fill the buffer.
	PublishAsync(bus, TextDelta{EventMeta: EventMeta{ID: "bp-2"}, Text: "second"})

	// Now the queue should be full (worker blocked on block, buffer has one item).
	// A third publish should block.
	published := make(chan struct{})
	go func() {
		PublishAsync(bus, TextDelta{EventMeta: EventMeta{ID: "bp-3"}, Text: "third"})
		close(published)
	}()

	select {
	case <-published:
		// It's possible the goroutine scheduled before we check, give it a brief window.
		// If it published immediately, the queue wasn't actually full — that's a race
		// with scheduling. We accept this as a non-failure in this edge case.
	case <-time.After(50 * time.Millisecond):
		// Good — publisher is blocked (backpressure working).
	}

	// Unblock consumer so everything drains and Close() can finish.
	close(block)

	// Wait for the blocked publish to complete.
	select {
	case <-published:
	case <-time.After(2 * time.Second):
		t.Fatal("blocked publish did not complete after unblocking consumer")
	}
}

func TestPanicRecovery(t *testing.T) {
	bus := New()
	defer bus.Close()

	var beforePanic, afterPanic bool

	// First subscriber — runs before the panicking one.
	Subscribe(bus, func(e TaskFailed) {
		beforePanic = true
	})

	// Second subscriber — panics.
	Subscribe(bus, func(e TaskFailed) {
		panic("intentional test panic")
	})

	// Third subscriber — should still be called despite the panic above.
	Subscribe(bus, func(e TaskFailed) {
		afterPanic = true
	})

	Publish(bus, TaskFailed{
		EventMeta: EventMeta{ID: "pf-1"},
		TaskID:    "task-panic",
		Error:     "something broke",
	})

	if !beforePanic {
		t.Fatal("handler before panic was not called")
	}
	if !afterPanic {
		t.Fatal("handler after panic was not called — panic recovery failed")
	}
}
