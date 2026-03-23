package server

import (
	"fmt"
	"net/http"

	"github.com/avifenesh/cairn/internal/memory"
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

// --- Curated Memory File (MEMORY.md) ---

func (s *Server) handleGetMemoryFile(w http.ResponseWriter, r *http.Request) {
	if s.curatedMemory == nil {
		writeJSON(w, http.StatusOK, map[string]any{"content": ""})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"content": s.curatedMemory.Content()})
}

func (s *Server) handlePutMemoryFile(w http.ResponseWriter, r *http.Request) {
	if s.curatedMemory == nil {
		writeError(w, http.StatusServiceUnavailable, "memory file not configured")
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.curatedMemory.Save(req.Content); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("save memory file: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- Patch endpoints for AGENTS.md and MEMORY.md ---

func (s *Server) handleGetAgentsPatch(w http.ResponseWriter, r *http.Request) {
	writePatchResponse(w, s.agentsFile)
}

func (s *Server) handleApproveAgentsPatch(w http.ResponseWriter, r *http.Request) {
	approvePatch(w, r, s.agentsFile)
}

func (s *Server) handleDenyAgentsPatch(w http.ResponseWriter, r *http.Request) {
	denyPatch(w, r, s.agentsFile)
}

func (s *Server) handleGetMemoryPatch(w http.ResponseWriter, r *http.Request) {
	writePatchResponse(w, s.curatedMemory)
}

func (s *Server) handleApproveMemoryPatch(w http.ResponseWriter, r *http.Request) {
	approvePatch(w, r, s.curatedMemory)
}

func (s *Server) handleDenyMemoryPatch(w http.ResponseWriter, r *http.Request) {
	denyPatch(w, r, s.curatedMemory)
}

// --- Shared patch helpers ---

// patchable is the subset of MarkdownFile used by the patch endpoints.
type patchable interface {
	PendingPatch() *memory.PendingPatch
	ApprovePatch(id string) error
	DenyPatch(id string) error
}

func writePatchResponse(w http.ResponseWriter, mf patchable) {
	if mf == nil {
		writeJSON(w, http.StatusOK, map[string]any{"patch": nil})
		return
	}
	patch := mf.PendingPatch()
	if patch == nil {
		writeJSON(w, http.StatusOK, map[string]any{"patch": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"patch": patch})
}

func approvePatch(w http.ResponseWriter, r *http.Request, mf patchable) {
	if mf == nil {
		writeError(w, http.StatusServiceUnavailable, "file not configured")
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := mf.ApprovePatch(req.ID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func denyPatch(w http.ResponseWriter, r *http.Request, mf patchable) {
	if mf == nil {
		writeError(w, http.StatusServiceUnavailable, "file not configured")
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := mf.DenyPatch(req.ID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
