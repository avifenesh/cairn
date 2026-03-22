package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/agent"
)

// --- Skills ---

func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	if s.toolSkills == nil {
		writeJSON(w, http.StatusOK, map[string]any{"skills": []any{}})
		return
	}
	items := s.toolSkills.List()
	skills := make([]map[string]any, 0, len(items))
	for _, sk := range items {
		skills = append(skills, map[string]any{
			"name":                   sk.Name,
			"description":            sk.Description,
			"inclusion":              sk.Inclusion,
			"scope":                  sk.Inclusion,
			"allowedTools":           sk.AllowedTools,
			"disableModelInvocation": sk.DisableModel,
			"userInvocable":          sk.Inclusion == "on-demand",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": skills})
}

func (s *Server) handleGetSkill(w http.ResponseWriter, r *http.Request) {
	if s.toolSkills == nil {
		writeError(w, http.StatusServiceUnavailable, "skill service not available")
		return
	}
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing skill name")
		return
	}
	sk := s.toolSkills.Get(name)
	if sk == nil {
		writeError(w, http.StatusNotFound, "skill not found: "+name)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":                   sk.Name,
		"description":            sk.Description,
		"inclusion":              sk.Inclusion,
		"content":                sk.Content,
		"allowedTools":           sk.AllowedTools,
		"disableModelInvocation": sk.DisableModel,
	})
}

func (s *Server) handleCreateSkill(w http.ResponseWriter, r *http.Request) {
	if s.toolSkills == nil {
		writeError(w, http.StatusServiceUnavailable, "skill service not available")
		return
	}
	var req struct {
		Name         string   `json:"name"`
		Description  string   `json:"description"`
		Content      string   `json:"content"`
		Inclusion    string   `json:"inclusion"`
		AllowedTools []string `json:"allowedTools"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" || req.Description == "" || req.Content == "" {
		writeError(w, http.StatusBadRequest, "name, description, and content are required")
		return
	}
	if err := s.toolSkills.Create(req.Name, req.Description, req.Content, req.Inclusion, req.AllowedTools); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "name": req.Name})
}

func (s *Server) handleUpdateSkill(w http.ResponseWriter, r *http.Request) {
	if s.toolSkills == nil {
		writeError(w, http.StatusServiceUnavailable, "skill service not available")
		return
	}
	name := r.PathValue("name")
	var req struct {
		Description  string   `json:"description"`
		Content      string   `json:"content"`
		Inclusion    string   `json:"inclusion"`
		AllowedTools []string `json:"allowedTools"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := s.toolSkills.Update(name, req.Description, req.Content, req.Inclusion, req.AllowedTools); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name})
}

func (s *Server) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	if s.toolSkills == nil {
		writeError(w, http.StatusServiceUnavailable, "skill service not available")
		return
	}
	name := r.PathValue("name")
	if err := s.toolSkills.Delete(name); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- Skill Suggestions ---

func (s *Server) handleSkillSuggestions(w http.ResponseWriter, r *http.Request) {
	if s.skillSuggestor == nil {
		writeJSON(w, http.StatusOK, map[string]any{"suggestions": []any{}, "updatedAt": nil})
		return
	}
	suggestions, updatedAt := s.skillSuggestor.Suggestions()
	if suggestions == nil {
		suggestions = []agent.SkillSuggestion{}
	}
	var updatedAtStr *string
	if !updatedAt.IsZero() {
		t := updatedAt.Format(time.RFC3339)
		updatedAtStr = &t
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"suggestions": suggestions,
		"updatedAt":   updatedAtStr,
	})
}

func (s *Server) handleDismissSkillSuggestion(w http.ResponseWriter, r *http.Request) {
	if s.skillSuggestor == nil {
		writeError(w, http.StatusServiceUnavailable, "suggestions not available")
		return
	}
	var req struct {
		Slug string `json:"slug"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Slug == "" {
		writeError(w, http.StatusBadRequest, "slug is required")
		return
	}
	s.skillSuggestor.Dismiss(req.Slug)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
