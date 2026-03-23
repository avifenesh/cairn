package server

import (
	"fmt"
	"net/http"
	"os"

	"github.com/avifenesh/cairn/internal/memory"
)

// --- Soul ---

func (s *Server) handleGetSoul(w http.ResponseWriter, r *http.Request) {
	if s.soul == nil {
		writeError(w, http.StatusNotFound, "soul not configured")
		return
	}

	w.Header().Set("Cache-Control", "no-store")
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

	if err := s.soul.DenyPatchWithReason(req.ID, req.Reason); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Save denial as accepted memory so reflection engine sees it immediately.
	if s.memories != nil && req.Reason != "" {
		m := &memory.Memory{
			Content:    fmt.Sprintf("Soul patch denied. Reason: %s", req.Reason),
			Category:   memory.CatDecision,
			Scope:      memory.ScopeGlobal,
			Status:     memory.StatusAccepted,
			Confidence: 0.9,
			Source:     "soul-review",
		}
		if err := s.memories.Create(r.Context(), m); err != nil {
			s.logger.Warn("failed to save soul denial to memory", "error", err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
