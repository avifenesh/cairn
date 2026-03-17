package server

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/avifenesh/cairn/internal/config"
	"github.com/avifenesh/cairn/internal/eventbus"
)

func TestSSE_Connect(t *testing.T) {
	_, ts := newTestServer(t, nil)

	// Connect to SSE stream.
	resp, err := http.Get(ts.URL + "/v1/stream")
	if err != nil {
		t.Fatalf("GET /v1/stream: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected Content-Type: text/event-stream, got %q", ct)
	}

	// Read the retry directive and ready event.
	scanner := bufio.NewScanner(resp.Body)
	var lines []string

	// Collect lines with a timeout.
	done := make(chan struct{})
	go func() {
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
			// After finding the ready event data line, we have enough.
			for _, l := range lines {
				if strings.HasPrefix(l, "data: ") && strings.Contains(l, "clientId") {
					close(done)
					return
				}
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for ready event")
	}

	// Verify we got a ready event with clientId.
	foundReady := false
	for _, l := range lines {
		if strings.HasPrefix(l, "event: ready") {
			foundReady = true
		}
	}
	if !foundReady {
		t.Fatalf("expected ready event, got lines: %v", lines)
	}
}

func TestSSE_BroadcastEvent(t *testing.T) {
	bus := eventbus.New()
	t.Cleanup(bus.Close)

	cfg := &config.Config{}
	srv := New(ServerConfig{
		Config: cfg,
		Bus:    bus,
	})
	srv.sse.Start()
	t.Cleanup(srv.sse.Close)

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	// Connect SSE client.
	resp, err := http.Get(ts.URL + "/v1/stream")
	if err != nil {
		t.Fatalf("GET /v1/stream: %v", err)
	}
	defer resp.Body.Close()

	// Read past the ready event.
	scanner := bufio.NewScanner(resp.Body)
	readyDone := make(chan struct{})
	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "clientId") {
				close(readyDone)
				return
			}
		}
	}()

	select {
	case <-readyDone:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for ready event")
	}

	// Now publish a bus event.
	eventbus.Publish(bus, eventbus.TaskCreated{
		EventMeta:   eventbus.NewMeta("test"),
		TaskID:      "task-123",
		Type:        "chat",
		Description: "test task",
	})

	// Read the broadcast event.
	eventDone := make(chan string)
	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") && strings.Contains(line, "task-123") {
				close(eventDone)
				return
			}
		}
	}()

	select {
	case <-eventDone:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for task_update event")
	}
}

func TestSSE_ReplayBuffer(t *testing.T) {
	bus := eventbus.New()
	t.Cleanup(bus.Close)

	broadcaster := NewSSEBroadcaster(bus, nil)
	broadcaster.Start()
	t.Cleanup(broadcaster.Close)

	// Add events to the replay buffer directly.
	broadcaster.broadcast("task_update", "evt-001", map[string]any{"taskId": "task-1"})
	broadcaster.broadcast("task_update", "evt-002", map[string]any{"taskId": "task-2"})
	broadcaster.broadcast("task_update", "evt-003", map[string]any{"taskId": "task-3"})

	// Verify the replay buffer has the events.
	broadcaster.replay.mu.Lock()
	if len(broadcaster.replay.events) != 3 {
		t.Fatalf("expected 3 replay events, got %d", len(broadcaster.replay.events))
	}
	broadcaster.replay.mu.Unlock()

	// Simulate replay: create a recorder and replay from evt-001 (should get evt-002 and evt-003).
	w := httptest.NewRecorder()
	// httptest.ResponseRecorder implements http.Flusher directly.
	broadcaster.replayFrom(w, w, "evt-001")

	body := w.Body.String()
	if !strings.Contains(body, "task-2") {
		t.Fatalf("replay should contain task-2, got: %s", body)
	}
	if !strings.Contains(body, "task-3") {
		t.Fatalf("replay should contain task-3, got: %s", body)
	}
	if strings.Contains(body, "task-1") {
		t.Fatalf("replay should NOT contain task-1 (it was the last-event-id), got: %s", body)
	}
}

func TestSSE_ReplayBufferMaxLen(t *testing.T) {
	broadcaster := NewSSEBroadcaster(nil, nil)
	broadcaster.replay.maxLen = 5

	// Add 8 events.
	for i := range 8 {
		data, _ := json.Marshal(map[string]int{"n": i})
		broadcaster.replay.add(
			strings.Replace("evt-X", "X", string(rune('0'+i)), 1),
			formatSSE("", "test", data),
		)
	}

	broadcaster.replay.mu.Lock()
	count := len(broadcaster.replay.events)
	broadcaster.replay.mu.Unlock()

	if count != 5 {
		t.Fatalf("expected 5 events in buffer (max), got %d", count)
	}
}
