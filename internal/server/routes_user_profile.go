package server

import (
	"fmt"
	"net/http"
)

// --- User Profile & Agents Config ---

func (s *Server) handleGetUserProfile(w http.ResponseWriter, r *http.Request) {
	if s.userProfile == nil {
		writeJSON(w, http.StatusOK, map[string]any{"content": ""})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"content": s.userProfile.Content()})
}

func (s *Server) handlePutUserProfile(w http.ResponseWriter, r *http.Request) {
	if s.userProfile == nil {
		writeError(w, http.StatusServiceUnavailable, "user profile not configured")
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.userProfile.Save(req.Content); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("save user profile: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleGetAgentsConfig(w http.ResponseWriter, r *http.Request) {
	if s.agentsFile == nil {
		writeJSON(w, http.StatusOK, map[string]any{"content": ""})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"content": s.agentsFile.Content()})
}

func (s *Server) handlePutAgentsConfig(w http.ResponseWriter, r *http.Request) {
	if s.agentsFile == nil {
		writeError(w, http.StatusServiceUnavailable, "agents config not configured")
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.agentsFile.Save(req.Content); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("save agents config: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
