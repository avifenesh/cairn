package server

import (
	"net/http"

	"github.com/avifenesh/cairn/internal/task"
)

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
