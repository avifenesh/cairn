package server

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/avifenesh/cairn/internal/agent"
	"github.com/avifenesh/cairn/internal/eventbus"
)

// SSEBroadcaster manages SSE client connections, subscribes to bus events,
// and fans out formatted SSE messages to all connected clients.
type SSEBroadcaster struct {
	clients sync.Map // clientID -> *sseClient
	replay  *replayBuffer
	bus     *eventbus.Bus
	logger  *slog.Logger
	unsubs  []func() // bus unsubscribe functions
	closed  atomic.Bool
}

// sseClient represents a single SSE connection.
type sseClient struct {
	id     string
	events chan []byte
	done   chan struct{}
}

// replayBuffer stores the last N events for reconnection replay.
type replayBuffer struct {
	mu     sync.Mutex
	events []replayEvent
	maxLen int
}

// replayEvent holds a formatted SSE event with its ID for replay filtering.
type replayEvent struct {
	id   string
	data []byte // fully formatted SSE message (id: + event: + data: + \n\n)
}

// NewSSEBroadcaster creates a broadcaster that will subscribe to the given bus.
func NewSSEBroadcaster(bus *eventbus.Bus, logger *slog.Logger) *SSEBroadcaster {
	if logger == nil {
		logger = slog.Default()
	}
	return &SSEBroadcaster{
		replay: &replayBuffer{maxLen: 1000},
		bus:    bus,
		logger: logger,
	}
}

// Start subscribes to bus events and begins broadcasting. Must be called
// before ServeHTTP to receive events.
func (b *SSEBroadcaster) Start() {
	if b.bus == nil {
		return
	}

	// Subscribe to all event types we want to broadcast.
	b.unsubs = append(b.unsubs,
		eventbus.Subscribe(b.bus, func(e eventbus.EventIngested) {
			b.broadcast("feed_update", e.ID, map[string]any{
				"sourceType": e.SourceType,
				"title":      e.Title,
				"url":        e.URL,
			})
		}),
		eventbus.Subscribe(b.bus, func(e eventbus.TaskCreated) {
			b.broadcast("task_update", e.ID, map[string]any{
				"taskId":      e.TaskID,
				"type":        e.Type,
				"description": e.Description,
				"status":      "queued",
			})
		}),
		eventbus.Subscribe(b.bus, func(e eventbus.TaskRunning) {
			b.broadcast("task_update", e.ID, map[string]any{
				"taskId": e.TaskID,
				"status": "running",
			})
		}),
		eventbus.Subscribe(b.bus, func(e eventbus.TaskCompleted) {
			b.broadcast("task_update", e.ID, map[string]any{
				"taskId": e.TaskID,
				"status": "completed",
				"result": e.Result,
			})
		}),
		eventbus.Subscribe(b.bus, func(e eventbus.TaskFailed) {
			b.broadcast("task_update", e.ID, map[string]any{
				"taskId": e.TaskID,
				"status": "failed",
				"error":  e.Error,
			})
		}),
		eventbus.Subscribe(b.bus, func(e eventbus.TextDelta) {
			b.broadcast("assistant_delta", e.ID, map[string]any{
				"taskId":    e.TaskID,
				"deltaText": e.Text,
			})
		}),
		eventbus.Subscribe(b.bus, func(e eventbus.StreamEnded) {
			b.broadcast("assistant_end", e.ID, map[string]any{
				"taskId":       e.TaskID,
				"inputTokens":  e.InputTokens,
				"outputTokens": e.OutputTokens,
				"finishReason": e.FinishReason,
			})
		}),
		eventbus.Subscribe(b.bus, func(e eventbus.ReasoningDelta) {
			b.broadcast("assistant_reasoning", e.ID, map[string]any{
				"taskId": e.TaskID,
				"text":   e.Text,
				"round":  e.Round,
			})
		}),
		eventbus.Subscribe(b.bus, func(e eventbus.ToolCallEvent) {
			b.broadcast("assistant_tool_call", e.ID, map[string]any{
				"taskId":   e.TaskID,
				"toolName": e.ToolName,
				"phase":    e.Phase,
			})
		}),
		eventbus.Subscribe(b.bus, func(e eventbus.ToolExecuted) {
			b.broadcast("tool_executed", e.ID, map[string]any{
				"taskId":     e.TaskID,
				"toolName":   e.ToolName,
				"durationMs": e.DurationMs,
				"isError":    e.IsError,
				"output":     e.Output,
				"error":      e.Error,
			})
		}),
		eventbus.Subscribe(b.bus, func(e eventbus.MemoryProposed) {
			b.broadcast("memory_proposed", e.ID, map[string]any{
				"memoryId": e.MemoryID,
				"content":  e.Content,
			})
		}),
		eventbus.Subscribe(b.bus, func(e eventbus.SoulPatchProposed) {
			b.broadcast("soul_patch_proposed", e.ID, map[string]any{
				"patchId": e.PatchID,
			})
		}),
		eventbus.Subscribe(b.bus, func(e agent.AgentHeartbeat) {
			b.broadcast("agent_heartbeat", e.ID, map[string]any{
				"tickNumber": e.TickNumber,
				"taskRun":    e.TaskRun,
				"durationMs": e.DurationMs,
			})
		}),
		eventbus.Subscribe(b.bus, func(e agent.AgentActivityEvent) {
			b.broadcast("agent_activity", e.ID, map[string]any{
				"entry": e.Entry,
			})
		}),
		eventbus.Subscribe(b.bus, func(e eventbus.MCPConnectionChanged) {
			b.broadcast("mcp_connection", e.ID, map[string]any{
				"serverName": e.ServerName,
				"status":     e.Status,
				"toolCount":  e.ToolCount,
				"error":      e.Error,
			})
		}),
		eventbus.Subscribe(b.bus, func(e eventbus.SessionEvent) {
			// Only broadcast low-frequency session events globally to avoid
			// flooding the main SSE stream. High-frequency text_delta/thinking
			// events are available via /v1/sessions/{id}/stream.
			switch e.EventType {
			case "state_change", "tool_call", "tool_result", "file_change", "round_complete", "user_steer":
				b.broadcast("session_event", e.ID, map[string]any{
					"sessionId": e.SessionID,
					"eventType": e.EventType,
					"payload":   e.Payload,
				})
			}
		}),
		// Rule execution events.
		eventbus.Subscribe(b.bus, func(e eventbus.RuleExecuted) {
			b.broadcast("rule_executed", e.ID, map[string]any{
				"ruleId":   e.RuleID,
				"ruleName": e.RuleName,
				"status":   e.Status,
			})
		}),
		// Subagent lifecycle events.
		eventbus.Subscribe(b.bus, func(e eventbus.SubagentStarted) {
			b.broadcast("subagent_started", e.ID, map[string]any{
				"parentTaskId": e.ParentTaskID,
				"subagentId":   e.SubagentID,
				"agentType":    e.AgentType,
				"execMode":     e.ExecMode,
				"instruction":  e.Instruction,
			})
		}),
		eventbus.Subscribe(b.bus, func(e eventbus.SubagentProgress) {
			b.broadcast("subagent_progress", e.ID, map[string]any{
				"subagentId": e.SubagentID,
				"round":      e.Round,
				"maxRounds":  e.MaxRounds,
				"toolName":   e.ToolName,
			})
		}),
		eventbus.Subscribe(b.bus, func(e eventbus.SubagentCompleted) {
			b.broadcast("subagent_completed", e.ID, map[string]any{
				"subagentId": e.SubagentID,
				"status":     e.Status,
				"summary":    e.Summary,
				"error":      e.Error,
				"durationMs": e.DurationMs,
				"toolCalls":  e.ToolCalls,
				"rounds":     e.Rounds,
			})
		}),
	)

	b.logger.Info("sse broadcaster started")
}

// Close unsubscribes from all bus events and closes all connected clients.
func (b *SSEBroadcaster) Close() {
	if !b.closed.CompareAndSwap(false, true) {
		return
	}

	for _, unsub := range b.unsubs {
		unsub()
	}
	b.unsubs = nil

	b.clients.Range(func(key, value any) bool {
		if client, ok := value.(*sseClient); ok {
			close(client.done)
		}
		b.clients.Delete(key)
		return true
	})

	b.logger.Info("sse broadcaster closed")
}

// ServeHTTP implements the SSE endpoint handler.
func (b *SSEBroadcaster) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable Nginx buffering

	clientID := generateClientID()
	client := &sseClient{
		id:     clientID,
		events: make(chan []byte, 256),
		done:   make(chan struct{}),
	}

	b.clients.Store(clientID, client)
	b.logger.Debug("sse client connected", "clientId", clientID)

	// Send retry directive.
	fmt.Fprintf(w, "retry: 5000\n\n")
	flusher.Flush()

	// Send ready event.
	readyData, _ := json.Marshal(map[string]string{"clientId": clientID})
	fmt.Fprintf(w, "event: ready\ndata: %s\n\n", readyData)
	flusher.Flush()

	// Replay missed events if Last-Event-ID is provided.
	if lastID := r.Header.Get("Last-Event-ID"); lastID != "" {
		b.replayFrom(w, flusher, lastID)
	}

	// Stream events until client disconnects or broadcaster closes.
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			b.removeClient(clientID)
			return
		case <-client.done:
			b.removeClient(clientID)
			return
		case msg, ok := <-client.events:
			if !ok {
				return
			}
			w.Write(msg)
			flusher.Flush()
		}
	}
}

// broadcast formats an SSE event and sends it to all connected clients.
func (b *SSEBroadcaster) broadcast(eventType, eventID string, payload map[string]any) {
	if b.closed.Load() {
		return
	}

	data, err := json.Marshal(payload)
	if err != nil {
		b.logger.Warn("sse: failed to marshal event", "type", eventType, "error", err)
		return
	}

	// Format as SSE message.
	msg := formatSSE(eventID, eventType, data)

	// Store in replay buffer.
	b.replay.add(eventID, msg)

	// Fan out to all clients.
	b.clients.Range(func(key, value any) bool {
		client, ok := value.(*sseClient)
		if !ok {
			return true
		}

		select {
		case client.events <- msg:
		default:
			// Client buffer full — drop the event to avoid blocking.
			b.logger.Debug("sse: dropping event for slow client", "clientId", client.id, "type", eventType)
		}
		return true
	})
}

// BroadcastRaw sends a pre-formatted event to all clients. Useful for
// events not originating from the bus.
func (b *SSEBroadcaster) BroadcastRaw(eventType, eventID string, payload map[string]any) {
	b.broadcast(eventType, eventID, payload)
}

// replayFrom sends all buffered events after lastID to the client.
func (b *SSEBroadcaster) replayFrom(w http.ResponseWriter, flusher http.Flusher, lastID string) {
	b.replay.mu.Lock()
	events := make([]replayEvent, len(b.replay.events))
	copy(events, b.replay.events)
	b.replay.mu.Unlock()

	found := false
	for _, ev := range events {
		if !found {
			if ev.id == lastID {
				found = true
			}
			continue
		}
		w.Write(ev.data)
		flusher.Flush()
	}
}

func (b *SSEBroadcaster) removeClient(id string) {
	b.clients.Delete(id)
	b.logger.Debug("sse client disconnected", "clientId", id)
}

// --- Replay Buffer ---

func (rb *replayBuffer) add(id string, data []byte) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.events = append(rb.events, replayEvent{id: id, data: data})

	// Trim to max length.
	if len(rb.events) > rb.maxLen {
		// Drop the oldest events.
		excess := len(rb.events) - rb.maxLen
		rb.events = rb.events[excess:]
	}
}

// --- SSE formatting ---

// formatSSE builds a complete SSE message with id, event, and data lines.
func formatSSE(id, eventType string, data []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString("id: ")
	buf.WriteString(id)
	buf.WriteByte('\n')
	buf.WriteString("event: ")
	buf.WriteString(eventType)
	buf.WriteByte('\n')
	buf.WriteString("data: ")
	buf.Write(data)
	buf.WriteString("\n\n")
	return buf.Bytes()
}

func generateClientID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return fmt.Sprintf("sse-%x", b)
}
