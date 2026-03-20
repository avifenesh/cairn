package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/agent"
	"github.com/avifenesh/cairn/internal/config"
	"github.com/avifenesh/cairn/internal/cron"
	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/llm"
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
	s.mux.HandleFunc("POST /v1/feed/{id}/read", s.handleMarkFeedRead)
	s.mux.HandleFunc("POST /v1/feed/read-all", s.handleMarkAllFeedRead)
	s.mux.HandleFunc("POST /v1/feed/{id}/archive", s.handleArchiveFeed)
	s.mux.HandleFunc("DELETE /v1/feed/{id}", s.handleDeleteFeed)
	s.mux.HandleFunc("GET /v1/dashboard", s.handleDashboard)

	// Tasks.
	s.mux.HandleFunc("GET /v1/tasks", s.handleListTasks)
	s.mux.HandleFunc("POST /v1/tasks", s.handleCreateTask)
	s.mux.HandleFunc("POST /v1/tasks/{id}/cancel", s.handleCancelTask)
	s.mux.HandleFunc("DELETE /v1/tasks/{id}", s.handleDeleteTask)

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
	s.mux.HandleFunc("DELETE /v1/memories/{id}", s.handleDeleteMemory)
	s.mux.HandleFunc("PUT /v1/memories/{id}", s.handleUpdateMemory)

	// Assistant / sessions.
	s.mux.HandleFunc("GET /v1/assistant/sessions", s.handleListSessions)
	s.mux.HandleFunc("GET /v1/assistant/sessions/{id}", s.handleGetSession)
	s.mux.HandleFunc("POST /v1/assistant/message", s.rateLimitMiddleware(10, time.Minute, s.handleAssistantMessage))
	s.mux.HandleFunc("POST /v1/upload", s.handleUpload)
	s.mux.HandleFunc("GET /v1/config", s.handleGetConfig)
	s.mux.HandleFunc("PATCH /v1/config", s.handlePatchConfig)

	// Skills.
	s.mux.HandleFunc("GET /v1/skills", s.handleListSkills)
	s.mux.HandleFunc("GET /v1/skills/{name}", s.handleGetSkill)
	s.mux.HandleFunc("POST /v1/skills", s.handleCreateSkill)
	s.mux.HandleFunc("PUT /v1/skills/{name}", s.handleUpdateSkill)
	s.mux.HandleFunc("DELETE /v1/skills/{name}", s.handleDeleteSkill)

	// Marketplace (ClawHub).
	if s.marketplace != nil {
		s.mux.HandleFunc("GET /v1/marketplace/search", s.handleMarketplaceSearch)
		s.mux.HandleFunc("GET /v1/marketplace/browse", s.handleMarketplaceBrowse)
		s.mux.HandleFunc("GET /v1/marketplace/skills/{slug}", s.handleMarketplaceDetail)
		s.mux.HandleFunc("GET /v1/marketplace/skills/{slug}/preview", s.handleMarketplacePreview)
		s.mux.HandleFunc("POST /v1/marketplace/skills/{slug}/install", s.handleMarketplaceInstall)
		s.mux.HandleFunc("POST /v1/marketplace/skills/{slug}/review", s.handleMarketplaceReview)
	}

	// Skill suggestions.
	s.mux.HandleFunc("GET /v1/skills/suggestions", s.handleSkillSuggestions)
	s.mux.HandleFunc("POST /v1/skills/suggestions/dismiss", s.handleDismissSkillSuggestion)

	// Soul.
	s.mux.HandleFunc("GET /v1/soul", s.handleGetSoul)
	s.mux.HandleFunc("PUT /v1/soul", s.handlePutSoul)
	s.mux.HandleFunc("GET /v1/soul/patch", s.handleGetSoulPatch)
	s.mux.HandleFunc("POST /v1/soul/patch/approve", s.handleApproveSoulPatch)
	s.mux.HandleFunc("POST /v1/soul/patch/deny", s.handleDenySoulPatch)

	// Cron jobs (optional).
	if s.cronStore != nil {
		s.mux.HandleFunc("GET /v1/crons", s.handleListCrons)
		s.mux.HandleFunc("POST /v1/crons", s.handleCreateCron)
		s.mux.HandleFunc("GET /v1/crons/{id}", s.handleGetCron)
		s.mux.HandleFunc("PATCH /v1/crons/{id}", s.handleUpdateCron)
		s.mux.HandleFunc("DELETE /v1/crons/{id}", s.handleDeleteCron)
	}

	// Agent activity.
	if s.activityStore != nil {
		s.mux.HandleFunc("GET /v1/agent/activity", s.handleAgentActivity)
	}

	// Webhooks (optional, wired when WEBHOOK_SECRETS is configured).
	if s.webhooks != nil {
		s.mux.Handle("POST /v1/webhooks/{name}", s.webhooks)
	}

	// Voice (optional — requires whisper + edge-tts).
	if s.voice != nil {
		s.mux.HandleFunc("POST /v1/assistant/voice", s.handleVoiceTranscribe)
		s.mux.HandleFunc("POST /v1/assistant/voice/tts", s.handleVoiceTTS)
	}

	// Auth (WebAuthn — optional, requires authStore + webauthn).
	if s.webauthn != nil {
		s.mux.HandleFunc("POST /v1/auth/register/start", s.requireWrite(s.handleAuthRegisterStart))
		s.mux.HandleFunc("POST /v1/auth/register/complete", s.requireWrite(s.handleAuthRegisterComplete))
		s.mux.HandleFunc("POST /v1/auth/login/start", s.handleAuthLoginStart)
		s.mux.HandleFunc("POST /v1/auth/login/complete", s.handleAuthLoginComplete)
		s.mux.HandleFunc("POST /v1/auth/logout", s.handleAuthLogout)
		s.mux.HandleFunc("GET /v1/auth/session", s.handleAuthSession)
		s.mux.HandleFunc("GET /v1/auth/credentials", s.requireWrite(s.handleListAuthCredentials))
		s.mux.HandleFunc("DELETE /v1/auth/credentials/{id}", s.requireWrite(s.handleDeleteAuthCredential))
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
	if s.toolEvents == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "hasMore": false})
		return
	}

	q := r.URL.Query()
	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	f := tool.EventFilter{
		Source:          q.Get("source"),
		Kind:            q.Get("kind"),
		UnreadOnly:      q.Get("unread") == "true",
		ExcludeArchived: q.Get("archived") != "true", // exclude archived by default
		Limit:           limit + 1,                   // fetch one extra to determine hasMore
		Before:          q.Get("before"),
	}

	events, err := s.toolEvents.List(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}

	items := make([]map[string]any, 0, len(events))
	for _, ev := range events {
		item := map[string]any{
			"id":         ev.ID,
			"source":     ev.Source,
			"kind":       ev.Kind,
			"title":      ev.Title,
			"body":       ev.Body,
			"url":        ev.URL,
			"author":     ev.Actor,
			"groupKey":   ev.GroupKey,
			"isRead":     ev.ReadAt != nil,
			"isArchived": ev.ArchivedAt != nil,
			"createdAt":  ev.CreatedAt.Format(time.RFC3339),
		}
		if ev.Metadata != nil {
			item["metadata"] = ev.Metadata
		}
		items = append(items, item)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":   items,
		"hasMore": hasMore,
	})
}

func (s *Server) handleMarkFeedRead(w http.ResponseWriter, r *http.Request) {
	if s.toolEvents == nil {
		writeError(w, http.StatusServiceUnavailable, "event service not available")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	if err := s.toolEvents.MarkRead(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleMarkAllFeedRead(w http.ResponseWriter, r *http.Request) {
	if s.toolEvents == nil {
		writeError(w, http.StatusServiceUnavailable, "event service not available")
		return
	}
	count, err := s.toolEvents.MarkAllRead(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changed": count})
}

func (s *Server) handleArchiveFeed(w http.ResponseWriter, r *http.Request) {
	if s.toolEvents == nil {
		writeError(w, http.StatusServiceUnavailable, "event service not available")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	if err := s.toolEvents.Archive(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleDeleteFeed(w http.ResponseWriter, r *http.Request) {
	if s.toolEvents == nil {
		writeError(w, http.StatusServiceUnavailable, "event service not available")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	if err := s.toolEvents.DeleteByID(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Populate feed items from event store.
	var feedItems []map[string]any
	var stats map[string]any

	if s.toolEvents != nil {
		q := r.URL.Query()
		limit := 20
		if v := q.Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}

		events, err := s.toolEvents.List(ctx, tool.EventFilter{
			ExcludeArchived: true,
			Limit:           limit,
		})
		if err == nil {
			feedItems = make([]map[string]any, 0, len(events))
			for _, ev := range events {
				feedItems = append(feedItems, map[string]any{
					"id":         ev.ID,
					"source":     ev.Source,
					"kind":       ev.Kind,
					"title":      ev.Title,
					"body":       ev.Body,
					"url":        ev.URL,
					"author":     ev.Actor,
					"groupKey":   ev.GroupKey,
					"isRead":     ev.ReadAt != nil,
					"isArchived": ev.ArchivedAt != nil,
					"createdAt":  ev.CreatedAt.Format(time.RFC3339),
				})
			}
		}

		total, _ := s.toolEvents.Count(ctx, tool.EventFilter{ExcludeArchived: true})
		unread, _ := s.toolEvents.Count(ctx, tool.EventFilter{UnreadOnly: true, ExcludeArchived: true})
		bySource, _ := s.toolEvents.CountBySource(ctx)

		archivedBySource, _ := s.toolEvents.CountArchivedBySource(ctx)

		stats = map[string]any{
			"total":            total,
			"unread":           unread,
			"bySource":         bySource,
			"archivedBySource": archivedBySource,
		}
	}

	if feedItems == nil {
		feedItems = []map[string]any{}
	}
	if stats == nil {
		stats = map[string]any{}
	}

	result := map[string]any{
		"feed":  feedItems,
		"stats": stats,
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
			stats["memories"] = len(mems)
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

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	if s.tasks == nil {
		writeError(w, http.StatusServiceUnavailable, "task engine not available")
		return
	}

	var req struct {
		Description string `json:"description"`
		Type        string `json:"type"`
		Priority    int    `json:"priority"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Description = strings.TrimSpace(req.Description)
	if req.Description == "" {
		writeError(w, http.StatusBadRequest, "description is required")
		return
	}
	if req.Type == "" {
		req.Type = "general"
	}
	if req.Priority < 0 || req.Priority > 4 {
		writeError(w, http.StatusBadRequest, "priority must be 0-4")
		return
	}

	input, _ := json.Marshal(map[string]string{"description": req.Description})

	t, err := s.tasks.Submit(r.Context(), &task.SubmitRequest{
		Type:        task.TaskType(req.Type),
		Priority:    task.Priority(req.Priority),
		Description: req.Description,
		Input:       input,
	})
	if err != nil {
		if errors.Is(err, task.ErrDuplicate) {
			writeError(w, http.StatusConflict, "duplicate task")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, marshalTask(t))
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

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	if s.tasks == nil {
		writeError(w, http.StatusServiceUnavailable, "task engine not available")
		return
	}

	taskID := r.PathValue("id")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "missing task id")
		return
	}

	if err := s.tasks.Delete(r.Context(), taskID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": taskID})
}

// --- Approvals ---

func (s *Server) handleListApprovals(w http.ResponseWriter, r *http.Request) {
	if s.approvals == nil {
		writeJSON(w, http.StatusOK, map[string]any{"approvals": []any{}})
		return
	}
	status := r.URL.Query().Get("status")
	approvals, err := s.approvals.List(r.Context(), status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if approvals == nil {
		approvals = []*task.Approval{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"approvals": approvals})
}

func (s *Server) handleApproveApproval(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing approval id")
		return
	}
	if s.approvals == nil {
		writeError(w, http.StatusServiceUnavailable, "approval store not configured")
		return
	}
	if err := s.approvals.Approve(r.Context(), id, "web"); err != nil {
		writeError(w, http.StatusNotFound, "approval not found or already decided")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "status": "approved"})
}

func (s *Server) handleDenyApproval(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing approval id")
		return
	}
	if s.approvals == nil {
		writeError(w, http.StatusServiceUnavailable, "approval store not configured")
		return
	}
	if err := s.approvals.Deny(r.Context(), id, "web"); err != nil {
		writeError(w, http.StatusNotFound, "approval not found or already decided")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "status": "denied"})
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

func (s *Server) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeError(w, http.StatusServiceUnavailable, "memory service not available")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing memory id")
		return
	}

	if err := s.memories.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func (s *Server) handleUpdateMemory(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeError(w, http.StatusServiceUnavailable, "memory service not available")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing memory id")
		return
	}

	var req struct {
		Content  string `json:"content"`
		Category string `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	m, err := s.memories.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "memory not found")
		return
	}

	m.Content = req.Content
	if req.Category != "" {
		m.Category = memory.Category(req.Category)
	}

	if err := s.memories.Update(r.Context(), m); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "memory": marshalMemory(m)})
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

	// Persist the user message in the session.
	if s.sessions != nil {
		userEvent := &agent.Event{
			SessionID: session.ID,
			Timestamp: time.Now(),
			Author:    "user",
			Parts:     []agent.Part{agent.TextPart{Text: req.Message}},
		}
		s.sessions.AppendEvent(ctx, session.ID, userEvent)
	}

	// Mark running immediately so the agent loop doesn't claim it.
	// Chat tasks are handled by the HTTP handler goroutine, not the loop.
	s.tasks.MarkRunning(ctx, t.ID)

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
		ToolMemories:   s.toolMemories,
		ToolEvents:     s.toolEvents,
		ToolDigest:     s.toolDigest,
		ToolJournal:    s.toolJournal,
		ToolTasks:      s.toolTasks,
		ToolStatus:     s.toolStatus,
		ToolSkills:     s.toolSkills,
		ToolNotifier:   s.toolNotifier,
		ToolCrons:      s.toolCrons,
		ToolConfig:     s.toolConfig,
		Config: &agent.AgentConfig{
			Model:     s.config.LLMModel,
			MaxRounds: s.config.MaxRoundsForMode(string(mode)),
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
				eventbus.Publish(s.bus, eventbus.ReasoningDelta{
					EventMeta: eventbus.NewMeta("agent"),
					TaskID:    t.ID,
					Text:      p.Text,
					Round:     ev.Event.Round,
				})
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
		if s.sessions != nil {
			s.sessions.AppendEvent(ctx, session.ID, ev.Event)
		}
	}

	// Complete the task.
	output, _ := json.Marshal(map[string]string{"text": fullText.String()})
	if err := s.tasks.Complete(ctx, t.ID, output); err != nil {
		slog.Error("failed to complete task", "task", t.ID, "error", err)
	}

	// Generate session title if empty (async, best-effort).
	if s.sessions != nil && s.llm != nil && session.Title == "" {
		go s.generateSessionTitle(session.ID, message, fullText.String())
	}
}

// generateSessionTitle uses a cheap LLM call to create a short title.
func (s *Server) generateSessionTitle(sessionID, userMsg, assistantMsg string) {
	s.logger.Info("generating session title", "session", sessionID, "userMsg", truncate(userMsg, 50))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := &llm.Request{
		Model:           "glm-4.5-air",
		System:          "You generate very short conversation titles (3-6 words). Reply with ONLY the title text, nothing else.",
		MaxTokens:       20,
		DisableThinking: true,
		Messages: []llm.Message{
			{Role: "user", Content: []llm.ContentBlock{llm.TextBlock{Text: fmt.Sprintf("User: %s\nAssistant: %s", truncate(userMsg, 150), truncate(assistantMsg, 150))}}},
		},
	}

	ch, err := s.llm.Stream(ctx, req)
	if err != nil {
		s.logger.Warn("title generation failed", "session", sessionID, "error", err)
		return
	}

	var title strings.Builder
	for ev := range ch {
		switch e := ev.(type) {
		case llm.TextDelta:
			title.WriteString(e.Text)
		case llm.StreamError:
			s.logger.Warn("title generation stream error", "session", sessionID, "error", e.Err)
		}
	}
	s.logger.Info("title generation completed", "session", sessionID, "title", title.String())

	t := strings.TrimSpace(title.String())
	if t == "" {
		return
	}
	// Cap at 60 chars
	if len(t) > 60 {
		t = t[:60]
	}

	if err := s.sessions.UpdateTitle(ctx, sessionID, t); err != nil {
		s.logger.Warn("failed to update session title", "session", sessionID, "error", err)
	}
}

// --- Skills ---

func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	if s.toolSkills == nil {
		writeJSON(w, http.StatusOK, map[string]any{"skills": []any{}})
		return
	}
	items := s.toolSkills.List()
	skills := make([]map[string]any, 0, len(items))
	for _, sk := range items {
		skills = append(skills, map[string]any{
			"name":                   sk.Name,
			"description":            sk.Description,
			"inclusion":              sk.Inclusion,
			"scope":                  sk.Inclusion,
			"allowedTools":           sk.AllowedTools,
			"disableModelInvocation": sk.DisableModel,
			"userInvocable":          sk.Inclusion == "on-demand",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": skills})
}

func (s *Server) handleGetSkill(w http.ResponseWriter, r *http.Request) {
	if s.toolSkills == nil {
		writeError(w, http.StatusServiceUnavailable, "skill service not available")
		return
	}
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing skill name")
		return
	}
	sk := s.toolSkills.Get(name)
	if sk == nil {
		writeError(w, http.StatusNotFound, "skill not found: "+name)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":                   sk.Name,
		"description":            sk.Description,
		"inclusion":              sk.Inclusion,
		"content":                sk.Content,
		"allowedTools":           sk.AllowedTools,
		"disableModelInvocation": sk.DisableModel,
	})
}

func (s *Server) handleCreateSkill(w http.ResponseWriter, r *http.Request) {
	if s.toolSkills == nil {
		writeError(w, http.StatusServiceUnavailable, "skill service not available")
		return
	}
	var req struct {
		Name         string   `json:"name"`
		Description  string   `json:"description"`
		Content      string   `json:"content"`
		Inclusion    string   `json:"inclusion"`
		AllowedTools []string `json:"allowedTools"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" || req.Description == "" || req.Content == "" {
		writeError(w, http.StatusBadRequest, "name, description, and content are required")
		return
	}
	if err := s.toolSkills.Create(req.Name, req.Description, req.Content, req.Inclusion, req.AllowedTools); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "name": req.Name})
}

func (s *Server) handleUpdateSkill(w http.ResponseWriter, r *http.Request) {
	if s.toolSkills == nil {
		writeError(w, http.StatusServiceUnavailable, "skill service not available")
		return
	}
	name := r.PathValue("name")
	var req struct {
		Description  string   `json:"description"`
		Content      string   `json:"content"`
		Inclusion    string   `json:"inclusion"`
		AllowedTools []string `json:"allowedTools"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := s.toolSkills.Update(name, req.Description, req.Content, req.Inclusion, req.AllowedTools); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name})
}

func (s *Server) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	if s.toolSkills == nil {
		writeError(w, http.StatusServiceUnavailable, "skill service not available")
		return
	}
	name := r.PathValue("name")
	if err := s.toolSkills.Delete(name); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- Skill Suggestions ---

func (s *Server) handleSkillSuggestions(w http.ResponseWriter, r *http.Request) {
	if s.skillSuggestor == nil {
		writeJSON(w, http.StatusOK, map[string]any{"suggestions": []any{}, "updatedAt": nil})
		return
	}
	suggestions, updatedAt := s.skillSuggestor.Suggestions()
	if suggestions == nil {
		suggestions = []agent.SkillSuggestion{}
	}
	var updatedAtStr *string
	if !updatedAt.IsZero() {
		t := updatedAt.Format(time.RFC3339)
		updatedAtStr = &t
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"suggestions": suggestions,
		"updatedAt":   updatedAtStr,
	})
}

func (s *Server) handleDismissSkillSuggestion(w http.ResponseWriter, r *http.Request) {
	if s.skillSuggestor == nil {
		writeError(w, http.StatusServiceUnavailable, "suggestions not available")
		return
	}
	var req struct {
		Slug string `json:"slug"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Slug == "" {
		writeError(w, http.StatusBadRequest, "slug is required")
		return
	}
	s.skillSuggestor.Dismiss(req.Slug)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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

func (s *Server) handleGetSoulPatch(w http.ResponseWriter, r *http.Request) {
	if s.soul == nil {
		writeJSON(w, http.StatusOK, map[string]any{"patch": nil})
		return
	}
	patch := s.soul.PendingPatch()
	if patch == nil {
		writeJSON(w, http.StatusOK, map[string]any{"patch": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"patch": patch})
}

func (s *Server) handleApproveSoulPatch(w http.ResponseWriter, r *http.Request) {
	if s.soul == nil {
		writeError(w, http.StatusServiceUnavailable, "soul not configured")
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.soul.ApprovePatch(req.ID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Reload soul in memory.
	if err := s.soul.Load(); err != nil {
		s.logger.Warn("failed to reload soul after patch approval", "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleDenySoulPatch(w http.ResponseWriter, r *http.Request) {
	if s.soul == nil {
		writeError(w, http.StatusServiceUnavailable, "soul not configured")
		return
	}

	var req struct {
		ID     string `json:"id"`
		Reason string `json:"reason"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.soul.DenyPatch(req.ID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Save denial reason + patch to memory.
	if s.memories != nil && req.Reason != "" {
		m := &memory.Memory{
			Content:    fmt.Sprintf("Soul patch denied. Reason: %s", req.Reason),
			Category:   memory.CatDecision,
			Scope:      memory.ScopeGlobal,
			Status:     memory.StatusProposed,
			Confidence: 0.9,
			Source:     "soul-review",
		}
		if err := s.memories.Create(r.Context(), m); err != nil {
			s.logger.Warn("failed to save soul denial to memory", "error", err)
		}
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
	if s.config != nil {
		status["mcp"] = map[string]any{
			"enabled":   s.config.MCPServerEnabled,
			"port":      s.config.MCPPort,
			"transport": s.config.MCPTransport,
		}
		channels := make([]map[string]any, 0)
		if s.config.TelegramBotToken != "" {
			channels = append(channels, map[string]any{"name": "telegram", "connected": true})
		}
		if s.config.DiscordBotToken != "" {
			channels = append(channels, map[string]any{"name": "discord", "connected": true})
		}
		if s.config.SlackBotToken != "" {
			channels = append(channels, map[string]any{"name": "slack", "connected": true})
		}
		status["channels"] = map[string]any{
			"items":          channels,
			"sessionTimeout": s.config.ChannelSessionTimeout,
		}
		status["embeddings"] = map[string]any{
			"enabled":    s.config.EmbeddingEnabled,
			"model":      s.config.EmbeddingModel,
			"dimensions": s.config.EmbeddingDimensions,
		}
		status["compaction"] = map[string]any{
			"triggerTokens": s.config.CompactionTriggerTokens,
			"keepRecent":    s.config.CompactionKeepRecent,
			"maxToolOutput": s.config.CompactionMaxToolOutput,
		}
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

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	if s.config == nil {
		writeJSON(w, http.StatusOK, config.PatchableConfig{})
		return
	}
	writeJSON(w, http.StatusOK, s.config.GetPatchable())
}

func (s *Server) handlePatchConfig(w http.ResponseWriter, r *http.Request) {
	if s.config == nil {
		writeError(w, http.StatusServiceUnavailable, "config not available")
		return
	}

	var patch config.PatchableConfig
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	s.config.ApplyPatch(patch)

	if err := s.config.SaveOverrides(s.config.DataDir); err != nil {
		s.logger.Warn("failed to save config overrides", "error", err)
	}

	if s.OnConfigPatch != nil {
		s.OnConfigPatch()
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "config": s.config.GetPatchable()})
}

// handleUpload accepts a multipart file upload (images/videos for vision tools).
// Files are saved to {DataDir}/uploads/ with a random filename.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	const maxUpload = 32 << 20 // 32MB
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)

	if err := r.ParseMultipartForm(maxUpload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "file too large (max 32MB)"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing file field"})
		return
	}
	defer file.Close()

	// Validate MIME type.
	ct := header.Header.Get("Content-Type")
	allowed := map[string]bool{
		"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true,
		"video/mp4": true, "video/quicktime": true, "video/x-m4v": true,
	}
	if !allowed[ct] {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("unsupported file type: %s", ct)})
		return
	}

	// Generate random filename.
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to generate filename"})
		return
	}
	exts, _ := mime.ExtensionsByType(ct)
	ext := ".bin"
	if len(exts) > 0 {
		ext = exts[0]
	}
	// Prefer common extensions.
	switch ct {
	case "image/png":
		ext = ".png"
	case "image/jpeg":
		ext = ".jpg"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	case "video/mp4":
		ext = ".mp4"
	case "video/quicktime":
		ext = ".mov"
	}
	filename := hex.EncodeToString(buf[:]) + ext

	// Save to uploads directory.
	dataDir := "./data"
	if s.config != nil && s.config.DataDir != "" {
		dataDir = s.config.DataDir
	}
	uploadDir := filepath.Join(dataDir, "uploads")
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to create upload directory"})
		return
	}

	destPath := filepath.Join(uploadDir, filename)
	dest, err := os.Create(destPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to save file"})
		return
	}
	defer dest.Close()

	written, err := io.Copy(dest, file)
	if err != nil {
		os.Remove(destPath)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to write file"})
		return
	}

	absPath, _ := filepath.Abs(destPath)
	writeJSON(w, http.StatusOK, map[string]any{
		"path":     absPath,
		"name":     header.Filename,
		"size":     written,
		"mimeType": ct,
	})
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
			"id":           s.ID,
			"title":        s.Title,
			"mode":         string(s.Mode),
			"messageCount": s.MessageCount,
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

// handleVoiceTranscribe accepts audio (multipart file) and returns transcribed text.
func (s *Server) handleVoiceTranscribe(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10MB hard limit
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid multipart form or file too large"})
		return
	}

	file, header, err := r.FormFile("audio")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing 'audio' file field"})
		return
	}
	defer file.Close()

	audioData, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "read audio failed"})
		return
	}

	text, err := s.voice.Transcribe(r.Context(), audioData, header.Filename)
	if err != nil {
		s.logger.Error("voice transcribe failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "transcription failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"text": text,
	})
}

// handleVoiceTTS converts text to speech and returns MP3 audio.
func (s *Server) handleVoiceTTS(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text  string `json:"text"`
		Voice string `json:"voice"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
		return
	}
	if body.Text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "text is required"})
		return
	}

	audio, err := s.voice.Synthesize(r.Context(), body.Text, body.Voice)
	if err != nil {
		s.logger.Error("voice TTS failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "speech synthesis failed"})
		return
	}

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(audio)))
	w.WriteHeader(http.StatusOK)
	w.Write(audio)
}

// --- Agent activity handlers ---

func (s *Server) handleAgentActivity(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 10000 {
			offset = n
		}
	}
	actType := q.Get("type")

	entries, err := s.activityStore.List(r.Context(), limit, offset, actType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if entries == nil {
		entries = []agent.ActivityEntry{}
	}

	stats, err := s.activityStore.GetToolStats(r.Context())
	if err != nil {
		stats = &agent.ToolStatsOverview{
			ByTool:       map[string]int{},
			ErrorsByTool: map[string]int{},
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": entries,
		"stats": stats,
	})
}

// --- Cron job handlers ---

func (s *Server) handleListCrons(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.cronStore.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if jobs == nil {
		jobs = []*cron.CronJob{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": jobs, "count": len(jobs)})
}

func (s *Server) handleCreateCron(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Schedule    string `json:"schedule"`
		Instruction string `json:"instruction"`
		Description string `json:"description"`
		Priority    *int   `json:"priority"`
		Timezone    string `json:"timezone"`
		CooldownMs  *int64 `json:"cooldownMs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" || req.Schedule == "" || req.Instruction == "" {
		writeError(w, http.StatusBadRequest, "name, schedule, and instruction are required")
		return
	}
	priority := 3
	if req.Priority != nil {
		priority = *req.Priority
	}
	tz := req.Timezone
	if tz == "" {
		tz = "UTC"
	}
	var cooldown int64 = 3600000
	if req.CooldownMs != nil {
		cooldown = *req.CooldownMs
	}

	job := &cron.CronJob{
		Enabled:     true,
		Name:        req.Name,
		Description: req.Description,
		Schedule:    req.Schedule,
		Instruction: req.Instruction,
		Timezone:    tz,
		Priority:    priority,
		CooldownMs:  cooldown,
	}
	if err := s.cronStore.Create(r.Context(), job); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, job)
}

func (s *Server) handleGetCron(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := s.cronStore.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "cron job not found")
		return
	}
	execs, _ := s.cronStore.ListExecutions(r.Context(), id, 10)
	writeJSON(w, http.StatusOK, map[string]any{"job": job, "executions": execs})
}

func (s *Server) handleUpdateCron(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Enabled     *bool   `json:"enabled"`
		Schedule    *string `json:"schedule"`
		Instruction *string `json:"instruction"`
		Description *string `json:"description"`
		Priority    *int    `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := s.cronStore.Update(r.Context(), id, req.Enabled, req.Schedule, req.Instruction, req.Description, req.Priority); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "cron job not found")
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	job, err := s.cronStore.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "cron job not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "job": job})
}

func (s *Server) handleDeleteCron(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.cronStore.Delete(r.Context(), id); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "cron job not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
