package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/avifenesh/cairn/internal/agent"
	"github.com/avifenesh/cairn/internal/memory"
	"github.com/avifenesh/cairn/internal/task"
)

// --- JSON helpers ---

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Warn("writeJSON: encode failed", "error", err)
	}
}

func readJSON(r *http.Request, dst any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB max
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	defer r.Body.Close()

	if len(body) == 0 {
		return fmt.Errorf("empty request body")
	}

	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error":   true,
		"message": message,
		"status":  status,
	})
}

// --- Serialization helpers ---

func marshalTasks(tasks []*task.Task) []map[string]any {
	if tasks == nil {
		return []map[string]any{}
	}
	result := make([]map[string]any, len(tasks))
	for i, t := range tasks {
		result[i] = marshalTask(t)
	}
	return result
}

func marshalTask(t *task.Task) map[string]any {
	m := map[string]any{
		"id":        t.ID,
		"type":      string(t.Type),
		"status":    string(t.Status),
		"priority":  int(t.Priority),
		"createdAt": t.CreatedAt.UTC().Format(time.RFC3339),
		"updatedAt": t.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if t.Description != "" {
		m["description"] = t.Description
	}
	if t.Error != "" {
		m["error"] = t.Error
	}
	if t.SessionID != "" {
		m["sessionId"] = t.SessionID
	}
	if t.Mode != "" {
		m["mode"] = t.Mode
	}
	if len(t.Input) > 0 {
		m["input"] = json.RawMessage(t.Input)
	}
	if len(t.Output) > 0 {
		m["output"] = json.RawMessage(t.Output)
	}
	return m
}

func marshalMemories(mems []*memory.Memory) []map[string]any {
	if mems == nil {
		return []map[string]any{}
	}
	result := make([]map[string]any, len(mems))
	for i, m := range mems {
		result[i] = marshalMemory(m)
	}
	return result
}

func marshalMemory(m *memory.Memory) map[string]any {
	result := map[string]any{
		"id":         m.ID,
		"content":    m.Content,
		"category":   string(m.Category),
		"scope":      string(m.Scope),
		"status":     string(m.Status),
		"confidence": m.Confidence,
		"source":     m.Source,
		"useCount":   m.UseCount,
		"createdAt":  m.CreatedAt.UTC().Format(time.RFC3339),
		"updatedAt":  m.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if m.LastUsedAt != nil {
		result["lastUsedAt"] = m.LastUsedAt.UTC().Format(time.RFC3339)
	}
	return result
}

func marshalSessions(sessions []*agent.Session) []map[string]any {
	if sessions == nil {
		return []map[string]any{}
	}
	result := make([]map[string]any, len(sessions))
	for i, s := range sessions {
		result[i] = map[string]any{
			"id":           s.ID,
			"title":        s.Title,
			"mode":         string(s.Mode),
			"messageCount": s.MessageCount,
			"metadata":     s.State,
			"createdAt":    s.CreatedAt.UTC().Format(time.RFC3339),
			"updatedAt":    s.UpdatedAt.UTC().Format(time.RFC3339),
		}
	}
	return result
}

func marshalSession(s *agent.Session) map[string]any {
	events := make([]map[string]any, 0, len(s.Events))
	for _, ev := range s.Events {
		evMap := map[string]any{
			"id":        ev.ID,
			"author":    ev.Author,
			"timestamp": ev.Timestamp.UTC().Format(time.RFC3339),
		}

		var parts []map[string]any
		for _, p := range ev.Parts {
			switch v := p.(type) {
			case agent.TextPart:
				parts = append(parts, map[string]any{"type": "text", "text": v.Text})
			case agent.ReasoningPart:
				parts = append(parts, map[string]any{"type": "reasoning", "text": v.Text})
			case agent.ToolPart:
				tp := map[string]any{
					"type":     "tool",
					"toolName": v.ToolName,
					"callId":   v.CallID,
					"status":   string(v.Status),
				}
				if v.Output != "" {
					tp["output"] = v.Output
				}
				if v.Error != "" {
					tp["error"] = v.Error
				}
				parts = append(parts, tp)
			}
		}
		evMap["parts"] = parts
		events = append(events, evMap)
	}

	return map[string]any{
		"id":        s.ID,
		"title":     s.Title,
		"mode":      string(s.Mode),
		"events":    events,
		"createdAt": s.CreatedAt.UTC().Format(time.RFC3339),
		"updatedAt": s.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

var startTime = time.Now()

type createMemoryRequest struct {
	Content    string   `json:"content"`
	Category   string   `json:"category"`
	Scope      string   `json:"scope"`
	Source     string   `json:"source"`
	Confidence *float64 `json:"confidence,omitempty"`
}

type assistantMessageRequest struct {
	Message   string `json:"message"`
	Mode      string `json:"mode"`
	SessionID string `json:"sessionId"`
}
