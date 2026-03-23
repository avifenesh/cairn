package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/avifenesh/cairn/internal/rules"
)

// --- Signal source handlers ---

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request) {
	if s.sourceRegistry == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.sourceRegistry.RegisteredSources()})
}

// --- Automation rule handlers ---

func (s *Server) handleListRules(w http.ResponseWriter, r *http.Request) {
	items, err := s.rulesStore.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []*rules.Rule{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "count": len(items)})
}

func (s *Server) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var req struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Trigger     rules.Trigger  `json:"trigger"`
		Condition   string         `json:"condition"`
		Actions     []rules.Action `json:"actions"`
		ThrottleMs  int64          `json:"throttleMs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.Name) > 256 {
		writeError(w, http.StatusBadRequest, "name too long (max 256 chars)")
		return
	}
	if len(req.Description) > 1024 {
		writeError(w, http.StatusBadRequest, "description too long (max 1024 chars)")
		return
	}
	if req.Trigger.Type == "" {
		writeError(w, http.StatusBadRequest, "trigger.type is required")
		return
	}
	if len(req.Actions) == 0 {
		writeError(w, http.StatusBadRequest, "at least one action is required")
		return
	}
	if len(req.Condition) > 2048 {
		writeError(w, http.StatusBadRequest, "condition too long (max 2048 chars)")
		return
	}
	if len(req.Actions) > 10 {
		writeError(w, http.StatusBadRequest, "too many actions (max 10)")
		return
	}

	rule := &rules.Rule{
		Name:        req.Name,
		Description: req.Description,
		Enabled:     true,
		Trigger:     req.Trigger,
		Condition:   req.Condition,
		Actions:     req.Actions,
		ThrottleMs:  req.ThrottleMs,
	}
	if err := s.rulesStore.Create(r.Context(), rule); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Refresh engine cache so the new rule is immediately active.
	if s.rulesEngine != nil {
		s.rulesEngine.RefreshCache()
	}

	writeJSON(w, http.StatusCreated, map[string]any{"rule": rule})
}

func (s *Server) handleGetRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rule, err := s.rulesStore.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}
	execs, _ := s.rulesStore.ListExecutions(r.Context(), id, 10)
	if execs == nil {
		execs = []*rules.Execution{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"rule": rule, "executions": execs})
}

func (s *Server) handleUpdateRule(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	id := r.PathValue("id")
	var req struct {
		Enabled     *bool          `json:"enabled"`
		Name        *string        `json:"name"`
		Description *string        `json:"description"`
		Trigger     *rules.Trigger `json:"trigger"`
		Condition   *string        `json:"condition"`
		Actions     []rules.Action `json:"actions"`
		ThrottleMs  *int64         `json:"throttleMs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Actions != nil && len(req.Actions) == 0 {
		writeError(w, http.StatusBadRequest, "at least one action is required")
		return
	}

	opts := rules.UpdateOpts{
		Enabled:     req.Enabled,
		Name:        req.Name,
		Description: req.Description,
		Trigger:     req.Trigger,
		Condition:   req.Condition,
		Actions:     req.Actions,
		ThrottleMs:  req.ThrottleMs,
	}
	if err := s.rulesStore.Update(r.Context(), id, opts); err != nil {
		if isRuleNotFound(err) {
			writeError(w, http.StatusNotFound, "rule not found")
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	// Refresh engine cache.
	if s.rulesEngine != nil {
		s.rulesEngine.RefreshCache()
	}

	rule, err := s.rulesStore.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "rule": rule})
}

func (s *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.rulesStore.Delete(r.Context(), id); err != nil {
		if isRuleNotFound(err) {
			writeError(w, http.StatusNotFound, "rule not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	// Refresh engine cache.
	if s.rulesEngine != nil {
		s.rulesEngine.RefreshCache()
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleListRuleExecutions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	execs, err := s.rulesStore.ListExecutions(r.Context(), id, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if execs == nil {
		execs = []*rules.Execution{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": execs})
}

func (s *Server) handleRecentRuleExecutions(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	execs, err := s.rulesStore.ListRecentExecutions(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if execs == nil {
		execs = []*rules.Execution{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":     execs,
		"updatedAt": time.Now().UTC().Format(time.RFC3339),
	})
}

// isRuleNotFound checks if the error is a rule-not-found error.
func isRuleNotFound(err error) bool {
	return errors.Is(err, rules.ErrNotFound)
}

// --- Rule template handlers ---

func (s *Server) handleListRuleTemplates(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	var templates []rules.Template
	if source != "" {
		templates = rules.ListTemplatesForSource(source)
	} else {
		templates = rules.ListTemplates()
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": templates})
}

func (s *Server) handleInstantiateRuleTemplate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	id := r.PathValue("id")

	var req struct {
		Params map[string]string `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	rule, err := rules.Instantiate(id, req.Params)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.rulesStore.Create(r.Context(), rule); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if s.rulesEngine != nil {
		s.rulesEngine.RefreshCache()
	}

	writeJSON(w, http.StatusCreated, map[string]any{"rule": rule})
}
