package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/avifenesh/cairn/internal/task"
)

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
