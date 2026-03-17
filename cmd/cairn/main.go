package main

import (
	"context"
	crypto_rand "crypto/rand"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/avifenesh/cairn/internal/agent"
	"github.com/avifenesh/cairn/internal/config"
	"github.com/avifenesh/cairn/internal/db"
	"github.com/avifenesh/cairn/internal/eventbus"
	"github.com/avifenesh/cairn/internal/llm"
	"github.com/avifenesh/cairn/internal/memory"
	"github.com/avifenesh/cairn/internal/tool"
	"github.com/avifenesh/cairn/internal/tool/builtin"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if len(os.Args) > 1 && os.Args[1] == "chat" {
		runChat(logger)
		return
	}

	fmt.Println("cairn: personal agent OS")
	fmt.Println("Usage: cairn chat \"your message here\"")
	fmt.Println("       cairn chat --mode coding \"your message\"")
}

// runChat implements the Phase 1 deliverable: `cairn chat "hello"` streams GLM response.
func runChat(logger *slog.Logger) {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: cairn chat \"your message\"")
		os.Exit(1)
	}
	message := strings.Join(os.Args[2:], " ")

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
	sessionID := newSessionID()
	session := &agent.Session{
		ID:    sessionID,
		Mode:  mode,
		State: map[string]any{"workDir": "."},
	}
	if sessionStore != nil {
		sessionStore.Create(ctx, session)
	}

	// Build invocation context.
	invCtx := &agent.InvocationContext{
		Context:     ctx,
		SessionID:   sessionID,
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

	// Run the agent.
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

		// Persist event to session.
		if sessionStore != nil && ev.Event.Author != "user" {
			sessionStore.AppendEvent(ctx, sessionID, ev.Event)
		}
	}

	fmt.Println() // Final newline
}

func newSessionID() string {
	b := make([]byte, 16)
	crypto_rand.Read(b)
	return fmt.Sprintf("%x", b)
}
