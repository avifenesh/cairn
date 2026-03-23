// Package server provides the HTTP server for Cairn, including REST API
// routes, SSE broadcasting, auth middleware, CORS, rate limiting, and
// static file serving. Uses Go 1.22+ net/http.ServeMux with pattern matching.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/avifenesh/cairn/internal/agent"
	"github.com/avifenesh/cairn/internal/agenttype"
	"github.com/avifenesh/cairn/internal/auth"
	"github.com/avifenesh/cairn/internal/config"
	"github.com/avifenesh/cairn/internal/cron"
	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/llm"
	cairnmcp "github.com/avifenesh/cairn/internal/mcp"
	"github.com/avifenesh/cairn/internal/memory"
	"github.com/avifenesh/cairn/internal/plugin"
	"github.com/avifenesh/cairn/internal/rules"
	"github.com/avifenesh/cairn/internal/skill"
	"github.com/avifenesh/cairn/internal/task"
	"github.com/avifenesh/cairn/internal/tool"
	"github.com/avifenesh/cairn/internal/voice"
)

// Server is the main HTTP server for Cairn, wiring together all API routes,
// SSE broadcasting, middleware, and static file serving.
type Server struct {
	mux            *http.ServeMux
	httpServer     *http.Server
	sse            *SSEBroadcaster
	agent          agent.Agent
	sessions       *agent.SessionStore
	tasks          *task.Engine
	memories       *memory.Service
	soul           *memory.Soul
	tools          *tool.Registry
	llm            llm.Provider
	bus            *eventbus.Bus
	config         *config.Config
	logger         *slog.Logger
	rateLimiter    *rateLimiter
	webhooks       http.Handler
	contextBuilder *memory.ContextBuilder
	plugins        *plugin.Manager
	journalStore   *agent.JournalStore

	// Tool service adapters (injected into ToolContext for agent tools).
	toolMemories tool.MemoryService
	toolEvents   tool.EventService
	toolDigest   tool.DigestService
	toolJournal  tool.JournalService
	toolTasks    tool.TaskService
	toolStatus   tool.StatusService
	toolSkills   tool.SkillService
	toolNotifier tool.NotifyService
	toolCrons    tool.CronService
	toolRules    tool.RulesService
	toolConfig   tool.ConfigService
	toolIdentity tool.IdentityService

	// Subagent runner (optional: enables cairn.spawnSubagent tool).
	subagentRunner tool.SubagentService

	// Voice service (optional).
	voice *voice.Service

	// Cron store (optional).
	cronStore *cron.Store

	// Rules store + engine (optional: automation rules).
	rulesStore  *rules.Store
	rulesEngine *rules.Engine

	// Activity store (optional).
	activityStore *agent.ActivityStore

	// Checkpoint store (optional: session crash recovery).
	checkpointStore *agent.CheckpointStore

	// Marketplace client (optional: ClawHub integration).
	marketplace *skill.MarketplaceClient

	// Skill suggestor (optional: auto-discovery).
	skillSuggestor *agent.SkillSuggestor

	// MCP client manager (optional: external MCP server connections).
	mcpClients *cairnmcp.ClientManager

	// Agent type service (optional: AGENT.md type definitions).
	agentTypes *agenttype.Service

	// Identity enrichment files (optional).
	userProfile   *memory.UserProfile
	agentsFile    *memory.AgentsFile
	curatedMemory *memory.MarkdownFile

	// steeringChannels tracks active session steering channels for the coding session panel.
	steeringChannels sync.Map // sessionID -> chan agent.SteeringMessage

	// Approval store (optional).
	approvals *task.ApprovalStore

	// WebAuthn auth (optional).
	authStore *auth.Store
	webauthn  *auth.WebAuthn

	// Poll trigger (optional: manual poll via POST /v1/poll/run).
	pollTrigger PollTrigger

	// OnConfigPatch is called after PATCH /v1/config is applied.
	// Allows external subsystems to react to config changes.
	OnConfigPatch func()
}

// PollTrigger allows the server to trigger a manual poll cycle.
type PollTrigger interface {
	PollNow(ctx context.Context)
}

// ServerConfig carries all dependencies needed to construct a Server.
type ServerConfig struct {
	Agent          agent.Agent
	Sessions       *agent.SessionStore
	Tasks          *task.Engine
	Memories       *memory.Service
	Soul           *memory.Soul
	Tools          *tool.Registry
	LLM            llm.Provider
	Bus            *eventbus.Bus
	Config         *config.Config
	Logger         *slog.Logger
	Webhooks       http.Handler           // optional: POST /v1/webhooks/{name}
	ContextBuilder *memory.ContextBuilder // optional: token-budgeted context
	Plugins        *plugin.Manager        // optional: lifecycle hooks
	JournalStore   *agent.JournalStore    // optional: for journal entries in context

	// Tool service adapters (set these so agent tools can access services).
	ToolMemories tool.MemoryService
	ToolEvents   tool.EventService
	ToolDigest   tool.DigestService
	ToolJournal  tool.JournalService
	ToolTasks    tool.TaskService
	ToolStatus   tool.StatusService
	ToolSkills   tool.SkillService
	ToolNotifier tool.NotifyService
	ToolCrons    tool.CronService
	ToolRules    tool.RulesService
	ToolConfig   tool.ConfigService
	ToolIdentity tool.IdentityService

	// Subagent runner (optional: enables cairn.spawnSubagent tool).
	SubagentRunner tool.SubagentService

	// Voice service (optional: STT/TTS).
	Voice *voice.Service

	// Cron store (optional: scheduled tasks).
	CronStore *cron.Store

	// Rules store + engine (optional: automation rules).
	RulesStore  *rules.Store
	RulesEngine *rules.Engine

	// Activity store (optional: agent observability).
	ActivityStore *agent.ActivityStore

	// Checkpoint store (optional: session crash recovery).
	CheckpointStore *agent.CheckpointStore

	// Marketplace client (optional: ClawHub integration).
	Marketplace *skill.MarketplaceClient

	// Skill suggestor (optional: auto-discovery).
	SkillSuggestor *agent.SkillSuggestor

	// MCP client manager (optional: external MCP server connections).
	MCPClients *cairnmcp.ClientManager

	// Approval store (optional: human-in-the-loop gates).
	Approvals *task.ApprovalStore

	// WebAuthn auth (optional: biometric login).
	AuthStore *auth.Store
	WebAuthn  *auth.WebAuthn

	// Poll trigger (optional: manual poll via POST /v1/poll/run).
	PollTrigger PollTrigger

	// Agent type service (optional: AGENT.md type definitions).
	AgentTypes *agenttype.Service

	// Identity enrichment files (optional).
	UserProfile   *memory.UserProfile
	AgentsFile    *memory.AgentsFile
	CuratedMemory *memory.MarkdownFile
}

// New creates a fully wired Server with all routes and middleware registered.
func New(cfg ServerConfig) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	mux := http.NewServeMux()

	s := &Server{
		mux:             mux,
		agent:           cfg.Agent,
		sessions:        cfg.Sessions,
		tasks:           cfg.Tasks,
		memories:        cfg.Memories,
		soul:            cfg.Soul,
		tools:           cfg.Tools,
		llm:             cfg.LLM,
		bus:             cfg.Bus,
		config:          cfg.Config,
		logger:          cfg.Logger,
		rateLimiter:     newRateLimiter(),
		webhooks:        cfg.Webhooks,
		contextBuilder:  cfg.ContextBuilder,
		plugins:         cfg.Plugins,
		journalStore:    cfg.JournalStore,
		toolMemories:    cfg.ToolMemories,
		toolEvents:      cfg.ToolEvents,
		toolDigest:      cfg.ToolDigest,
		toolJournal:     cfg.ToolJournal,
		toolTasks:       cfg.ToolTasks,
		toolStatus:      cfg.ToolStatus,
		toolSkills:      cfg.ToolSkills,
		toolNotifier:    cfg.ToolNotifier,
		toolCrons:       cfg.ToolCrons,
		toolRules:       cfg.ToolRules,
		toolConfig:      cfg.ToolConfig,
		toolIdentity:   cfg.ToolIdentity,
		subagentRunner:  cfg.SubagentRunner,
		voice:           cfg.Voice,
		cronStore:       cfg.CronStore,
		rulesStore:      cfg.RulesStore,
		rulesEngine:     cfg.RulesEngine,
		activityStore:   cfg.ActivityStore,
		checkpointStore: cfg.CheckpointStore,
		marketplace:     cfg.Marketplace,
		skillSuggestor:  cfg.SkillSuggestor,
		mcpClients:      cfg.MCPClients,
		approvals:       cfg.Approvals,
		authStore:       cfg.AuthStore,
		webauthn:        cfg.WebAuthn,
		pollTrigger:     cfg.PollTrigger,
		agentTypes:      cfg.AgentTypes,
		userProfile:     cfg.UserProfile,
		agentsFile:      cfg.AgentsFile,
		curatedMemory:   cfg.CuratedMemory,
	}

	// Create SSE broadcaster.
	s.sse = NewSSEBroadcaster(cfg.Bus, cfg.Logger)

	// Register all routes.
	s.registerRoutes()

	return s
}

// Start begins serving HTTP on the given address (e.g. ":8787").
// It blocks until the server shuts down.
func (s *Server) Start(addr string) error {
	// Start SSE broadcaster (subscribes to bus events, fans out to clients).
	s.sse.Start()

	// Start rate limiter cleanup.
	s.rateLimiter.startCleanup()

	handler := s.corsMiddleware(s.authMiddleware(s.mux))

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // SSE requires no write timeout
		IdleTimeout:  120 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
	}

	s.logger.Info("server starting", "addr", addr)
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server: listen: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the server, closing SSE clients and draining
// in-flight requests within the context deadline.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("server shutting down")

	s.sse.Close()
	s.rateLimiter.stop()

	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// Handler returns the root HTTP handler (for testing).
func (s *Server) Handler() http.Handler {
	return s.corsMiddleware(s.authMiddleware(s.mux))
}
