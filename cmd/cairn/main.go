package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/avifenesh/cairn/internal/agent"
	cairnchannel "github.com/avifenesh/cairn/internal/channel"
	"github.com/avifenesh/cairn/internal/config"
	"github.com/avifenesh/cairn/internal/db"
	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/llm"
	cairnmcp "github.com/avifenesh/cairn/internal/mcp"
	"github.com/avifenesh/cairn/internal/memory"
	"github.com/avifenesh/cairn/internal/plugin"
	"github.com/avifenesh/cairn/internal/server"
	signalplane "github.com/avifenesh/cairn/internal/signal"
	"github.com/avifenesh/cairn/internal/skill"
	"github.com/avifenesh/cairn/internal/task"
	"github.com/avifenesh/cairn/internal/tool"
	"github.com/avifenesh/cairn/internal/tool/builtin"
)

// Set by goreleaser ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "chat":
			runChat(logger)
			return
		case "serve":
			runServe(logger)
			return
		case "version":
			fmt.Printf("cairn %s (%s) built %s\n", version, commit, date)
			return
		case "install":
			if len(os.Args) > 2 && os.Args[2] == "skill" {
				if len(os.Args) < 4 {
					fmt.Fprintln(os.Stderr, "Usage: cairn install skill <source>")
					fmt.Fprintln(os.Stderr, "  source: git URL (https://... or *.git) or local directory path")
					os.Exit(1)
				}
				runInstallSkill(logger, os.Args[3])
				return
			}
			fmt.Fprintln(os.Stderr, "Usage: cairn install skill <source>")
			os.Exit(1)
			return
		}
	}

	fmt.Println("cairn: personal agent OS")
	fmt.Printf("version %s (%s)\n\n", version, commit)
	fmt.Println("Usage: cairn chat \"your message here\"")
	fmt.Println("       cairn chat --mode coding \"your message\"")
	fmt.Println("       cairn serve                      # start HTTP server")
	fmt.Println("       cairn install skill <source>     # install a skill from git URL or local path")
	fmt.Println("       cairn version                    # show version info")
}

// runServe starts the HTTP server with all subsystems initialized.
func runServe(logger *slog.Logger) {
	// Load config.
	cfg := config.LoadOptional()

	// Initialize event bus.
	bus := eventbus.New(eventbus.WithLogger(logger))
	defer bus.Close()

	// Initialize database.
	database, err := db.Open(cfg.DatabasePath)
	if err != nil {
		logger.Error("database required for serve mode", "error", err)
		os.Exit(1)
	}
	defer database.Close()
	if err := database.Migrate(); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}

	// Initialize LLM provider (optional for serve — some endpoints work without it).
	var provider llm.Provider
	if cfg.LLMAPIKey != "" {
		registry := llm.NewRegistry(logger)
		if err := registry.RegisterFromConfig(llm.ProviderConfig{
			Type:    cfg.LLMProvider,
			APIKey:  cfg.LLMAPIKey,
			BaseURL: cfg.LLMBaseURL,
			Model:   cfg.LLMModel,
		}); err != nil {
			logger.Error("failed to register LLM provider", "error", err)
			os.Exit(1)
		}

		if cfg.LLMFallbackModel != "" {
			registry.SetFallback(cfg.LLMModel, cfg.LLMFallbackModel)
		}

		var resolveErr error
		provider, _, resolveErr = registry.WithRetryAndFallback(cfg.LLMModel, llm.DefaultRetryConfig())
		if resolveErr != nil {
			logger.Warn("LLM provider not available, agent endpoints disabled", "error", resolveErr)
		} else {
			logger.Info("llm provider ready", "provider", cfg.LLMProvider, "model", cfg.LLMModel)
		}
	}

	// Configure web tool backends BEFORE registering tools (All() checks ZaiEnabled).
	if cfg.ZaiWebEnabled && cfg.LLMAPIKey != "" {
		builtin.SetZaiConfig(cfg.LLMAPIKey, cfg.ZaiBaseURL)
		logger.Info("zai web tools enabled", "baseURL", cfg.ZaiBaseURL)
	}
	builtin.SetWebConfig(cfg.SearXNGURL, time.Duration(cfg.WebFetchTimeout)*time.Second, cfg.WebFetchMaxSize)

	// Initialize tool registry.
	toolRegistry := tool.NewRegistry()
	toolRegistry.Register(builtin.All()...)

	// Initialize memory service.
	memStore := memory.NewStore(database)
	memService := memory.NewService(memStore, memory.NoopEmbedder{}, bus)
	soul := memory.NewSoul(cfg.SoulPath)
	soul.Load() // ignore error if SOUL.md doesn't exist yet

	// Initialize context builder (token-budgeted memory injection).
	ctxBuilder := memory.NewContextBuilder(memStore, memory.NoopEmbedder{}, memory.ContextConfig{
		TokenBudget:     cfg.MemoryContextBudget,
		HardRuleReserve: cfg.MemoryHardRuleReserve,
		DecayHalfLife:   cfg.MemoryDecayHalfLife,
		StaleThreshold:  cfg.MemoryStaleThreshold,
	})
	logger.Info("context builder ready", "budget", cfg.MemoryContextBudget, "hardRuleReserve", cfg.MemoryHardRuleReserve)

	// Initialize plugin manager.
	pluginMgr := plugin.NewManager(logger)
	pluginMgr.Register(plugin.NewLoggingPlugin(logger))
	if cfg.BudgetDailyCap > 0 || cfg.BudgetWeeklyCap > 0 {
		pluginMgr.Register(plugin.NewBudgetPlugin(plugin.BudgetConfig{
			DailyCap:  cfg.BudgetDailyCap,
			WeeklyCap: cfg.BudgetWeeklyCap,
		}, logger))
		logger.Info("budget plugin active", "dailyCap", cfg.BudgetDailyCap, "weeklyCap", cfg.BudgetWeeklyCap)
	}
	// Initialize session store + journal store.
	sessionStore := agent.NewSessionStore(database)
	journalStore := agent.NewJournalStore(database.DB)

	// Initialize task engine.
	taskStore := task.NewStore(database)
	taskEngine := task.NewEngine(taskStore, bus, nil)
	defer taskEngine.Close()

	// Create the ReAct agent.
	var reactAgent agent.Agent
	if provider != nil {
		reactAgent = agent.NewReActAgent("cairn", logger)
	}

	// Initialize signal plane (source polling).
	eventStore := signalplane.NewEventStore(database.DB)
	sourceState := signalplane.NewSourceState(database.DB)
	scheduler := signalplane.NewScheduler(eventStore, sourceState, bus, logger)

	pollInterval := time.Duration(cfg.PollInterval) * time.Second

	if cfg.GHToken != "" {
		scheduler.Register(signalplane.NewGitHubPoller(signalplane.GitHubConfig{
			Token: cfg.GHToken,
			Orgs:  cfg.GHOrgs,
		}), pollInterval)
		logger.Info("signal: github poller registered", "orgs", cfg.GHOrgs)
	}

	if len(cfg.HNKeywords) > 0 || cfg.HNMinScore > 0 {
		scheduler.Register(signalplane.NewHNPoller(signalplane.HNConfig{
			Keywords: cfg.HNKeywords,
			MinScore: cfg.HNMinScore,
		}), pollInterval)
		logger.Info("signal: hn poller registered", "keywords", cfg.HNKeywords, "minScore", cfg.HNMinScore)
	}

	if len(cfg.RedditSubs) > 0 {
		scheduler.Register(signalplane.NewRedditPoller(signalplane.RedditConfig{
			Subreddits: cfg.RedditSubs,
		}), pollInterval)
		logger.Info("signal: reddit poller registered", "subreddits", cfg.RedditSubs)
	}

	if len(cfg.NPMPackages) > 0 {
		scheduler.Register(signalplane.NewNPMPoller(signalplane.NPMConfig{
			Packages: cfg.NPMPackages,
		}), 15*time.Minute) // npm/crates poll less frequently
		logger.Info("signal: npm poller registered", "packages", cfg.NPMPackages)
	}

	if len(cfg.CratesPackages) > 0 {
		scheduler.Register(signalplane.NewCratesPoller(signalplane.CratesConfig{
			Crates: cfg.CratesPackages,
		}), 15*time.Minute)
		logger.Info("signal: crates poller registered", "crates", cfg.CratesPackages)
	}

	scheduler.Start()
	defer scheduler.Close()

	// Initialize digest runner (for agent digest tool).
	var digestRunner *signalplane.DigestRunner
	if provider != nil {
		digestRunner = signalplane.NewDigestRunner(eventStore, provider, cfg.LLMModel)
	}

	// Build tool service adapters for agent tools.
	memAdapter := &memoryAdapter{svc: memService}
	eventAdapter := &eventAdapter{store: eventStore}
	var digestAdapt tool.DigestService
	if digestRunner != nil {
		digestAdapt = &digestAdapter{runner: digestRunner}
	}
	journalAdapt := &journalAdapter{store: journalStore}

	// Initialize skill service.
	skillSvc := skill.NewService(cfg.SkillDirs, logger)
	if err := skillSvc.Discover(); err != nil {
		logger.Warn("skill discovery failed", "error", err)
	} else {
		logger.Info("skills discovered", "count", len(skillSvc.List()))
	}
	skillAdapt := &skillAdapter{svc: skillSvc}

	taskAdapt := &taskAdapter{engine: taskEngine}
	// Collect active poller names for status display.
	var pollerNames []string
	if cfg.GHToken != "" {
		pollerNames = append(pollerNames, "github")
	}
	if len(cfg.HNKeywords) > 0 || cfg.HNMinScore > 0 {
		pollerNames = append(pollerNames, "hn")
	}
	if len(cfg.RedditSubs) > 0 {
		pollerNames = append(pollerNames, "reddit")
	}
	if len(cfg.NPMPackages) > 0 {
		pollerNames = append(pollerNames, "npm")
	}
	if len(cfg.CratesPackages) > 0 {
		pollerNames = append(pollerNames, "crates")
	}
	statusAdapt := &statusAdapter{
		tasks:       taskEngine,
		events:      eventStore,
		memories:    memService,
		startedAt:   time.Now(),
		pollerNames: pollerNames,
	}

	// Initialize webhook handler.
	var webhookHandler *signalplane.WebhookHandler
	if len(cfg.WebhookSecrets) > 0 {
		webhookHandler = signalplane.NewWebhookHandler(eventStore, cfg.WebhookSecrets)
		logger.Info("signal: webhook handler ready", "webhooks", len(cfg.WebhookSecrets))
	}

	// Start always-on agent loop (if idle mode enabled and agent available).
	if cfg.IdleModeEnabled && reactAgent != nil && provider != nil {
		journaler := agent.NewJournaler(journalStore, provider, cfg.LLMModel)

		reflector := agent.NewReflectionEngine(journalStore, memService, soul, provider, cfg.LLMModel, agent.ReflectionConfig{
			Interval: time.Duration(cfg.ReflectionInterval) * time.Second,
		})

		agentLoop := agent.NewLoop(agent.LoopConfig{
			TickInterval:       time.Duration(cfg.AgentTickInterval) * time.Second,
			ReflectionInterval: time.Duration(cfg.ReflectionInterval) * time.Second,
			Model:              cfg.LLMModel,
		}, agent.LoopDeps{
			Agent:        reactAgent,
			Tasks:        taskEngine,
			Events:       eventStore,
			Memories:     memService,
			Soul:         soul,
			Tools:        toolRegistry,
			Provider:     provider,
			Bus:          bus,
			Journaler:    journaler,
			Reflector:    reflector,
			Logger:       logger,
			ToolMemories: memAdapter,
			ToolEvents:   eventAdapter,
			ToolDigest:   digestAdapt,
			ToolJournal:  journalAdapt,
			ToolTasks:    taskAdapt,
			ToolStatus:   statusAdapt,
			ToolSkills:   skillAdapt,
		})
		agentLoop.Start()
		defer agentLoop.Close()
		logger.Info("agent loop started", "tick", cfg.AgentTickInterval, "reflection", cfg.ReflectionInterval)
	}

	// Create and start the server.
	srv := server.New(server.ServerConfig{
		Agent:          reactAgent,
		Sessions:       sessionStore,
		Tasks:          taskEngine,
		Memories:       memService,
		Soul:           soul,
		Tools:          toolRegistry,
		LLM:            provider,
		Bus:            bus,
		Config:         cfg,
		Logger:         logger,
		Webhooks:       webhookHandler,
		ContextBuilder: ctxBuilder,
		Plugins:        pluginMgr,
		JournalStore:   journalStore,
		ToolMemories:   memAdapter,
		ToolEvents:     eventAdapter,
		ToolDigest:     digestAdapt,
		ToolJournal:    journalAdapt,
		ToolTasks:      taskAdapt,
		ToolStatus:     statusAdapt,
		ToolSkills:     skillAdapt,
	})

	// Graceful shutdown context — all subsystems observe this.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start channel router if any channels are configured.
	if cfg.TelegramBotToken != "" {
		channelSessionStore := cairnchannel.NewSessionStore(database.DB)
		sessionTimeout := time.Duration(cfg.ChannelSessionTimeout) * time.Minute

		// Channel message handler: look up/create session, run agent.
		channelHandler := func(ctx context.Context, msg *cairnchannel.IncomingMessage) (*cairnchannel.OutgoingMessage, error) {
			// Handle /new command — reset session.
			if msg.IsCommand && msg.Command == "new" {
				if err := channelSessionStore.Reset(ctx, msg.ChannelID, msg.ChatID); err != nil {
					return nil, fmt.Errorf("channel session reset: %w", err)
				}
				return &cairnchannel.OutgoingMessage{Text: "New session started."}, nil
			}

			// Look up or create session.
			sessionID, isNew, err := channelSessionStore.GetOrCreate(ctx, msg.ChannelID, msg.ChatID, sessionTimeout)
			if err != nil {
				return nil, fmt.Errorf("channel session: %w", err)
			}

			// Load or create agent session.
			session, getErr := sessionStore.Get(ctx, sessionID)
			if getErr != nil && !isNew {
				return nil, fmt.Errorf("channel: load session: %w", getErr)
			}
			if session == nil || isNew {
				session = &agent.Session{
					ID:    sessionID,
					Mode:  tool.ModeTalk,
					State: map[string]any{"channel": msg.ChannelID, "chatID": msg.ChatID},
				}
				if err := sessionStore.Create(ctx, session); err != nil {
					return nil, fmt.Errorf("channel: create session: %w", err)
				}
			}

			// Determine message text.
			text := msg.Text
			if msg.IsCommand {
				text = "/" + msg.Command
				if msg.Args != "" {
					text += " " + msg.Args
				}
			}

			// Build invocation context.
			invCtx := &agent.InvocationContext{
				Context:        ctx,
				SessionID:      sessionID,
				UserMessage:    text,
				Mode:           tool.ModeTalk,
				Session:        session,
				Tools:          toolRegistry,
				LLM:            provider,
				Memory:         memService,
				Soul:           soul,
				Bus:            bus,
				ContextBuilder: ctxBuilder,
				Plugins:        pluginMgr,
				ToolMemories:   memAdapter,
				ToolEvents:     eventAdapter,
				ToolDigest:     digestAdapt,
				ToolJournal:    journalAdapt,
				ToolTasks:      taskAdapt,
				ToolStatus:     statusAdapt,
				ToolSkills:     skillAdapt,
				Config:         &agent.AgentConfig{Model: cfg.LLMModel},
			}

			// Run agent, collect response.
			var response strings.Builder
			for ev := range reactAgent.Run(invCtx) {
				if ev.Err != nil {
					return nil, ev.Err
				}
				if ev.Event == nil {
					continue
				}
				for _, part := range ev.Event.Parts {
					if tp, ok := part.(agent.TextPart); ok && ev.Event.Author != "user" {
						response.WriteString(tp.Text)
					}
				}
			}

			// Persist conversation.
			userEv := &agent.Event{Author: "user", Parts: []agent.Part{agent.TextPart{Text: text}}}
			if err := sessionStore.AppendEvent(ctx, sessionID, userEv); err != nil {
				logger.Warn("channel: failed to persist user event", "error", err)
			}
			if response.Len() > 0 {
				assistantEv := &agent.Event{Author: cfg.LLMModel, Parts: []agent.Part{agent.TextPart{Text: response.String()}}}
				if err := sessionStore.AppendEvent(ctx, sessionID, assistantEv); err != nil {
					logger.Warn("channel: failed to persist assistant event", "error", err)
				}
			}

			return &cairnchannel.OutgoingMessage{Text: response.String()}, nil
		}

		channelRouter := cairnchannel.NewRouter(channelHandler, logger)

		tg, err := cairnchannel.NewTelegram(cairnchannel.TelegramConfig{
			BotToken: cfg.TelegramBotToken,
			ChatID:   cfg.TelegramChatID,
		}, channelHandler, logger)
		if err != nil {
			logger.Error("telegram adapter failed", "error", err)
		} else {
			channelRouter.Register(tg)
			logger.Info("telegram channel registered", "chatID", cfg.TelegramChatID)
		}

		// Start router in background — stopped by ctx cancel on shutdown.
		go func() {
			if err := channelRouter.Start(ctx); err != nil && err != context.Canceled {
				logger.Error("channel router error", "error", err)
			}
		}()
	}

	// Start MCP server if enabled.
	if cfg.MCPServerEnabled {
		// Determine working directory for filesystem tools.
		workDir, _ := os.Getwd()

		mcpToolCtx := &tool.ToolContext{
			Cancel:   ctx,
			WorkDir:  workDir,
			Memories: memAdapter,
			Events:   eventAdapter,
			Digest:   digestAdapt,
			Journal:  journalAdapt,
			Tasks:    taskAdapt,
			Status:   statusAdapt,
			Skills:   skillAdapt,
		}
		mcpSrv := cairnmcp.New(cairnmcp.Config{
			Port:           cfg.MCPPort,
			Transport:      cfg.MCPTransport,
			WriteRateLimit: cfg.MCPWriteRateLimit,
		}, toolRegistry, mcpToolCtx, logger)

		transport := cfg.MCPTransport
		switch transport {
		case "http", "stdio", "both":
			// valid
		default:
			logger.Error("invalid MCP_TRANSPORT, must be http/stdio/both", "value", transport)
			os.Exit(1)
		}

		if transport == "http" || transport == "both" {
			go func() {
				if err := mcpSrv.ServeHTTP(); err != nil {
					logger.Error("mcp http server error", "error", err)
				}
			}()
		}
		if transport == "stdio" || transport == "both" {
			go func() {
				if err := mcpSrv.ServeStdio(ctx); err != nil && ctx.Err() == nil {
					logger.Error("mcp stdio server error", "error", err)
				}
			}()
		}
		logger.Info("mcp server started", "transport", transport, "port", cfg.MCPPort)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("received signal, shutting down", "signal", sig)
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown error", "error", err)
		}
	}()

	_ = ctx // shutdown handled via signal
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	if err := srv.Start(addr); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

// runChat implements the Phase 1 deliverable: `cairn chat "hello"` streams GLM response.
func runChat(logger *slog.Logger) {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: cairn chat \"your message\"")
		os.Exit(1)
	}
	// Build message from args, filtering out flags.
	var msgParts []string
	for i := 2; i < len(os.Args); i++ {
		if os.Args[i] == "--mode" && i+1 < len(os.Args) {
			i++ // skip flag value
			continue
		}
		msgParts = append(msgParts, os.Args[i])
	}
	message := strings.Join(msgParts, " ")

	// Load config
	cfg := config.LoadOptional()
	if cfg.LLMAPIKey == "" {
		fmt.Fprintln(os.Stderr, "Error: LLM_API_KEY (or GLM_API_KEY / OPENAI_API_KEY) is required")
		os.Exit(1)
	}

	// Initialize event bus
	bus := eventbus.New(eventbus.WithLogger(logger))
	defer bus.Close()

	// Initialize database
	database, err := db.Open(cfg.DatabasePath)
	if err != nil {
		logger.Warn("database not available, running without persistence", "error", err)
	} else {
		defer database.Close()
		if err := database.Migrate(); err != nil {
			logger.Warn("migration failed", "error", err)
		}
	}

	// Initialize LLM provider registry
	registry := llm.NewRegistry(logger)
	if err := registry.RegisterFromConfig(llm.ProviderConfig{
		Type:    cfg.LLMProvider,
		APIKey:  cfg.LLMAPIKey,
		BaseURL: cfg.LLMBaseURL,
		Model:   cfg.LLMModel,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to register LLM provider: %v\n", err)
		os.Exit(1)
	}

	// Configure fallback if set.
	if cfg.LLMFallbackModel != "" {
		registry.SetFallback(cfg.LLMModel, cfg.LLMFallbackModel)
	}

	// Resolve provider with retry + fallback.
	provider, _, err := registry.WithRetryAndFallback(cfg.LLMModel, llm.DefaultRetryConfig())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to resolve LLM provider: %v\n", err)
		os.Exit(1)
	}

	logger.Info("llm provider ready", "provider", cfg.LLMProvider, "model", cfg.LLMModel)

	// Initialize tool registry with built-in tools.
	toolRegistry := tool.NewRegistry()
	toolRegistry.Register(builtin.All()...)

	// Initialize memory service (keyword-only for now, no embeddings).
	var memService *memory.Service
	var soul *memory.Soul
	if database != nil {
		memStore := memory.NewStore(database)
		memService = memory.NewService(memStore, memory.NoopEmbedder{}, bus)
		soul = memory.NewSoul(cfg.SoulPath)
		soul.Load() // ignore error if SOUL.md doesn't exist yet
	}

	// Initialize session store.
	var sessionStore *agent.SessionStore
	if database != nil {
		sessionStore = agent.NewSessionStore(database)
	}

	// Create the ReAct agent.
	reactAgent := agent.NewReActAgent("cairn", logger)

	// Create context with cancellation on SIGINT.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Determine mode from --mode flag or default to talk.
	mode := tool.ModeTalk
	for i, arg := range os.Args {
		if arg == "--mode" && i+1 < len(os.Args) {
			mode = tool.Mode(os.Args[i+1])
		}
	}

	// Create or load session.
	session := &agent.Session{
		Mode:  mode,
		State: map[string]any{"workDir": "."},
	}
	if sessionStore != nil {
		if err := sessionStore.Create(ctx, session); err != nil {
			logger.Warn("failed to create session", "error", err)
		}
	}
	if session.ID == "" {
		session.ID = "ephemeral" // no DB, generate a placeholder
	}

	// Build tool service adapters for CLI chat.
	var chatMemAdapter tool.MemoryService
	var chatSkillAdapter tool.SkillService
	if memService != nil {
		chatMemAdapter = &memoryAdapter{svc: memService}
	}
	chatSkillSvc := skill.NewService(cfg.SkillDirs, logger)
	if err := chatSkillSvc.Discover(); err == nil && len(chatSkillSvc.List()) > 0 {
		chatSkillAdapter = &skillAdapter{svc: chatSkillSvc}
	}

	// Build invocation context.
	invCtx := &agent.InvocationContext{
		Context:      ctx,
		SessionID:    session.ID,
		UserMessage:  message,
		Mode:         mode,
		Session:      session,
		Tools:        toolRegistry,
		LLM:          provider,
		Memory:       memService,
		Soul:         soul,
		Bus:          bus,
		ToolMemories: chatMemAdapter,
		ToolSkills:   chatSkillAdapter,
		Config: &agent.AgentConfig{
			Model: cfg.LLMModel,
		},
	}

	// Run the agent, accumulate full text for persistence.
	var fullText strings.Builder
	for ev := range reactAgent.Run(invCtx) {
		if ev.Err != nil {
			fmt.Fprintf(os.Stderr, "\nError: %v\n", ev.Err)
			os.Exit(1)
		}
		if ev.Event == nil {
			continue
		}

		for _, part := range ev.Event.Parts {
			switch p := part.(type) {
			case agent.TextPart:
				if ev.Event.Author != "user" {
					fmt.Print(p.Text)
					fullText.WriteString(p.Text)
				}
			case agent.ReasoningPart:
				fmt.Fprintf(os.Stderr, "\033[2m%s\033[0m", p.Text)
			case agent.ToolPart:
				if p.Status == agent.ToolRunning {
					fmt.Fprintf(os.Stderr, "\033[33m[%s]\033[0m ", p.ToolName)
				} else if p.Status == agent.ToolFailed {
					fmt.Fprintf(os.Stderr, "\033[31m[%s failed: %s]\033[0m\n", p.ToolName, p.Error)
				}
			}
		}

		// Collect events for batch persistence (don't persist streaming deltas individually).
	}

	fmt.Println() // Final newline

	// Persist the full conversation as two consolidated events (user + assistant).
	if sessionStore != nil {
		userEv := &agent.Event{Author: "user", Parts: []agent.Part{agent.TextPart{Text: message}}}
		sessionStore.AppendEvent(ctx, session.ID, userEv)

		if fullText.Len() > 0 {
			assistantEv := &agent.Event{Author: invCtx.Config.Model, Parts: []agent.Part{agent.TextPart{Text: fullText.String()}}}
			sessionStore.AppendEvent(ctx, session.ID, assistantEv)
		}
	}
}

// exitf prints an error message to stderr and exits with code 1.
func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

// runInstallSkill installs a skill from a git URL or local directory path
// into ~/.cairn/skills/{name}/.
func runInstallSkill(logger *slog.Logger, source string) {
	logger.Info("installing skill", "source", source)

	// Determine install target directory.
	home, err := os.UserHomeDir()
	if err != nil {
		exitf("cannot determine home directory: %v", err)
	}
	installBase := filepath.Join(home, ".cairn", "skills")

	// Ensure the install directory exists.
	if err := os.MkdirAll(installBase, 0755); err != nil {
		exitf("cannot create skill directory %s: %v", installBase, err)
	}

	var srcDir string // directory containing the SKILL.md

	if isGitURL(source) {
		// Clone to a temp directory.
		tmpDir, err := os.MkdirTemp("", "cairn-skill-install-*")
		if err != nil {
			exitf("cannot create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		logger.Info("cloning repository", "url", source)
		cmd := exec.Command("git", "clone", "--depth", "1", "--", source, tmpDir)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if err := cmd.Run(); err != nil {
			exitf("git clone failed: %v", err)
		}

		srcDir, err = findSkillDir(tmpDir)
		if err != nil {
			exitf("%v", err)
		}
	} else {
		// Local path — resolve and verify.
		absPath, err := filepath.Abs(source)
		if err != nil {
			exitf("invalid path %q: %v", source, err)
		}
		logger.Debug("resolving local path", "source", source, "abs", absPath)

		srcDir, err = findSkillDir(absPath)
		if err != nil {
			exitf("%v", err)
		}
	}

	// Verify SKILL.md is a regular file (not a symlink) before parsing.
	skillPath := filepath.Join(srcDir, "SKILL.md")
	fi, err := os.Lstat(skillPath)
	if err != nil {
		exitf("cannot stat SKILL.md: %v", err)
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		exitf("SKILL.md is a symlink — refusing to parse for security")
	}
	if !fi.Mode().IsRegular() {
		exitf("SKILL.md is not a regular file")
	}

	// Parse and validate the SKILL.md.
	sk, err := skill.Parse(skillPath)
	if err != nil {
		exitf("failed to parse SKILL.md: %v", err)
	}

	// Collect known tool names from builtin registry for validation.
	knownToolNames := make([]string, 0, len(builtin.All()))
	for _, t := range builtin.All() {
		knownToolNames = append(knownToolNames, t.Name())
	}

	// Validate.
	issues := skill.Validate(sk, knownToolNames)
	for _, iss := range issues {
		fmt.Printf("  [%s] %s\n", iss.Severity, iss.Message)
	}
	hasErrors := false
	for _, iss := range issues {
		if iss.Severity == skill.SeverityError {
			hasErrors = true
		}
	}
	if hasErrors {
		exitf("skill has validation errors, aborting install")
	}

	// Copy the skill directory to ~/.cairn/skills/{name}/.
	destDir := filepath.Join(installBase, sk.Name)

	// Remove existing if present.
	if _, err := os.Stat(destDir); err == nil {
		fmt.Printf("Replacing existing skill %q...\n", sk.Name)
		if err := os.RemoveAll(destDir); err != nil {
			exitf("cannot remove existing skill: %v", err)
		}
	}

	if err := copyDir(srcDir, destDir); err != nil {
		exitf("failed to copy skill: %v", err)
	}

	fmt.Printf("Installed skill %q to %s\n", sk.Name, destDir)
	fmt.Printf("  Name:        %s\n", sk.Name)
	fmt.Printf("  Description: %s\n", sk.Description)
	fmt.Printf("  Inclusion:   %s\n", sk.Inclusion)
	if len(sk.AllowedTools) > 0 {
		fmt.Printf("  Tools:       %s\n", strings.Join(sk.AllowedTools, ", "))
	}
}

// isGitURL returns true if source looks like a git URL.
// Handles HTTPS (contains "://"), bare .git suffix, and SCP-like URLs (git@host:user/repo).
func isGitURL(source string) bool {
	return strings.Contains(source, "://") ||
		strings.HasSuffix(source, ".git") ||
		(strings.Contains(source, "@") && strings.Contains(source, ":"))
}

// findSkillDir locates the directory containing SKILL.md within root.
// It checks root itself first, then immediate subdirectories.
func findSkillDir(root string) (string, error) {
	// Check root directly.
	if _, err := os.Stat(filepath.Join(root, "SKILL.md")); err == nil {
		return root, nil
	}

	// Check immediate subdirectories.
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("cannot read directory %q: %w", root, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(root, entry.Name(), "SKILL.md")
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Join(root, entry.Name()), nil
		}
	}

	return "", fmt.Errorf("no SKILL.md found in %q or its subdirectories", root)
}

// maxCopyFileSize is the maximum file size (10 MB) allowed during skill copy.
const maxCopyFileSize = 10 * 1024 * 1024

// copyDir recursively copies a directory tree, skipping .git directories,
// rejecting symlinks, and enforcing a per-file size limit.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Compute relative path.
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		// Skip .git directories (cloned repos include these).
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}

		// Reject symlinks to prevent dereferencing arbitrary files.
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symlink found at %s — refusing to copy for security", rel)
		}

		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}

		// Copy regular file using streaming I/O with size limit.
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxCopyFileSize {
			return fmt.Errorf("file %s exceeds maximum size (%d bytes)", rel, maxCopyFileSize)
		}

		return copyFile(path, target, info.Mode())
	})
}

// copyFile streams a single file from src to dst using io.Copy.
func copyFile(src, dst string, perm fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
