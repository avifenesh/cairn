package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/memory"
	"github.com/avifenesh/cairn/internal/task"
	"github.com/avifenesh/cairn/internal/tool"
)

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
