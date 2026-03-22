package server

import (
	"net/http"
	"time"
)

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
