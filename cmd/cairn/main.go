package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/avifenesh/cairn/internal/agent"
	"github.com/avifenesh/cairn/internal/config"
	"github.com/avifenesh/cairn/internal/db"
	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/llm"
	"github.com/avifenesh/cairn/internal/memory"
	"github.com/avifenesh/cairn/internal/plugin"
	"github.com/avifenesh/cairn/internal/server"
	signalplane "github.com/avifenesh/cairn/internal/signal"
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
		}
	}

	fmt.Println("cairn: personal agent OS")
	fmt.Printf("version %s (%s)\n\n", version, commit)
	fmt.Println("Usage: cairn chat \"your message here\"")
	fmt.Println("       cairn chat --mode coding \"your message\"")
	fmt.Println("       cairn serve               # start HTTP server")
	fmt.Println("       cairn version             # show version info")
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
			Agent:     reactAgent,
			Tasks:     taskEngine,
			Events:    eventStore,
			Memories:  memService,
			Soul:      soul,
			Tools:     toolRegistry,
			Provider:  provider,
			Bus:       bus,
			Journaler: journaler,
			Reflector: reflector,
			Logger:    logger,
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
	})

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	// Build invocation context.
	invCtx := &agent.InvocationContext{
		Context:     ctx,
		SessionID:   session.ID,
		UserMessage: message,
		Mode:        mode,
		Session:     session,
		Tools:       toolRegistry,
		LLM:         provider,
		Memory:      memService,
		Soul:        soul,
		Bus:         bus,
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
