package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/avifenesh/cairn/internal/agent"
	"github.com/avifenesh/cairn/internal/eventbus"
)

// handleSessionStream serves a session-scoped SSE event stream.
// Only events matching the session ID are sent to the client.
// Does not support Last-Event-ID replay; clients will miss events
// that occur while disconnected. Use GET /v1/sessions/{id}/events
// to hydrate prior events on page load.
func (s *Server) handleSessionStream(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session ID required")
		return
	}

	// Verify session exists.
	_, err := s.sessions.Get(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()

	// Subscribe to SessionEvent on the bus, filtering by session ID.
	events := make(chan eventbus.SessionEvent, 64)
	unsub := eventbus.Subscribe(s.bus, func(e eventbus.SessionEvent) {
		if e.SessionID == sessionID {
			select {
			case events <- e:
			default:
				// Drop if client is too slow.
			}
		}
	})
	defer unsub()

	// Keepalive ticker.
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-events:
			data, _ := json.Marshal(map[string]any{
				"id":        ev.ID,
				"sessionId": ev.SessionID,
				"eventType": ev.EventType,
				"payload":   ev.Payload,
				"timestamp": ev.Timestamp,
			})
			// Use stable event ID from EventMeta for SSE id field.
			fmt.Fprintf(w, "id: %s\nevent: session_event\ndata: %s\n\n", ev.ID, data)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// handleSessionEvents returns paginated session events from history.
// Query params: limit (default 100).
func (s *Server) handleSessionEvents(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session ID required")
		return
	}

	// Verify session exists and get its events.
	session, err := s.sessions.Get(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}

	// Convert session events to a serializable format.
	type eventDTO struct {
		ID        string `json:"id"`
		Timestamp string `json:"timestamp"`
		Author    string `json:"author"`
		Round     int    `json:"round"`
		Parts     []any  `json:"parts"`
	}

	events := session.Events
	if len(events) > limit {
		events = events[len(events)-limit:]
	}

	result := make([]eventDTO, 0, len(events))
	for _, ev := range events {
		parts := make([]any, 0, len(ev.Parts))
		for _, p := range ev.Parts {
			parts = append(parts, p)
		}
		result = append(result, eventDTO{
			ID:        ev.ID,
			Timestamp: ev.Timestamp.Format(time.RFC3339Nano),
			Author:    ev.Author,
			Round:     ev.Round,
			Parts:     parts,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"sessionId": sessionID,
		"events":    result,
		"total":     len(session.Events),
	})
}

// handleSessionSteer injects a steering message into an active session.
func (s *Server) handleSessionSteer(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session ID required")
		return
	}

	// Limit body size to 64KB to prevent DoS.
	body := http.MaxBytesReader(w, r.Body, 64<<10)
	defer body.Close()

	var req struct {
		Content  string `json:"content"`
		Priority string `json:"priority"` // normal, stop
	}
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Content == "" && req.Priority != "stop" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	// Validate priority - only "normal" and "stop" are implemented.
	switch req.Priority {
	case "", "normal":
		req.Priority = "normal"
	case "stop":
		// ok
	default:
		writeError(w, http.StatusBadRequest, "priority must be 'normal' or 'stop'")
		return
	}

	// Find the steering channel for this session.
	ch, ok := s.steeringChannels.Load(sessionID)
	if !ok {
		writeError(w, http.StatusNotFound, "no active session with that ID (session may have completed)")
		return
	}

	steerCh, ok := ch.(chan agent.SteeringMessage)
	if !ok {
		writeError(w, http.StatusInternalServerError, "invalid steering channel")
		return
	}

	msg := agent.SteeringMessage{
		Content:  req.Content,
		Priority: req.Priority,
	}

	select {
	case steerCh <- msg:
		writeJSON(w, http.StatusOK, map[string]string{"status": "delivered"})
	default:
		writeError(w, http.StatusServiceUnavailable, "steering channel full, agent may be busy")
	}
}

// RegisterSteeringChannel registers a steering channel for an active session.
// Called when a session starts. The channel should be removed when the session ends.
func (s *Server) RegisterSteeringChannel(sessionID string, ch chan agent.SteeringMessage) {
	s.steeringChannels.Store(sessionID, ch)
}

// UnregisterSteeringChannel removes a steering channel when a session ends.
func (s *Server) UnregisterSteeringChannel(sessionID string) {
	s.steeringChannels.Delete(sessionID)
}
