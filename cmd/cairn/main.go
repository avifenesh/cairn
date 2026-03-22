package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/avifenesh/cairn/internal/agent"
	"github.com/avifenesh/cairn/internal/auth"
	cairnchannel "github.com/avifenesh/cairn/internal/channel"
	"github.com/avifenesh/cairn/internal/config"
	cairncron "github.com/avifenesh/cairn/internal/cron"
	"github.com/avifenesh/cairn/internal/db"
	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/rules"
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
	"github.com/avifenesh/cairn/internal/voice"
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
// newEmbedder creates an embedder from config, returning NoopEmbedder if disabled.
func newEmbedder(cfg *config.Config) memory.Embedder {
	if cfg.EmbeddingEnabled && cfg.EmbeddingAPIKey != "" {
		return memory.NewOpenAIEmbedder(
			cfg.EmbeddingAPIKey, cfg.EmbeddingBaseURL,
			cfg.EmbeddingModel, cfg.EmbeddingDimensions,
		)
	}
	return memory.NoopEmbedder{}
}

func runServe(logger *slog.Logger) {
	// Load config + apply persisted overrides from config.json.
	cfg := config.LoadOptional()
	cfg.LoadOverrides(cfg.DataDir)

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
		} else if cfg.LLMProvider == "glm" {
			// Default GLM fallback chain: glm-5-turbo -> glm-5 -> glm-4.7
			registry.SetFallback("glm-5-turbo", "glm-5")
			registry.SetFallback("glm-5", "glm-4.7")
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
	// ZAI_API_KEY is a separate MCP key from z.ai/manage-apikey; falls back to LLM key.
	zaiKey := cfg.ZaiAPIKey
	if zaiKey == "" {
		zaiKey = cfg.LLMAPIKey
	}
	if cfg.ZaiWebEnabled && zaiKey != "" && cfg.LLMProvider == "glm" {
		builtin.SetZaiConfig(zaiKey, cfg.ZaiBaseURL)
		logger.Info("zai web tools enabled", "baseURL", cfg.ZaiBaseURL)
	}
	if cfg.ZaiVisionEnabled && zaiKey != "" {
		npxPath, err := exec.LookPath("npx")
		if err != nil {
			logger.Warn("vision tools disabled: npx not found in PATH", "error", err)
		} else {
			builtin.SetVisionConfig(zaiKey, npxPath)
			defer builtin.CloseVision()
			logger.Info("zai vision tools enabled", "npx", npxPath)
		}
	}
	builtin.SetWebConfig(cfg.SearXNGURL, time.Duration(cfg.WebFetchTimeout)*time.Second, cfg.WebFetchMaxSize)

	// Configure Google Workspace CLI tools if gws is available.
	gwsPath, gwsErr := exec.LookPath("gws")
	if gwsErr == nil {
		builtin.SetGWSConfig(gwsPath)
		logger.Info("google workspace tools enabled", "gws", gwsPath)
	}

	// Initialize tool registry.
	toolRegistry := tool.NewRegistry()
	toolRegistry.Register(builtin.All()...)

	// Initialize embedder.
	embedder := newEmbedder(cfg)
	if embedder.Dimensions() > 0 {
		logger.Info("embedding enabled",
			"model", cfg.EmbeddingModel,
			"dimensions", cfg.EmbeddingDimensions,
			"baseURL", cfg.EmbeddingBaseURL,
		)
	}

	// Initialize memory service.
	memStore := memory.NewStore(database)
	memService := memory.NewService(memStore, embedder, bus)
	soul := memory.NewSoul(cfg.SoulPath)
	if err := soul.Load(); err != nil {
		logger.Warn("soul: initial load failed (file may not exist yet)", "path", cfg.SoulPath, "error", err)
	} else {
		// Only watch if the file loaded successfully. Avoids noisy stat
		// warnings every 5s when the file doesn't exist.
		soul.OnChange(func(content string) {
			logger.Info("soul reloaded from disk", "path", cfg.SoulPath, "bytes", len(content))
		})
	}

	// Initialize context builder (token-budgeted memory injection).
	ctxBuilder := memory.NewContextBuilder(memStore, embedder, memory.ContextConfig{
		TokenBudget:     cfg.MemoryContextBudget,
		HardRuleReserve: cfg.MemoryHardRuleReserve,
		DecayHalfLife:   cfg.MemoryDecayHalfLife,
		StaleThreshold:  cfg.MemoryStaleThreshold,
	})
	logger.Info("context builder ready", "budget", cfg.MemoryContextBudget, "hardRuleReserve", cfg.MemoryHardRuleReserve)

	// Backfill embeddings for existing memories in background.
	if embedder.Dimensions() > 0 {
		go func() {
			bctx, bcancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer bcancel()
			if err := memService.BackfillEmbeddings(bctx); err != nil {
				logger.Warn("embedding backfill failed", "error", err)
			}
		}()
	}

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

	// Initialize worktree manager for coding task isolation (before engine,
	// so engine.RecoverStuck can clean up orphaned worktrees on restart).
	var worktreeMgr *task.WorktreeManager
	if cfg.CodingEnabled {
		repoDir, err := os.Getwd()
		if err != nil {
			logger.Error("worktree manager: failed to get working directory", "error", err)
		} else {
			worktreeDir := filepath.Join(os.TempDir(), "cairn-worktrees")
			worktreeMgr = task.NewWorktreeManager(repoDir, worktreeDir)
			logger.Info("worktree manager initialized", "repoDir", repoDir, "worktreeDir", worktreeDir,
				"allowedRepos", len(cfg.CodingAllowedRepos))
		}
	}

	// Initialize task engine (receives worktree manager for recovery cleanup).
	taskStore := task.NewStore(database)
	taskEngine := task.NewEngine(taskStore, bus, worktreeMgr)
	taskEngine.StartReaper(1 * time.Minute)
	defer taskEngine.Close()

	// Initialize cron store.
	cronStore := cairncron.NewStore(database.DB)

	// Initialize rules store + engine (optional: automation rules).
	var rulesStore *rules.Store
	var rulesEngine *rules.Engine
	if cfg.RulesEnabled {
		rulesStore = rules.NewStore(database.DB)
		logger.Info("rules store initialized")
	}

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

		// GitHub signal intelligence: external engagement, growth metrics, stargazers, followers, new repos.
		if cfg.GHOwner != "" {
			scheduler.Register(signalplane.NewGitHubSignalPoller(signalplane.GitHubSignalConfig{
				Token:           cfg.GHToken,
				Owner:           cfg.GHOwner,
				TrackedRepos:    cfg.GHTrackedRepos,
				Orgs:            cfg.GHOrgs,
				BotFilter:       cfg.GHBotFilter,
				State:           sourceState,
				Logger:          logger,
				MetricsInterval: time.Duration(cfg.GHMetricsInterval) * time.Second,
			}), pollInterval)
			logger.Info("signal: github_signal poller registered", "owner", cfg.GHOwner, "repos", len(cfg.GHTrackedRepos))
		}
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
			State:    sourceState,
			Logger:   logger,
		}), 4*time.Hour) // download metrics don't change fast
		logger.Info("signal: npm poller registered", "packages", cfg.NPMPackages)
	}

	if len(cfg.CratesPackages) > 0 {
		scheduler.Register(signalplane.NewCratesPoller(signalplane.CratesConfig{
			Crates: cfg.CratesPackages,
			State:  sourceState,
			Logger: logger,
		}), 4*time.Hour)
		logger.Info("signal: crates poller registered", "crates", cfg.CratesPackages)
	}

	// Gmail poller (via gws CLI).
	if gwsErr == nil && cfg.GmailEnabled {
		scheduler.Register(signalplane.NewGmailPoller(signalplane.GmailConfig{
			GWSPath:     gwsPath,
			FilterQuery: cfg.GmailFilterQuery,
			State:       sourceState,
			Logger:      logger,
		}), pollInterval)
		logger.Info("signal: gmail poller registered")
	}

	// Calendar poller (via gws CLI).
	if gwsErr == nil && cfg.CalendarEnabled {
		scheduler.Register(signalplane.NewCalendarPoller(signalplane.CalendarConfig{
			GWSPath:    gwsPath,
			LookaheadH: cfg.CalendarLookaheadH,
			Logger:     logger,
		}), pollInterval)
		logger.Info("signal: calendar poller registered", "lookahead", cfg.CalendarLookaheadH)
	}

	// RSS feed poller.
	if cfg.RSSEnabled && len(cfg.RSSFeeds) > 0 {
		scheduler.Register(signalplane.NewRSSPoller(signalplane.RSSConfig{
			Feeds:  cfg.RSSFeeds,
			Logger: logger,
		}), pollInterval)
		logger.Info("signal: rss poller registered", "feeds", len(cfg.RSSFeeds))
	}

	// Stack Overflow poller.
	if cfg.SOEnabled && len(cfg.SOTags) > 0 {
		scheduler.Register(signalplane.NewSOPoller(signalplane.SOConfig{
			Tags:   cfg.SOTags,
			APIKey: cfg.SOAPIKey,
			Logger: logger,
		}), time.Duration(cfg.SOPollInterval)*time.Minute)
		logger.Info("signal: stackoverflow poller registered", "tags", cfg.SOTags)
	}

	// Dev.to poller.
	if cfg.DevToEnabled && (len(cfg.DevToTags) > 0 || cfg.DevToUsername != "") {
		scheduler.Register(signalplane.NewDevToPoller(signalplane.DevToConfig{
			Tags:     cfg.DevToTags,
			Username: cfg.DevToUsername,
			Logger:   logger,
		}), time.Duration(cfg.DevToPollInterval)*time.Minute)
		logger.Info("signal: devto poller registered", "tags", cfg.DevToTags, "user", cfg.DevToUsername)
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

	// Initialize marketplace client (ClawHub).
	marketplace := skill.NewMarketplaceClient("", logger)

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

	// Initialize memory extractor (used by both agent loop and channel handler).
	var memExtractor *memory.Extractor
	if cfg.MemoryAutoExtract && memService != nil && provider != nil {
		memExtractor = memory.NewExtractor(memService, provider, cfg.LLMModel, logger)
		logger.Info("memory auto-extraction enabled")
	}

	// Create cron, config, rules, and activity adapters (needed by both server and agent loop).
	cronAdapt := &cronAdapter{store: cronStore}
	cfgAdapt := &configAdapter{cfg: cfg}
	// Rules notifier wrapper — the inner notifier is set once channels are configured.
	rulesNotify := &rulesNotifier{}
	var rulesAdapt tool.RulesService
	if rulesStore != nil {
		rulesEngine = rules.NewEngine(rules.EngineDeps{
			Store:    rulesStore,
			Bus:      bus,
			Notifier: rulesNotify,
			Tasks:    &rulesTaskSubmitter{engine: taskEngine},
			Logger:   logger,
		})
		rulesEngine.Start()
		defer rulesEngine.Close()

		rulesAdapt = &rulesAdapter{store: rulesStore, engine: rulesEngine}
		builtin.SetRulesEnabled(true)
		logger.Info("rules engine started")
	}
	activityStore := agent.NewActivityStore(database.DB)
	checkpointStore := agent.NewCheckpointStore(database)

	// Approval store — human-in-the-loop gates (needed by both orchestrator and server).
	approvalStore := task.NewApprovalStore(database.DB)

	// Create subagent runner (available to both Loop and Server).
	var subagentRunner *agent.SubagentRunner
	if provider != nil && toolRegistry != nil {
		subagentRunner = agent.NewSubagentRunner(agent.SubagentRunnerDeps{
			Tasks:          taskEngine,
			Tools:          toolRegistry,
			Provider:       provider,
			Bus:            bus,
			Worktrees:      worktreeMgr,
			Logger:         logger,
			Memories:       memService,
			Soul:           soul,
			ContextBuilder: ctxBuilder,
			Plugins:        pluginMgr,
			ActivityStore:  activityStore,
			ToolMemories:   memAdapter,
			ToolEvents:     eventAdapter,
			ToolDigest:     digestAdapt,
			ToolJournal:    journalAdapt,
			ToolTasks:      taskAdapt,
			ToolStatus:     statusAdapt,
			ToolSkills:     skillAdapt,
			ToolNotifier:   nil, // set later after channels init
			ToolCrons:      cronAdapt,
			ToolRules:      rulesAdapt,
			ToolConfig:     cfgAdapt,
			Model:          cfg.LLMModel,
		})
		logger.Info("subagent runner initialized")
	}

	// Recover stuck tasks unconditionally — tasks die on restart regardless
	// of whether idle mode is enabled.
	recoveryStats := agent.RecoverOnStartup(context.Background(), agent.RecoveryDeps{
		TaskEngine:    taskEngine,
		ActivityStore: activityStore,
		Logger:        logger,
	})
	if recoveryStats.Total > 0 {
		logger.Info("startup recovery complete",
			"recoveredTasks", recoveryStats.Total,
			"requeued", len(recoveryStats.Requeued),
			"failed", len(recoveryStats.Failed))
	}

	// Recover interrupted sessions (clean up checkpoints, log for visibility).
	// RecoverSessions logs its own summary when checkpoints are found.
	agent.RecoverSessions(context.Background(), agent.SessionRecoveryDeps{
		CheckpointStore: checkpointStore,
		TaskEngine:      taskEngine,
		Logger:          logger,
	})

	// Start always-on agent loop (if idle mode enabled and agent available).
	var agentLoop *agent.Loop
	if cfg.IdleModeEnabled && reactAgent != nil && provider != nil {
		journaler := agent.NewJournaler(journalStore, provider, cfg.LLMModel)

		// Use process cwd as repo dir for reflection git context.
		reflectRepoDir, _ := os.Getwd()
		reflector := agent.NewReflectionEngine(journalStore, memService, soul, provider, cfg.LLMModel, agent.ReflectionConfig{
			Interval: time.Duration(cfg.ReflectionInterval) * time.Second,
			RepoDir:  reflectRepoDir,
		})

		// Restore loop state from previous run.
		loopState, _ := agent.RecoverLoopState(context.Background(), database.DB, logger)

		agentLoop = agent.NewLoop(agent.LoopConfig{
			TickInterval:       time.Duration(cfg.AgentTickInterval) * time.Second,
			ReflectionInterval: time.Duration(cfg.ReflectionInterval) * time.Second,
			Model:              cfg.LLMModel,
			IdleEnabled:        true,
			TalkMaxRounds:      cfg.TalkMaxRounds,
			WorkMaxRounds:      cfg.WorkMaxRounds,
			CodingMaxRounds:    cfg.CodingMaxRounds,
			CodingEnabled:      cfg.CodingEnabled,
			CodingAllowedRepos: cfg.CodingAllowedRepos,
			BriefingModel:      cfg.LLMFallbackModel,
		}, agent.LoopDeps{
			Agent:           reactAgent,
			Tasks:           taskEngine,
			Events:          eventStore,
			Memories:        memService,
			Soul:            soul,
			Tools:           toolRegistry,
			Provider:        provider,
			Bus:             bus,
			Journaler:       journaler,
			Extractor:       memExtractor,
			Reflector:       reflector,
			Logger:          logger,
			ToolMemories:    memAdapter,
			ToolEvents:      eventAdapter,
			ToolDigest:      digestAdapt,
			ToolJournal:     journalAdapt,
			ToolTasks:       taskAdapt,
			ToolStatus:      statusAdapt,
			ToolSkills:      skillAdapt,
			ToolCrons:       cronAdapt,
			ToolRules:       rulesAdapt,
			ToolConfig:      cfgAdapt,
			ContextBuilder:  ctxBuilder,
			Plugins:         pluginMgr,
			CronStore:       cronStore,
			ActivityStore:   activityStore,
			Sessions:        sessionStore,
			Checkpoints:     checkpointStore,
			DB:              database.DB,
			SubagentRunner:  subagentRunner,
			WorktreeManager: worktreeMgr,
			Marketplace:     marketplace,
			Approvals:       approvalStore,
		})
		agentLoop.SetInitialState(loopState)
		agentLoop.Start()
		defer agentLoop.Close()
		logger.Info("agent loop started", "tick", cfg.AgentTickInterval, "reflection", cfg.ReflectionInterval,
			"restoredTicks", loopState.TickCount)
	}

	// Initialize voice service (optional).
	var voiceSvc *voice.Service
	if cfg.VoiceEnabled {
		voiceSvc = voice.New(voice.Config{
			WhisperURL: cfg.WhisperURL,
			TTSVoice:   cfg.TTSVoice,
			TTSEnabled: true,
			STTEnabled: true,
			TempDir:    cfg.DataDir,
		}, logger)
		logger.Info("voice enabled", "whisper", cfg.WhisperURL, "ttsVoice", cfg.TTSVoice)
	}

	// WebAuthn auth (biometric login).
	authStore := auth.NewStore(database.DB)
	var webauthnHandler *auth.WebAuthn
	if cfg.FrontendOrigin != "" {
		// Derive RPID (hostname) from FRONTEND_ORIGIN URL.
		var rpID string
		if u, err := url.Parse(cfg.FrontendOrigin); err == nil && u.Hostname() != "" {
			rpID = u.Hostname()
		} else {
			rpID = cfg.FrontendOrigin
		}
		var err error
		webauthnHandler, err = auth.NewWebAuthn("Cairn", rpID, cfg.FrontendOrigin, authStore)
		if err != nil {
			logger.Error("webauthn init failed", "error", err)
		} else {
			logger.Info("webauthn enabled", "rpID", rpID, "origin", cfg.FrontendOrigin)
		}
	}

	// Initialize MCP client manager for external server connections.
	mcpClientMgr := cairnmcp.NewClientManager(toolRegistry, bus, logger)

	// Create and start the server.
	srv := server.New(server.ServerConfig{
		Agent:           reactAgent,
		Sessions:        sessionStore,
		Tasks:           taskEngine,
		Memories:        memService,
		Soul:            soul,
		Tools:           toolRegistry,
		LLM:             provider,
		Bus:             bus,
		Config:          cfg,
		Logger:          logger,
		Webhooks:        webhookHandler,
		ContextBuilder:  ctxBuilder,
		Plugins:         pluginMgr,
		JournalStore:    journalStore,
		ToolMemories:    memAdapter,
		ToolEvents:      eventAdapter,
		ToolDigest:      digestAdapt,
		ToolJournal:     journalAdapt,
		ToolTasks:       taskAdapt,
		ToolStatus:      statusAdapt,
		ToolSkills:      skillAdapt,
		ToolCrons:       cronAdapt,
		ToolRules:       rulesAdapt,
		ToolConfig:      cfgAdapt,
		SubagentRunner:  subagentRunner,
		Voice:           voiceSvc,
		CronStore:       cronStore,
		RulesStore:      rulesStore,
		RulesEngine:     rulesEngine,
		ActivityStore:   activityStore,
		CheckpointStore: checkpointStore,
		Marketplace:     marketplace,
		SkillSuggestor: func() *agent.SkillSuggestor {
			if agentLoop != nil {
				return agentLoop.SkillSuggestor()
			}
			return nil
		}(),
		MCPClients:  mcpClientMgr,
		Approvals:   approvalStore,
		AuthStore:   authStore,
		WebAuthn:    webauthnHandler,
		PollTrigger: scheduler,
	})

	// Graceful shutdown context — all subsystems observe this.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start Soul file watcher (uses shutdown context).
	if soul.Content() != "" {
		go soul.Watch(ctx)
	}

	// Notify adapter — set when channels are configured, nil otherwise.
	var notifyAdapt tool.NotifyService

	// Channel mode overrides (per channel+chatID, in-memory).
	var channelModes sync.Map

	// Start channel router if any channels are configured.
	if cfg.TelegramBotToken != "" || cfg.DiscordBotToken != "" || cfg.SlackBotToken != "" {
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

			// Handle /mode command — switch agent mode for this session.
			if msg.IsCommand && msg.Command == "mode" {
				modeArg := strings.TrimSpace(strings.ToLower(msg.Args))
				switch modeArg {
				case "talk", "work", "coding":
					// Store mode in channel session metadata via a lightweight DB update.
					// We use a dedicated table column or piggyback on the session State.
					// For now, store as a simple key in a session-scoped map.
					channelModeKey := fmt.Sprintf("mode:%s:%s", msg.ChannelID, msg.ChatID)
					channelModes.Store(channelModeKey, modeArg)
					labels := map[string]string{
						"talk":   "Talk (concise answers, 40 rounds)",
						"work":   "Work (run commands, create artifacts, 80 rounds)",
						"coding": "Coding (edit files, git, PRs, 400 rounds)",
					}
					return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Mode: **%s**", labels[modeArg])}, nil
				case "":
					// Show current mode.
					channelModeKey := fmt.Sprintf("mode:%s:%s", msg.ChannelID, msg.ChatID)
					current := "talk"
					if v, ok := channelModes.Load(channelModeKey); ok {
						current = v.(string)
					}
					return &cairnchannel.OutgoingMessage{Text: fmt.Sprintf("Current mode: **%s**\nSwitch with: `/mode talk`, `/mode work`, `/mode coding`", current)}, nil
				default:
					return &cairnchannel.OutgoingMessage{Text: "Unknown mode. Use: `/mode talk`, `/mode work`, `/mode coding`"}, nil
				}
			}

			// Handle /memories command — list, accept, reject, delete, compact, search.
			if msg.IsCommand && msg.Command == "memories" {
				if !isOwnerMessage(msg, cfg) {
					return &cairnchannel.OutgoingMessage{Text: "Not authorized."}, nil
				}
				return handleMemoriesCommand(ctx, msg.Args, memService)
			}

			// Handle /patch command — show, approve, deny pending SOUL patch.
			if msg.IsCommand && msg.Command == "patch" {
				if !isOwnerMessage(msg, cfg) {
					return &cairnchannel.OutgoingMessage{Text: "Not authorized."}, nil
				}
				return handlePatchCommand(ctx, msg.Args, soul)
			}

			// Transcribe voice message before NL parsing so spoken "yes"/"approve" works.
			if len(msg.Audio) > 0 {
				if voiceSvc == nil {
					return &cairnchannel.OutgoingMessage{Text: "Voice messages are not enabled. Please type your message."}, nil
				}
				transcribed, tErr := voiceSvc.Transcribe(ctx, msg.Audio, msg.AudioFilename)
				if tErr != nil {
					logger.Warn("channel: voice transcription failed", "error", tErr)
					return &cairnchannel.OutgoingMessage{Text: "Sorry, I couldn't understand the voice message. Please try again or type your message."}, nil
				}
				if transcribed != "" {
					msg.Text = transcribed
					logger.Info("channel: voice transcribed", "text", transcribed[:min(len(transcribed), 80)])
				}
			}

			// Handle button callbacks and natural language approval intents.
			isCallback := msg.IsCommand && msg.Command == "callback"
			var nlIntent *ApprovalIntent
			if !msg.IsCommand {
				nlIntent = parseApprovalIntent(msg.Text)
			}
			if isCallback || nlIntent != nil {
				if !isOwnerMessage(msg, cfg) {
					return &cairnchannel.OutgoingMessage{Text: "Not authorized."}, nil
				}
				if isCallback {
					return handleCallbackData(ctx, msg.Args, memService, soul, approvalStore, msg.ChannelID)
				}
				return handleApprovalIntent(ctx, nlIntent, memService, soul, approvalStore, msg.ChannelID)
			}

			// Determine mode from /mode command state (default: talk).
			channelMode := tool.ModeTalk
			channelModeKey := fmt.Sprintf("mode:%s:%s", msg.ChannelID, msg.ChatID)
			if v, ok := channelModes.Load(channelModeKey); ok {
				switch v.(string) {
				case "work":
					channelMode = tool.ModeWork
				case "coding":
					channelMode = tool.ModeCoding
				}
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
					Mode:  channelMode,
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
				Mode:           channelMode,
				Session:        session,
				Tools:          toolRegistry,
				LLM:            provider,
				Memory:         memService,
				Soul:           soul,
				Bus:            bus,
				ContextBuilder: ctxBuilder,
				Plugins:        pluginMgr,
				ActivityStore:  activityStore,
				ToolMemories:   memAdapter,
				ToolEvents:     eventAdapter,
				ToolDigest:     digestAdapt,
				ToolJournal:    journalAdapt,
				ToolTasks:      taskAdapt,
				ToolStatus:     statusAdapt,
				ToolSkills:     skillAdapt,
				ToolNotifier:   notifyAdapt,
				ToolCrons:      cronAdapt,
				ToolRules:      rulesAdapt,
				ToolConfig:     cfgAdapt,
				Config:         &agent.AgentConfig{Model: cfg.LLMModel, MaxRounds: cfg.MaxRoundsForMode(string(channelMode))},
				CompactionConfig: agent.CompactionConfig{
					TriggerTokens:   cfg.CompactionTriggerTokens,
					KeepRecentPairs: cfg.CompactionKeepRecent,
					MaxToolOutput:   cfg.CompactionMaxToolOutput,
				},
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

			// Extract memories from channel conversation (fire-and-forget).
			// Use the current exchange text directly (session.Events may be stale).
			if memExtractor != nil && response.Len() > 0 {
				transcript := "User: " + text + "\nAssistant: " + response.String()
				go func() {
					ectx, ecancel := context.WithTimeout(context.Background(), 2*time.Minute)
					defer ecancel()
					memExtractor.Extract(ectx, transcript)
				}()
			}

			out := &cairnchannel.OutgoingMessage{Text: response.String()}

			// Synthesize voice reply for voice messages (cap at 500 chars for fast TTS).
			if len(msg.Audio) > 0 && voiceSvc != nil && response.Len() > 0 {
				ttsText := response.String()
				if len(ttsText) > 500 {
					ttsText = ttsText[:500]
				}
				audio, ttsErr := voiceSvc.Synthesize(ctx, ttsText, "")
				if ttsErr != nil {
					logger.Warn("channel: TTS synthesis failed", "error", ttsErr)
				} else {
					out.Audio = audio
					logger.Info("channel: voice reply synthesized", "audioBytes", len(audio))
				}
			}

			return out, nil
		}

		channelRouter := cairnchannel.NewRouter(channelHandler, logger)
		notifyCfg := &cairnchannel.NotifyConfig{
			PreferredChannel: cfg.PreferredChannel,
			QuietHoursStart:  cfg.QuietHoursStart,
			QuietHoursEnd:    cfg.QuietHoursEnd,
			QuietHoursTZ:     cfg.QuietHoursTZ,
		}
		channelRouter.SetNotifyConfig(notifyCfg)

		// Sync NotifyConfig when runtime config is patched.
		srv.OnConfigPatch = func() {
			notifyCfg.PreferredChannel = cfg.PreferredChannel
			notifyCfg.QuietHoursStart = cfg.QuietHoursStart
			notifyCfg.QuietHoursEnd = cfg.QuietHoursEnd
			notifyCfg.QuietHoursTZ = cfg.QuietHoursTZ
		}

		if cfg.TelegramBotToken != "" {
			tg, err := cairnchannel.NewTelegram(cairnchannel.TelegramConfig{
				BotToken: cfg.TelegramBotToken,
				ChatID:   cfg.TelegramChatID,
			}, channelHandler, logger)
			if err != nil {
				logger.Error("telegram adapter failed", "error", err)
			} else {
				channelRouter.Register(tg)
				builtin.SetTelegramBot(tg.Bot(), cfg.TelegramChatID)
				logger.Info("telegram channel registered", "chatID", cfg.TelegramChatID)
			}
		}

		if cfg.DiscordBotToken != "" {
			dc, err := cairnchannel.NewDiscord(cairnchannel.DiscordConfig{
				BotToken:  cfg.DiscordBotToken,
				ChannelID: cfg.DiscordChannelID,
			}, channelHandler, logger)
			if err != nil {
				logger.Error("discord adapter failed", "error", err)
			} else {
				channelRouter.Register(dc)
				logger.Info("discord channel registered", "channelID", cfg.DiscordChannelID)
			}
		}

		if cfg.SlackBotToken != "" {
			sl, err := cairnchannel.NewSlack(cairnchannel.SlackConfig{
				BotToken:  cfg.SlackBotToken,
				AppToken:  cfg.SlackAppToken,
				ChannelID: cfg.SlackChannelID,
			}, channelHandler, logger)
			if err != nil {
				logger.Error("slack adapter failed", "error", err)
			} else {
				channelRouter.Register(sl)
				logger.Info("slack channel registered", "channelID", cfg.SlackChannelID)
			}
		}

		// Wire notifier adapter for tools + idle loop.
		notifyAdapt = &notifierAdapter{router: channelRouter}
		if agentLoop != nil {
			agentLoop.SetNotifier(notifyAdapt)
		}

		// Wire rules engine notifier (deferred until channels are ready).
		rulesNotify.notifier = notifyAdapt

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
			Notifier: notifyAdapt,
			Crons:    cronAdapt,
			Rules:    rulesAdapt,
			Config:   cfgAdapt,
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

	// Connect to configured external MCP servers.
	if len(cfg.MCPClientServers) > 0 {
		serverConfigs, err := cairnmcp.ParseServerConfigs(cfg.MCPClientServers)
		if err != nil {
			logger.Warn("invalid MCP_SERVERS config", "error", err)
		} else if len(serverConfigs) > 0 {
			mcpClientMgr.ConnectAll(ctx, serverConfigs)
			logger.Info("mcp client connections initiated", "servers", len(serverConfigs))
		}
	}
	defer mcpClientMgr.Close()

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
	} else if cfg.LLMProvider == "glm" {
		registry.SetFallback("glm-5-turbo", "glm-5")
		registry.SetFallback("glm-5", "glm-4.7")
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

	// Initialize memory service.
	chatEmbedder := newEmbedder(cfg)
	var memService *memory.Service
	var soul *memory.Soul
	if database != nil {
		memStore := memory.NewStore(database)
		memService = memory.NewService(memStore, chatEmbedder, bus)
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
		CompactionConfig: agent.CompactionConfig{
			TriggerTokens:   cfg.CompactionTriggerTokens,
			KeepRecentPairs: cfg.CompactionKeepRecent,
			MaxToolOutput:   8000,
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
