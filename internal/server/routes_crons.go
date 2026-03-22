package server

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/avifenesh/cairn/internal/cron"
)

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
	var cooldown int64 = 300000 // 5 minutes default
	if req.CooldownMs != nil {
		cooldown = *req.CooldownMs // 0 = no cooldown
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
		CooldownMs  *int64  `json:"cooldownMs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := s.cronStore.Update(r.Context(), id, req.Enabled, req.Schedule, req.Instruction, req.Description, req.Priority, req.CooldownMs); err != nil {
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
