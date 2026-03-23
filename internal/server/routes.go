package server

import "time"

// registerRoutes sets up all HTTP route handlers on the server's mux.
func (s *Server) registerRoutes() {
	// Health / readiness — always open.
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /ready", s.handleReady)

	// SSE stream.
	s.mux.HandleFunc("GET /v1/stream", s.sse.ServeHTTP)

	// Feed.
	s.mux.HandleFunc("GET /v1/feed", s.handleListFeed)
	s.mux.HandleFunc("POST /v1/feed/{id}/read", s.handleMarkFeedRead)
	s.mux.HandleFunc("POST /v1/feed/read-all", s.handleMarkAllFeedRead)
	s.mux.HandleFunc("POST /v1/feed/{id}/archive", s.handleArchiveFeed)
	s.mux.HandleFunc("DELETE /v1/feed/{id}", s.handleDeleteFeed)
	s.mux.HandleFunc("GET /v1/dashboard", s.handleDashboard)

	// Tasks.
	s.mux.HandleFunc("GET /v1/tasks", s.handleListTasks)
	s.mux.HandleFunc("POST /v1/tasks", s.handleCreateTask)
	s.mux.HandleFunc("POST /v1/tasks/{id}/cancel", s.handleCancelTask)
	s.mux.HandleFunc("DELETE /v1/tasks/{id}", s.handleDeleteTask)

	// Subagent routes.
	s.mux.HandleFunc("GET /v1/subagents", s.handleListSubagents)
	s.mux.HandleFunc("GET /v1/subagents/{id}", s.handleGetSubagent)
	s.mux.HandleFunc("POST /v1/subagents/{id}/cancel", s.handleCancelSubagent)

	// Approvals.
	s.mux.HandleFunc("GET /v1/approvals", s.handleListApprovals)
	s.mux.HandleFunc("POST /v1/approvals/{id}/approve", s.handleApproveApproval)
	s.mux.HandleFunc("POST /v1/approvals/{id}/deny", s.handleDenyApproval)

	// Memories.
	s.mux.HandleFunc("GET /v1/memories", s.handleListMemories)
	s.mux.HandleFunc("GET /v1/memories/search", s.handleSearchMemories)
	s.mux.HandleFunc("POST /v1/memories", s.handleCreateMemory)
	s.mux.HandleFunc("POST /v1/memories/{id}/accept", s.handleAcceptMemory)
	s.mux.HandleFunc("POST /v1/memories/{id}/reject", s.handleRejectMemory)
	s.mux.HandleFunc("DELETE /v1/memories/{id}", s.handleDeleteMemory)
	s.mux.HandleFunc("PUT /v1/memories/{id}", s.handleUpdateMemory)

	// Assistant / sessions.
	s.mux.HandleFunc("GET /v1/assistant/sessions", s.handleListSessions)
	s.mux.HandleFunc("GET /v1/assistant/sessions/{id}", s.handleGetSession)
	s.mux.HandleFunc("POST /v1/assistant/message", s.rateLimitMiddleware(10, time.Minute, s.handleAssistantMessage))

	// Session observability (coding session panel).
	s.mux.HandleFunc("GET /v1/sessions/{id}/stream", s.handleSessionStream)
	s.mux.HandleFunc("GET /v1/sessions/{id}/events", s.handleSessionEvents)
	s.mux.HandleFunc("POST /v1/sessions/{id}/steer", s.handleSessionSteer)
	s.mux.HandleFunc("POST /v1/upload", s.handleUpload)
	s.mux.HandleFunc("GET /v1/config", s.handleGetConfig)
	s.mux.HandleFunc("PATCH /v1/config", s.handlePatchConfig)

	// Skills.
	s.mux.HandleFunc("GET /v1/skills", s.handleListSkills)
	s.mux.HandleFunc("GET /v1/skills/{name}", s.handleGetSkill)
	s.mux.HandleFunc("POST /v1/skills", s.handleCreateSkill)
	s.mux.HandleFunc("PUT /v1/skills/{name}", s.handleUpdateSkill)
	s.mux.HandleFunc("DELETE /v1/skills/{name}", s.handleDeleteSkill)

	// Agent types.
	s.mux.HandleFunc("GET /v1/agent-types", s.handleListAgentTypes)
	s.mux.HandleFunc("GET /v1/agent-types/{name}", s.handleGetAgentType)
	s.mux.HandleFunc("POST /v1/agent-types", s.requireWrite(s.handleCreateAgentType))
	s.mux.HandleFunc("PUT /v1/agent-types/{name}", s.requireWrite(s.handleUpdateAgentType))
	s.mux.HandleFunc("POST /v1/agent-types/{name}/run", s.requireWrite(s.rateLimitMiddleware(10, time.Minute, s.handleRunAgentType)))
	s.mux.HandleFunc("DELETE /v1/agent-types/{name}", s.requireWrite(s.handleDeleteAgentType))

	// User profile and agents config.
	s.mux.HandleFunc("GET /v1/user-profile", s.handleGetUserProfile)
	s.mux.HandleFunc("PUT /v1/user-profile", s.handlePutUserProfile)
	s.mux.HandleFunc("GET /v1/agents-config", s.handleGetAgentsConfig)
	s.mux.HandleFunc("PUT /v1/agents-config", s.handlePutAgentsConfig)

	// Marketplace (ClawHub).
	if s.marketplace != nil {
		s.mux.HandleFunc("GET /v1/marketplace/search", s.handleMarketplaceSearch)
		s.mux.HandleFunc("GET /v1/marketplace/browse", s.handleMarketplaceBrowse)
		s.mux.HandleFunc("GET /v1/marketplace/skills/{slug}", s.handleMarketplaceDetail)
		s.mux.HandleFunc("GET /v1/marketplace/skills/{slug}/preview", s.handleMarketplacePreview)
		s.mux.HandleFunc("POST /v1/marketplace/skills/{slug}/install", s.handleMarketplaceInstall)
		s.mux.HandleFunc("POST /v1/marketplace/skills/{slug}/review", s.handleMarketplaceReview)
	}

	// Skill suggestions.
	s.mux.HandleFunc("GET /v1/skills/suggestions", s.handleSkillSuggestions)
	s.mux.HandleFunc("POST /v1/skills/suggestions/dismiss", s.handleDismissSkillSuggestion)

	// MCP client connections.
	s.mux.HandleFunc("GET /v1/mcp/connections", s.handleListMCPConnections)
	if s.mcpClients != nil {
		s.mux.HandleFunc("POST /v1/mcp/connections", s.handleAddMCPConnection)
		s.mux.HandleFunc("DELETE /v1/mcp/connections/{name}", s.handleRemoveMCPConnection)
		s.mux.HandleFunc("POST /v1/mcp/connections/{name}/reconnect", s.handleReconnectMCPConnection)
	}

	// Soul.
	s.mux.HandleFunc("GET /v1/soul", s.handleGetSoul)
	s.mux.HandleFunc("PUT /v1/soul", s.handlePutSoul)
	s.mux.HandleFunc("GET /v1/soul/patch", s.handleGetSoulPatch)
	s.mux.HandleFunc("POST /v1/soul/patch/approve", s.handleApproveSoulPatch)
	s.mux.HandleFunc("POST /v1/soul/patch/deny", s.handleDenySoulPatch)

	// Cron jobs (optional).
	if s.cronStore != nil {
		s.mux.HandleFunc("GET /v1/crons", s.handleListCrons)
		s.mux.HandleFunc("POST /v1/crons", s.handleCreateCron)
		s.mux.HandleFunc("GET /v1/crons/{id}", s.handleGetCron)
		s.mux.HandleFunc("PATCH /v1/crons/{id}", s.handleUpdateCron)
		s.mux.HandleFunc("DELETE /v1/crons/{id}", s.handleDeleteCron)
	}

	// Signal sources (unconditional — useful for rules UI and general status).
	s.mux.HandleFunc("GET /v1/sources", s.handleListSources)

	// Automation rules (optional).
	if s.rulesStore != nil {
		s.mux.HandleFunc("GET /v1/rules/executions/recent", s.handleRecentRuleExecutions)
		s.mux.HandleFunc("GET /v1/rules", s.handleListRules)
		s.mux.HandleFunc("POST /v1/rules", s.requireWrite(s.handleCreateRule))
		s.mux.HandleFunc("GET /v1/rules/{id}", s.handleGetRule)
		s.mux.HandleFunc("PATCH /v1/rules/{id}", s.requireWrite(s.handleUpdateRule))
		s.mux.HandleFunc("DELETE /v1/rules/{id}", s.requireWrite(s.handleDeleteRule))
		s.mux.HandleFunc("GET /v1/rules/{id}/executions", s.handleListRuleExecutions)
		s.mux.HandleFunc("GET /v1/rule-templates", s.handleListRuleTemplates)
		s.mux.HandleFunc("POST /v1/rule-templates/{id}/instantiate", s.requireWrite(s.handleInstantiateRuleTemplate))
	}

	// Agent activity.
	if s.activityStore != nil {
		s.mux.HandleFunc("GET /v1/agent/activity", s.handleAgentActivity)
	}

	// Webhooks (optional, wired when WEBHOOK_SECRETS is configured).
	if s.webhooks != nil {
		s.mux.Handle("POST /v1/webhooks/{name}", s.webhooks)
	}

	// Voice (optional — requires whisper + edge-tts).
	if s.voice != nil {
		s.mux.HandleFunc("POST /v1/assistant/voice", s.handleVoiceTranscribe)
		s.mux.HandleFunc("POST /v1/assistant/voice/tts", s.handleVoiceTTS)
	}

	// Auth (WebAuthn — optional, requires authStore + webauthn).
	if s.webauthn != nil {
		s.mux.HandleFunc("POST /v1/auth/register/start", s.requireWrite(s.handleAuthRegisterStart))
		s.mux.HandleFunc("POST /v1/auth/register/complete", s.requireWrite(s.handleAuthRegisterComplete))
		s.mux.HandleFunc("POST /v1/auth/login/start", s.handleAuthLoginStart)
		s.mux.HandleFunc("POST /v1/auth/login/complete", s.handleAuthLoginComplete)
		s.mux.HandleFunc("POST /v1/auth/logout", s.handleAuthLogout)
		s.mux.HandleFunc("GET /v1/auth/session", s.handleAuthSession)
		s.mux.HandleFunc("GET /v1/auth/credentials", s.requireWrite(s.handleListAuthCredentials))
		s.mux.HandleFunc("DELETE /v1/auth/credentials/{id}", s.requireWrite(s.handleDeleteAuthCredential))
	}

	// System.
	s.mux.HandleFunc("GET /v1/status", s.handleStatus)
	s.mux.HandleFunc("GET /v1/costs", s.handleCosts)
	s.mux.HandleFunc("GET /v1/journal", s.handleJournal)
	s.mux.HandleFunc("GET /v1/plugins", s.handlePlugins)
	s.mux.HandleFunc("POST /v1/poll/run", s.handlePollRun)

	// Static files (SPA fallback).
	s.mux.Handle("/", s.staticHandler())
}
