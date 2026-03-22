package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/avifenesh/cairn/internal/memory"
)

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
