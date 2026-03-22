package server

import (
	"net/http"
	"strconv"

	"github.com/avifenesh/cairn/internal/agent"
)

// --- Agent activity handlers ---

func (s *Server) handleAgentActivity(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 50
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 10000 {
			offset = n
		}
	}
	actType := q.Get("type")

	entries, err := s.activityStore.List(r.Context(), limit, offset, actType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if entries == nil {
		entries = []agent.ActivityEntry{}
	}

	stats, err := s.activityStore.GetToolStats(r.Context())
	if err != nil {
		stats = &agent.ToolStatsOverview{
			ByTool:       map[string]int{},
			ErrorsByTool: map[string]int{},
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": entries,
		"stats": stats,
	})
}
