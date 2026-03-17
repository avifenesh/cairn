package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/avifenesh/pub-go/internal/config"
	"github.com/avifenesh/pub-go/internal/db"
	"github.com/avifenesh/pub-go/internal/eventbus"
	"github.com/avifenesh/pub-go/internal/llm"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if len(os.Args) > 1 && os.Args[1] == "chat" {
		runChat(logger)
		return
	}

	fmt.Println("pub-go: personal agent OS")
	fmt.Println("Usage: pub-go chat \"your message here\"")
}

// runChat implements the Phase 1 deliverable: `pub-go chat "hello"` streams GLM response.
func runChat(logger *slog.Logger) {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: pub-go chat \"your message\"")
		os.Exit(1)
	}
	message := strings.Join(os.Args[2:], " ")

	// Load config
	cfg := config.LoadOptional()
	if cfg.GLMAPIKey == "" {
		fmt.Fprintln(os.Stderr, "Error: GLM_API_KEY environment variable is required")
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

	// Initialize LLM provider
	provider := llm.NewGLMProvider(cfg.GLMAPIKey, cfg.GLMBaseURL, cfg.GLMModel)

	// Wire up event bus to log LLM events
	eventbus.Subscribe(bus, func(e eventbus.StreamStarted) {
		logger.Debug("stream started", "model", e.Model)
	})
	eventbus.Subscribe(bus, func(e eventbus.StreamEnded) {
		logger.Debug("stream ended", "tokens_in", e.InputTokens, "tokens_out", e.OutputTokens)
	})

	// Create context with cancellation on SIGINT
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Build request
	req := &llm.Request{
		Model:     cfg.GLMModel,
		System:    "You are Pub, a personal agent operating system. Be concise and helpful.",
		MaxTokens: 4096,
		Messages: []llm.Message{
			{
				Role:    llm.RoleUser,
				Content: []llm.ContentBlock{llm.TextBlock{Text: message}},
			},
		},
	}

	// Stream response
	eventCh, err := provider.Stream(ctx, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting stream: %v\n", err)
		os.Exit(1)
	}

	var totalIn, totalOut int
	for event := range eventCh {
		switch e := event.(type) {
		case llm.TextDelta:
			fmt.Print(e.Text)
		case llm.ReasoningDelta:
			// Show reasoning in dim color
			fmt.Fprintf(os.Stderr, "\033[2m%s\033[0m", e.Text)
		case llm.ToolCallDelta:
			logger.Debug("tool call", "name", e.Name)
		case llm.MessageEnd:
			totalIn = e.InputTokens
			totalOut = e.OutputTokens
			if e.FinishReason == "network_error" {
				fmt.Fprintln(os.Stderr, "\n[network error — retry not yet implemented in chat mode]")
			}
		case llm.StreamError:
			fmt.Fprintf(os.Stderr, "\nStream error: %v\n", e.Err)
			os.Exit(1)
		}
	}

	fmt.Println() // Final newline
	if totalIn > 0 || totalOut > 0 {
		logger.Info("usage", "input_tokens", totalIn, "output_tokens", totalOut)
	}

	// Publish stream ended event
	eventbus.Publish(bus, eventbus.StreamEnded{
		EventMeta:    eventbus.NewMeta("llm"),
		InputTokens:  totalIn,
		OutputTokens: totalOut,
		FinishReason: "stop",
	})
}
