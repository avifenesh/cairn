// Package mcp provides an MCP (Model Context Protocol) server that exposes
// Cairn's tools and resources to external MCP clients like Claude Code and Cursor.
package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/avifenesh/cairn/internal/tool"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// Config holds MCP server configuration.
type Config struct {
	Port           int    // HTTP/SSE port (default 3001)
	Transport      string // "stdio", "http", or "both"
	WriteRateLimit int    // Max write tool calls per minute (default 100)
}

// Server wraps an MCP server with Cairn's tools and resources.
type Server struct {
	mcpSrv  *mcpserver.MCPServer
	httpSrv *mcpserver.StreamableHTTPServer
	config  Config
	logger  *slog.Logger
}

// New creates an MCP server with all Cairn tools and resources registered.
func New(cfg Config, reg *tool.Registry, toolCtx *tool.ToolContext, logger *slog.Logger) *Server {
	if cfg.Port == 0 {
		cfg.Port = 3001
	}
	if cfg.Transport == "" {
		cfg.Transport = "http"
	}
	if cfg.WriteRateLimit == 0 {
		cfg.WriteRateLimit = 100
	}
	if logger == nil {
		logger = slog.Default()
	}

	// Create MCP server with capabilities.
	mcpSrv := mcpserver.NewMCPServer(
		"cairn",
		"1.0.0",
		mcpserver.WithToolCapabilities(true),
		mcpserver.WithResourceCapabilities(true, true),
		mcpserver.WithRecovery(),
		mcpserver.WithToolHandlerMiddleware(
			WriteRateLimitMiddleware(cfg.WriteRateLimit, time.Minute),
		),
	)

	// Require toolCtx — handlers will panic without it.
	if toolCtx == nil {
		panic("mcp.New: toolCtx must not be nil")
	}

	// Register all Cairn tools.
	registerTools(mcpSrv, reg, toolCtx)

	// Register resources (nil-safe — only registers if service available).
	if toolCtx != nil {
		registerResources(mcpSrv, toolCtx.Events, toolCtx.Memories)
	}

	logger.Info("mcp server created",
		"tools", len(reg.All()),
		"transport", cfg.Transport,
		"port", cfg.Port,
	)

	return &Server{
		mcpSrv: mcpSrv,
		config: cfg,
		logger: logger,
	}
}

// MCPServer returns the underlying mcp-go server (for testing).
func (s *Server) MCPServer() *mcpserver.MCPServer {
	return s.mcpSrv
}

// ServeStdio starts the MCP server on stdio (stdin/stdout).
// Blocks until ctx is cancelled or ServeStdio returns.
// Note: the underlying mcp-go ServeStdio handles its own signal management;
// when ctx is cancelled this function returns but the stdio goroutine may
// still be draining. This is acceptable for shutdown.
func (s *Server) ServeStdio(ctx context.Context) error {
	s.logger.Info("mcp stdio transport starting")
	errCh := make(chan error, 1)
	go func() {
		errCh <- mcpserver.ServeStdio(s.mcpSrv)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ServeHTTP starts the MCP server on HTTP/SSE.
// Blocks until an error occurs.
func (s *Server) ServeHTTP() error {
	addr := fmt.Sprintf(":%d", s.config.Port)
	s.logger.Info("mcp http transport starting", "addr", addr)

	s.httpSrv = mcpserver.NewStreamableHTTPServer(s.mcpSrv)
	return s.httpSrv.Start(addr)
}

// ToolNames returns the names of all registered MCP tools (for logging/testing).
func (s *Server) ToolNames() []string {
	tools := s.mcpSrv.ListTools()
	result := make([]string, 0, len(tools))
	for name := range tools {
		result = append(result, name)
	}
	return result
}
