package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/avifenesh/cairn/internal/config"
	cairnmcp "github.com/avifenesh/cairn/internal/mcp"
)

// --- MCP Connections ---

func (s *Server) handleListMCPConnections(w http.ResponseWriter, r *http.Request) {
	if s.mcpClients == nil {
		writeJSON(w, http.StatusOK, map[string]any{"connections": []any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": s.mcpClients.Status()})
}

func (s *Server) handleAddMCPConnection(w http.ResponseWriter, r *http.Request) {
	var cfg cairnmcp.MCPServerConfig
	if err := readJSON(r, &cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if cfg.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := s.mcpClients.Connect(r.Context(), cfg); err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, cairnmcp.ErrInvalidTransport),
			errors.Is(err, cairnmcp.ErrMissingCommand),
			errors.Is(err, cairnmcp.ErrMissingURL):
			status = http.StatusBadRequest
		case errors.Is(err, cairnmcp.ErrDuplicateName):
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	// Persist to config so connections survive restart.
	s.persistMCPConnections()
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "name": cfg.Name})
}

func (s *Server) handleRemoveMCPConnection(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.mcpClients.Disconnect(name); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	s.persistMCPConnections()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleReconnectMCPConnection(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.mcpClients.Reconnect(name); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, cairnmcp.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// persistMCPConnections saves the current MCP client connections to config.json.
func (s *Server) persistMCPConnections() {
	if s.mcpClients == nil || s.config == nil {
		return
	}
	configs := s.mcpClients.Configs()
	raw, err := json.Marshal(configs)
	if err != nil {
		s.logger.Warn("failed to marshal MCP connections", "error", err)
		return
	}
	configStr := string(raw)
	s.config.ApplyPatch(config.PatchableConfig{MCPClientServers: &configStr})
	if err := s.config.SaveOverrides(s.config.DataDir); err != nil {
		s.logger.Warn("failed to persist MCP connections", "error", err)
	}
}
