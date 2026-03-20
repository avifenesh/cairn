package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// --- Marketplace (ClawHub) handlers ---

func (s *Server) handleMarketplaceSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeError(w, http.StatusBadRequest, "missing q parameter")
		return
	}
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 20 {
			limit = parsed
		}
	}

	results, err := s.marketplace.Search(r.Context(), query, limit)
	if err != nil {
		writeError(w, http.StatusBadGateway, "marketplace search failed: "+err.Error())
		return
	}

	// Check which result slugs are already installed locally.
	installed := make(map[string]bool)
	if s.toolSkills != nil {
		for _, r := range results {
			if s.toolSkills.Get(r.Slug) != nil {
				installed[r.Slug] = true
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"results":   results,
		"installed": installed,
	})
}

func (s *Server) handleMarketplaceBrowse(w http.ResponseWriter, r *http.Request) {
	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "trending"
	}
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	skills, err := s.marketplace.Browse(r.Context(), sort, limit)
	if err != nil {
		writeError(w, http.StatusBadGateway, "marketplace browse failed: "+err.Error())
		return
	}

	installed := make(map[string]bool)
	if s.toolSkills != nil {
		for _, sk := range skills {
			if s.toolSkills.Get(sk.Slug) != nil {
				installed[sk.Slug] = true
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"skills":    skills,
		"installed": installed,
	})
}

func (s *Server) handleMarketplaceDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "missing slug")
		return
	}

	detail, err := s.marketplace.Detail(r.Context(), slug)
	if err != nil {
		writeError(w, http.StatusBadGateway, "marketplace detail failed: "+err.Error())
		return
	}

	installed := false
	if s.toolSkills != nil {
		if existing := s.toolSkills.Get(slug); existing != nil {
			installed = true
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"skill":     detail,
		"installed": installed,
	})
}

func (s *Server) handleMarketplacePreview(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "missing slug")
		return
	}

	content, err := s.marketplace.Preview(r.Context(), slug)
	if err != nil {
		writeError(w, http.StatusBadGateway, "marketplace preview failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"content": content,
	})
}

func (s *Server) handleMarketplaceInstall(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "missing slug")
		return
	}

	// Sanitize slug against path traversal.
	if strings.Contains(slug, "/") || strings.Contains(slug, "\\") || strings.Contains(slug, "..") || slug == "." {
		writeError(w, http.StatusBadRequest, "invalid slug")
		return
	}

	if s.toolSkills == nil {
		writeError(w, http.StatusServiceUnavailable, "skill service not available")
		return
	}

	// Check for name collision.
	if existing := s.toolSkills.Get(slug); existing != nil {
		writeError(w, http.StatusConflict, "skill "+slug+" already exists locally")
		return
	}

	targetDir := s.toolSkills.InstallDir()
	if targetDir == "" {
		writeError(w, http.StatusInternalServerError, "no skill install directory configured")
		return
	}

	prov, err := s.marketplace.Install(r.Context(), slug, targetDir)
	if err != nil {
		writeError(w, http.StatusBadGateway, "install failed: "+err.Error())
		return
	}

	// Re-discover skills.
	if refreshErr := s.toolSkills.Refresh(); refreshErr != nil {
		s.logger.Warn("skill re-discovery failed after marketplace install", "error", refreshErr)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"name":    slug,
		"version": prov.Version,
	})
}
