package server

import (
	"context"
	"net/http"
	"time"

	"github.com/avifenesh/cairn/internal/plugin"
)

// --- System ---

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]any{
		"ok":      true,
		"uptime":  time.Since(startTime).String(),
		"version": "0.1.0",
	}
	if s.agent != nil {
		status["agent"] = s.agent.Name()
	}
	if s.config != nil {
		status["mcp"] = map[string]any{
			"enabled":   s.config.MCPServerEnabled,
			"port":      s.config.MCPPort,
			"transport": s.config.MCPTransport,
		}
		if s.mcpClients != nil {
			status["mcpConnections"] = s.mcpClients.Status()
		}
		channels := make([]map[string]any, 0)
		if s.config.TelegramBotToken != "" {
			channels = append(channels, map[string]any{"name": "telegram", "connected": true})
		}
		if s.config.DiscordBotToken != "" {
			channels = append(channels, map[string]any{"name": "discord", "connected": true})
		}
		if s.config.SlackBotToken != "" {
			channels = append(channels, map[string]any{"name": "slack", "connected": true})
		}
		status["channels"] = map[string]any{
			"items":          channels,
			"sessionTimeout": s.config.ChannelSessionTimeout,
		}
		status["embeddings"] = map[string]any{
			"enabled":    s.config.EmbeddingEnabled,
			"model":      s.config.EmbeddingModel,
			"dimensions": s.config.EmbeddingDimensions,
		}
		status["compaction"] = map[string]any{
			"triggerTokens": s.config.CompactionTriggerTokens,
			"keepRecent":    s.config.CompactionKeepRecent,
			"maxToolOutput": s.config.CompactionMaxToolOutput,
		}
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleCosts(w http.ResponseWriter, r *http.Request) {
	// Match frontend CostData interface: todayUsd, weekUsd, budgetDailyUsd, budgetWeeklyUsd.
	result := map[string]any{
		"todayUsd":        0.0,
		"weekUsd":         0.0,
		"budgetDailyUsd":  0.0,
		"budgetWeeklyUsd": 0.0,
		"totalCalls":      int64(0),
		"blocked":         int64(0),
	}
	if s.plugins != nil {
		for _, p := range s.plugins.Plugins() {
			if bp, ok := p.(*plugin.BudgetPlugin); ok {
				stats := bp.Stats()
				result["todayUsd"] = stats.DailySpend
				result["weekUsd"] = stats.WeeklySpend
				result["budgetDailyUsd"] = stats.DailyCap
				result["budgetWeeklyUsd"] = stats.WeeklyCap
				result["totalCalls"] = stats.TotalCalls
				result["blocked"] = stats.Blocked
				break
			}
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handlePollRun(w http.ResponseWriter, r *http.Request) {
	if s.pollTrigger == nil {
		writeError(w, http.StatusServiceUnavailable, "signal plane not configured")
		return
	}
	// Use background context - the poll must outlive the HTTP request.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	go func() {
		defer cancel()
		s.pollTrigger.PollNow(ctx)
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "message": "poll triggered"})
}

func (s *Server) handleJournal(w http.ResponseWriter, r *http.Request) {
	if s.journalStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}})
		return
	}
	entries, err := s.journalStore.Recent(r.Context(), 7*24*time.Hour)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		items = append(items, map[string]any{
			"sessionId": e.SessionID,
			"summary":   e.Summary,
			"mode":      e.Mode,
			"learnings": e.Learnings,
			"errors":    e.Errors,
			"createdAt": e.CreatedAt.Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handlePlugins(w http.ResponseWriter, r *http.Request) {
	items := make([]map[string]any, 0)
	if s.plugins != nil {
		for _, p := range s.plugins.Plugins() {
			items = append(items, map[string]any{
				"name": p.Name(),
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
