package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/agent"
	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/memory"
	"github.com/avifenesh/cairn/internal/task"
	"github.com/avifenesh/cairn/internal/tool"
)

// registerRoutes sets up all HTTP route handlers on the server's mux.
func (s *Server) registerRoutes() {
	// Health / readiness — always open.
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /ready", s.handleReady)

	// SSE stream.
	s.mux.HandleFunc("GET /v1/stream", s.sse.ServeHTTP)

	// Feed.
	s.mux.HandleFunc("GET /v1/feed", s.handleListFeed)
	s.mux.HandleFunc("GET /v1/dashboard", s.handleDashboard)

	// Tasks.
	s.mux.HandleFunc("GET /v1/tasks", s.handleListTasks)
	s.mux.HandleFunc("POST /v1/tasks/{id}/cancel", s.handleCancelTask)

	// Approvals.
	s.mux.HandleFunc("GET /v1/approvals", s.handleListApprovals)
	s.mux.HandleFunc("POST /v1/approvals/{id}/approve", s.handleApproveApproval)
	s.mux.HandleFunc("POST /v1/approvals/{id}/deny", s.handleDenyApproval)

	// Memories.
	s.mux.HandleFunc("GET /v1/memories", s.handleListMemories)
	s.mux.HandleFunc("GET /v1/memories/search", s.handleSearchMemories)
	s.mux.HandleFunc("POST /v1/memories", s.handleCreateMemory)
	s.mux.HandleFunc("POST /v1/memories/{id}/accept", s.handleAcceptMemory)
	s.mux.HandleFunc("POST /v1/memories/{id}/reject", s.handleRejectMemory)

	// Assistant / sessions.
	s.mux.HandleFunc("GET /v1/assistant/sessions", s.handleListSessions)
	s.mux.HandleFunc("GET /v1/assistant/sessions/{id}", s.handleGetSession)
	s.mux.HandleFunc("POST /v1/assistant/message", s.rateLimitMiddleware(10, time.Minute, s.handleAssistantMessage))

	// Skills.
	s.mux.HandleFunc("GET /v1/skills", s.handleListSkills)

	// Soul.
	s.mux.HandleFunc("GET /v1/soul", s.handleGetSoul)
	s.mux.HandleFunc("PUT /v1/soul", s.handlePutSoul)

	// Webhooks (optional, wired when WEBHOOK_SECRETS is configured).
	if s.webhooks != nil {
		s.mux.Handle("POST /v1/webhooks/{name}", s.webhooks)
	}

	// System.
	s.mux.HandleFunc("GET /v1/status", s.handleStatus)
	s.mux.HandleFunc("GET /v1/costs", s.handleCosts)
	s.mux.HandleFunc("POST /v1/poll/run", s.handlePollRun)

	// Static files (SPA fallback).
	s.mux.Handle("/", s.staticHandler())
}

// --- Health & Readiness ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":  true,
		"now": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	checks := map[string]any{
		"ok": true,
	}
	if s.tasks != nil {
		checks["tasks"] = "ready"
	}
	if s.memories != nil {
		checks["memories"] = "ready"
	}
	if s.sessions != nil {
		checks["sessions"] = "ready"
	}
	if s.agent != nil {
		checks["agent"] = "ready"
	}
	writeJSON(w, http.StatusOK, checks)
}

// --- Feed ---

func (s *Server) handleListFeed(w http.ResponseWriter, r *http.Request) {
	// Stub: feed events would come from a feed store.
	// For now return empty list.
	writeJSON(w, http.StatusOK, map[string]any{
		"items": []any{},
		"total": 0,
	})
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	result := map[string]any{
		"feed":  []any{},
		"stats": map[string]any{},
	}

	// Populate task counts if available.
	if s.tasks != nil {
		var taskCounts []map[string]any
		for _, status := range []task.TaskStatus{task.StatusQueued, task.StatusRunning, task.StatusCompleted, task.StatusFailed} {
			tasks, err := s.tasks.List(ctx, task.ListOpts{Status: status, Limit: 100})
			if err == nil {
				taskCounts = append(taskCounts, map[string]any{
					"status": string(status),
					"count":  len(tasks),
				})
			}
		}
		result["taskCounts"] = taskCounts
	}

	// Populate memory count if available.
	if s.memories != nil {
		mems, err := s.memories.List(ctx, memory.ListOpts{Limit: 1})
		if err == nil {
			result["stats"].(map[string]any)["memories"] = len(mems)
		}
	}

	writeJSON(w, http.StatusOK, result)
}

// --- Tasks ---

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	if s.tasks == nil {
		writeJSON(w, http.StatusOK, map[string]any{"tasks": []any{}})
		return
	}

	ctx := r.Context()
	opts := task.ListOpts{Limit: 50}

	if statusQ := r.URL.Query().Get("status"); statusQ != "" {
		opts.Status = task.TaskStatus(statusQ)
	}
	if typeQ := r.URL.Query().Get("type"); typeQ != "" {
		opts.Type = task.TaskType(typeQ)
	}
	if limitQ := r.URL.Query().Get("limit"); limitQ != "" {
		if n, err := strconv.Atoi(limitQ); err == nil && n > 0 {
			opts.Limit = n
		}
	}

	tasks, err := s.tasks.List(ctx, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"tasks": marshalTasks(tasks)})
}

func (s *Server) handleCancelTask(w http.ResponseWriter, r *http.Request) {
	if s.tasks == nil {
		writeError(w, http.StatusServiceUnavailable, "task engine not available")
		return
	}

	taskID := r.PathValue("id")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "missing task id")
		return
	}

	if err := s.tasks.Cancel(r.Context(), taskID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- Approvals ---

func (s *Server) handleListApprovals(w http.ResponseWriter, r *http.Request) {
	// Stub: approvals would come from a dedicated store.
	writeJSON(w, http.StatusOK, map[string]any{"approvals": []any{}})
}

func (s *Server) handleApproveApproval(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing approval id")
		return
	}
	// Stub: would dispatch to approval store.
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func (s *Server) handleDenyApproval(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing approval id")
		return
	}
	// Stub: would dispatch to approval store.
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

// --- Memories ---

func (s *Server) handleListMemories(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeJSON(w, http.StatusOK, map[string]any{"memories": []any{}})
		return
	}

	ctx := r.Context()
	opts := memory.ListOpts{Limit: 50}

	if statusQ := r.URL.Query().Get("status"); statusQ != "" {
		opts.Status = memory.Status(statusQ)
	}
	if catQ := r.URL.Query().Get("category"); catQ != "" {
		opts.Category = memory.Category(catQ)
	}
	if limitQ := r.URL.Query().Get("limit"); limitQ != "" {
		if n, err := strconv.Atoi(limitQ); err == nil && n > 0 {
			opts.Limit = n
		}
	}

	mems, err := s.memories.List(ctx, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"memories": marshalMemories(mems)})
}

func (s *Server) handleSearchMemories(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeJSON(w, http.StatusOK, map[string]any{"results": []any{}})
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "missing q parameter")
		return
	}

	limit := 10
	if limitQ := r.URL.Query().Get("limit"); limitQ != "" {
		if n, err := strconv.Atoi(limitQ); err == nil && n > 0 {
			limit = n
		}
	}

	results, err := s.memories.Search(r.Context(), query, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var items []map[string]any
	for _, sr := range results {
		items = append(items, map[string]any{
			"memory": marshalMemory(sr.Memory),
			"score":  sr.Score,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"results": items})
}

type createMemoryRequest struct {
	Content    string   `json:"content"`
	Category   string   `json:"category"`
	Scope      string   `json:"scope"`
	Source     string   `json:"source"`
	Confidence *float64 `json:"confidence,omitempty"`
}

func (s *Server) handleCreateMemory(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeError(w, http.StatusServiceUnavailable, "memory service not available")
		return
	}

	var req createMemoryRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	m := &memory.Memory{
		Content:  req.Content,
		Category: memory.Category(req.Category),
		Scope:    memory.Scope(req.Scope),
		Source:   req.Source,
	}
	if req.Confidence != nil {
		m.Confidence = *req.Confidence
	}

	if err := s.memories.Create(r.Context(), m); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"ok":     true,
		"memory": marshalMemory(m),
	})
}

func (s *Server) handleAcceptMemory(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeError(w, http.StatusServiceUnavailable, "memory service not available")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing memory id")
		return
	}

	if err := s.memories.Accept(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func (s *Server) handleRejectMemory(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeError(w, http.StatusServiceUnavailable, "memory service not available")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing memory id")
		return
	}

	if err := s.memories.Reject(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

// --- Assistant / Sessions ---

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	if s.sessions == nil {
		writeJSON(w, http.StatusOK, map[string]any{"sessions": []any{}})
		return
	}

	limit := 50
	if limitQ := r.URL.Query().Get("limit"); limitQ != "" {
		if n, err := strconv.Atoi(limitQ); err == nil && n > 0 {
			limit = n
		}
	}

	sessions, err := s.sessions.List(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"sessions": marshalSessions(sessions)})
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	if s.sessions == nil {
		writeError(w, http.StatusServiceUnavailable, "session store not available")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing session id")
		return
	}

	session, err := s.sessions.Get(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, marshalSession(session))
}

type assistantMessageRequest struct {
	Message   string `json:"message"`
	Mode      string `json:"mode"`
	SessionID string `json:"sessionId"`
}

func (s *Server) handleAssistantMessage(w http.ResponseWriter, r *http.Request) {
	if s.agent == nil {
		writeError(w, http.StatusServiceUnavailable, "agent not available")
		return
	}
	if s.tasks == nil {
		writeError(w, http.StatusServiceUnavailable, "task engine not available")
		return
	}

	var req assistantMessageRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	mode := tool.ModeTalk
	if req.Mode != "" {
		mode = tool.Mode(req.Mode)
	}

	// Create or load session.
	ctx := r.Context()
	var session *agent.Session

	if req.SessionID != "" && s.sessions != nil {
		var err error
		session, err = s.sessions.Get(ctx, req.SessionID)
		if err != nil {
			s.logger.Warn("failed to load session, creating new", "id", req.SessionID, "error", err)
			session = nil
		}
	}

	if session == nil {
		session = &agent.Session{
			Mode:  mode,
			State: map[string]any{"workDir": "."},
		}
		if s.sessions != nil {
			if err := s.sessions.Create(ctx, session); err != nil {
				s.logger.Warn("failed to create session", "error", err)
			}
		}
		if session.ID == "" {
			session.ID = "ephemeral"
		}
	}

	// Create a task for this assistant invocation.
	taskInput, _ := json.Marshal(map[string]string{
		"message":   req.Message,
		"sessionId": session.ID,
		"mode":      string(mode),
	})

	t, err := s.tasks.Submit(ctx, &task.SubmitRequest{
		Type:        task.TypeChat,
		Priority:    task.PriorityNormal,
		Mode:        string(mode),
		SessionID:   session.ID,
		Input:       taskInput,
		Description: truncate(req.Message, 100),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("task submit: %v", err))
		return
	}

	// Run the agent asynchronously.
	go s.runAgent(session, t, req.Message, mode)

	// Return 202 with the task ID.
	writeJSON(w, http.StatusAccepted, map[string]any{
		"taskId":    t.ID,
		"sessionId": session.ID,
	})
}

// runAgent executes the agent in a background goroutine, publishing events
// to the bus for SSE broadcast.
func (s *Server) runAgent(session *agent.Session, t *task.Task, message string, mode tool.Mode) {
	ctx := context.Background()

	// Mark task as running.
	eventbus.Publish(s.bus, eventbus.TaskRunning{
		EventMeta: eventbus.NewMeta("server"),
		TaskID:    t.ID,
	})

	// Build journal entries for context (last 48h).
	var journalEntries []memory.JournalDigestEntry
	if s.journalStore != nil {
		if entries, err := s.journalStore.Recent(ctx, 48*time.Hour); err != nil {
			s.logger.Warn("journal entries failed, proceeding without", "error", err)
		} else {
			for _, e := range entries {
				journalEntries = append(journalEntries, memory.JournalDigestEntry{
					Summary:   e.Summary,
					Mode:      e.Mode,
					CreatedAt: e.CreatedAt,
					Learnings: e.Learnings,
					Errors:    e.Errors,
				})
			}
		}
	}

	invCtx := &agent.InvocationContext{
		Context:        ctx,
		SessionID:      session.ID,
		UserMessage:    message,
		Mode:           mode,
		Session:        session,
		Tools:          s.tools,
		LLM:            s.llm,
		Memory:         s.memories,
		Soul:           s.soul,
		Bus:            s.bus,
		ContextBuilder: s.contextBuilder,
		JournalEntries: journalEntries,
		Plugins:        s.plugins,
		Config: &agent.AgentConfig{
			Model: s.config.LLMModel,
		},
	}

	var fullText strings.Builder

	for ev := range s.agent.Run(invCtx) {
		if ev.Err != nil {
			slog.Error("agent run error", "task", t.ID, "error", ev.Err)
			s.tasks.Fail(ctx, t.ID, ev.Err)
			return
		}
		if ev.Event == nil {
			continue
		}

		for _, part := range ev.Event.Parts {
			switch p := part.(type) {
			case agent.TextPart:
				if ev.Event.Author != "user" {
					fullText.WriteString(p.Text)
					// Publish text delta for SSE.
					eventbus.Publish(s.bus, eventbus.TextDelta{
						EventMeta: eventbus.NewMeta("agent"),
						TaskID:    t.ID,
						Text:      p.Text,
					})
				}
			case agent.ReasoningPart:
				// Reasoning traces are published separately for SSE.
				// The bus subscriber in sse.go maps these to assistant_reasoning.
			case agent.ToolPart:
				eventbus.Publish(s.bus, eventbus.ToolCallEvent{
					EventMeta: eventbus.NewMeta("agent"),
					TaskID:    t.ID,
					ToolName:  p.ToolName,
					Phase:     string(p.Status),
				})
			}
		}

		// Persist events to session store.
		if s.sessions != nil && ev.Event.Author != "user" {
			s.sessions.AppendEvent(ctx, session.ID, ev.Event)
		}
	}

	// Complete the task.
	output, _ := json.Marshal(map[string]string{"text": fullText.String()})
	if err := s.tasks.Complete(ctx, t.ID, output); err != nil {
		slog.Error("failed to complete task", "task", t.ID, "error", err)
	}
}

// --- Skills ---

func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"skills": []any{}})
}

// --- Soul ---

func (s *Server) handleGetSoul(w http.ResponseWriter, r *http.Request) {
	if s.soul == nil {
		writeError(w, http.StatusNotFound, "soul not configured")
		return
	}

	content := s.soul.Content()
	writeJSON(w, http.StatusOK, map[string]any{
		"content": content,
	})
}

func (s *Server) handlePutSoul(w http.ResponseWriter, r *http.Request) {
	if s.soul == nil {
		writeError(w, http.StatusServiceUnavailable, "soul not configured")
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Write SOUL.md to disk.
	soulPath := s.config.SoulPath
	if soulPath == "" {
		soulPath = "./SOUL.md"
	}

	if err := os.WriteFile(soulPath, []byte(req.Content), 0644); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("write soul: %v", err))
		return
	}

	// Reload in memory.
	if err := s.soul.Load(); err != nil {
		s.logger.Warn("failed to reload soul after write", "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- System ---

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]any{
		"ok":      true,
		"uptime":  time.Since(startTime).String(),
		"version": "0.1.0",
	}
	if s.agent != nil {
		status["agent"] = s.agent.Name()
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleCosts(w http.ResponseWriter, r *http.Request) {
	// Stub: cost tracking would integrate with the LLM budget system.
	writeJSON(w, http.StatusOK, map[string]any{
		"totalUSD":  0.0,
		"today":     0.0,
		"thisMonth": 0.0,
	})
}

func (s *Server) handlePollRun(w http.ResponseWriter, r *http.Request) {
	// Stub: manual poll trigger would integrate with the signal plane.
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "message": "poll triggered"})
}

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
			"id":        s.ID,
			"title":     s.Title,
			"mode":      string(s.Mode),
			"createdAt": s.CreatedAt.UTC().Format(time.RFC3339),
			"updatedAt": s.UpdatedAt.UTC().Format(time.RFC3339),
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
