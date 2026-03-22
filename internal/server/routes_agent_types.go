package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/tool"
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
			"skills":       at.Skills,
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
		"skills":       at.Skills,
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
	// Validate mode if provided.
	if req.Mode != "" && req.Mode != "talk" && req.Mode != "work" && req.Mode != "coding" {
		writeError(w, http.StatusBadRequest, "mode must be one of: talk, work, coding")
		return
	}
	// Build AGENT.md content from structured fields.
	// Strip newlines from frontmatter values to prevent injection.
	var md strings.Builder
	md.WriteString("---\n")
	fmt.Fprintf(&md, "name: %s\n", req.Name)
	fmt.Fprintf(&md, "description: \"%s\"\n", stripNewlines(req.Description))
	if req.Mode != "" {
		fmt.Fprintf(&md, "mode: %s\n", stripNewlines(req.Mode))
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

func (s *Server) handleUpdateAgentType(w http.ResponseWriter, r *http.Request) {
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

	var req struct {
		Description *string  `json:"description"`
		Content     *string  `json:"content"`
		Mode        *string  `json:"mode"`
		MaxRounds   *int     `json:"maxRounds"`
		Worktree    *bool    `json:"worktree"`
		DeniedTools []string `json:"deniedTools"`
		Skills      []string `json:"skills"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Validate mode if provided.
	if req.Mode != nil && *req.Mode != "talk" && *req.Mode != "work" && *req.Mode != "coding" {
		writeError(w, http.StatusBadRequest, "mode must be one of: talk, work, coding")
		return
	}

	// Merge: keep existing values for nil/zero fields.
	desc := at.Description
	if req.Description != nil {
		desc = *req.Description
	}
	mode := string(at.Mode)
	if req.Mode != nil {
		mode = *req.Mode
	}
	maxRounds := at.MaxRounds
	if req.MaxRounds != nil && *req.MaxRounds > 0 {
		maxRounds = *req.MaxRounds
	}
	worktree := at.Worktree
	if req.Worktree != nil {
		worktree = *req.Worktree
	}
	deniedTools := at.DeniedTools
	if req.DeniedTools != nil {
		deniedTools = req.DeniedTools
	}
	skills := at.Skills
	if req.Skills != nil {
		skills = req.Skills
	}
	content := at.Content
	if req.Content != nil {
		content = *req.Content
	}

	// Build AGENT.md frontmatter + body string.
	// Strip newlines from all frontmatter values to prevent injection.
	var md strings.Builder
	md.WriteString("---\n")
	fmt.Fprintf(&md, "name: %s\n", name)
	fmt.Fprintf(&md, "description: \"%s\"\n", stripNewlines(desc))
	fmt.Fprintf(&md, "mode: %s\n", stripNewlines(mode))
	fmt.Fprintf(&md, "max-rounds: %d\n", maxRounds)
	if worktree {
		md.WriteString("worktree: true\n")
	}
	if len(deniedTools) > 0 {
		sanitized := make([]string, len(deniedTools))
		for i, dt := range deniedTools {
			sanitized[i] = stripNewlines(dt)
		}
		fmt.Fprintf(&md, "denied-tools: %s\n", strings.Join(sanitized, ", "))
	}
	if len(skills) > 0 {
		sanitized := make([]string, len(skills))
		for i, sk := range skills {
			sanitized[i] = stripNewlines(sk)
		}
		fmt.Fprintf(&md, "skills: %s\n", strings.Join(sanitized, ","))
	}
	md.WriteString("---\n")
	md.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		md.WriteString("\n")
	}

	if err := s.agentTypes.Update(name, md.String()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleRunAgentType(w http.ResponseWriter, r *http.Request) {
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
	if s.subagentRunner == nil {
		writeError(w, http.StatusServiceUnavailable, "subagent runner not available")
		return
	}

	var req struct {
		Instruction string `json:"instruction"`
		Context     string `json:"context"`
		ExecMode    string `json:"execMode"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Instruction == "" {
		writeError(w, http.StatusBadRequest, "instruction is required")
		return
	}
	execMode := req.ExecMode
	if execMode == "" {
		execMode = "background"
	}
	// Only background is supported for the /run endpoint; foreground would
	// block the HTTP handler goroutine indefinitely.
	if execMode != "background" {
		writeError(w, http.StatusBadRequest, "execMode must be \"background\"")
		return
	}

	spawnReq := &tool.SubagentSpawnRequest{
		Type:        name,
		Instruction: req.Instruction,
		Context:     req.Context,
		ExecMode:    execMode,
	}

	parentID := fmt.Sprintf("ui-run-%d", time.Now().UnixMilli())
	result, err := s.subagentRunner.Spawn(r.Context(), parentID, spawnReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"taskId":    result.TaskID,
		"sessionId": result.SessionID,
		"status":    result.Status,
	})
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
