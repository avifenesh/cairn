package server

import (
	"fmt"
	"net/http"
	"strings"
)

// --- Agent Types ---

func (s *Server) handleListAgentTypes(w http.ResponseWriter, r *http.Request) {
	if s.agentTypes == nil {
		writeJSON(w, http.StatusOK, map[string]any{"types": []any{}})
		return
	}
	items := s.agentTypes.List()
	types := make([]map[string]any, 0, len(items))
	for _, at := range items {
		types = append(types, map[string]any{
			"name":         at.Name,
			"description":  at.Description,
			"mode":         string(at.Mode),
			"allowedTools": at.AllowedTools,
			"deniedTools":  at.DeniedTools,
			"maxRounds":    at.MaxRounds,
			"worktree":     at.Worktree,
			"model":        at.Model,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"types": types})
}

func (s *Server) handleGetAgentType(w http.ResponseWriter, r *http.Request) {
	if s.agentTypes == nil {
		writeError(w, http.StatusServiceUnavailable, "agent types not configured")
		return
	}
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing type name")
		return
	}
	at := s.agentTypes.Get(name)
	if at == nil {
		writeError(w, http.StatusNotFound, "agent type not found: "+name)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":         at.Name,
		"description":  at.Description,
		"mode":         string(at.Mode),
		"allowedTools": at.AllowedTools,
		"deniedTools":  at.DeniedTools,
		"maxRounds":    at.MaxRounds,
		"worktree":     at.Worktree,
		"model":        at.Model,
		"content":      at.Content,
	})
}

func (s *Server) handleCreateAgentType(w http.ResponseWriter, r *http.Request) {
	if s.agentTypes == nil {
		writeError(w, http.StatusServiceUnavailable, "agent types not configured")
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Content     string `json:"content"`
		Mode        string `json:"mode"`
		MaxRounds   int    `json:"maxRounds"`
		Worktree    bool   `json:"worktree"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" || req.Description == "" {
		writeError(w, http.StatusBadRequest, "name and description are required")
		return
	}
	// Build AGENT.md content from structured fields.
	var md strings.Builder
	md.WriteString("---\n")
	fmt.Fprintf(&md, "name: %s\n", req.Name)
	fmt.Fprintf(&md, "description: %q\n", req.Description)
	if req.Mode != "" {
		fmt.Fprintf(&md, "mode: %s\n", req.Mode)
	}
	if req.MaxRounds > 0 {
		fmt.Fprintf(&md, "max-rounds: %d\n", req.MaxRounds)
	}
	if req.Worktree {
		md.WriteString("worktree: true\n")
	}
	md.WriteString("---\n\n")
	md.WriteString(req.Content)
	md.WriteString("\n")
	if err := s.agentTypes.Create(req.Name, md.String()); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "name": req.Name})
}

func (s *Server) handleDeleteAgentType(w http.ResponseWriter, r *http.Request) {
	if s.agentTypes == nil {
		writeError(w, http.StatusServiceUnavailable, "agent types not configured")
		return
	}
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing type name")
		return
	}
	if err := s.agentTypes.Delete(name); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
