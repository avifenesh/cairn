package mcp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// writeToolPrefixes identifies tools that perform write operations.
var writeToolPrefixes = []string{
	"cairn.create", "cairn.manage", "cairn.markRead",
	"cairn.completeTask", "cairn.compose",
	"pub.writeFile", "pub.editFile", "pub.deleteFile",
	"pub.shell", "pub.gitRun",
}

// isWriteTool returns true if the tool performs write operations.
func isWriteTool(name string) bool {
	for _, prefix := range writeToolPrefixes {
		if strings.HasPrefix(name, prefix) || name == prefix {
			return true
		}
	}
	return false
}

// writeRateLimiter tracks per-session write call counts with a sliding window.
type writeRateLimiter struct {
	mu     sync.Mutex
	calls  []time.Time
	limit  int
	window time.Duration
}

func newWriteRateLimiter(limit int, window time.Duration) *writeRateLimiter {
	return &writeRateLimiter{
		limit:  limit,
		window: window,
	}
}

func (r *writeRateLimiter) allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.window)

	// Remove expired entries.
	valid := r.calls[:0]
	for _, t := range r.calls {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	r.calls = valid

	if len(r.calls) >= r.limit {
		return false
	}
	r.calls = append(r.calls, now)
	return true
}

// WriteRateLimitMiddleware returns an MCP tool handler middleware that rate-limits
// write tool calls to the specified limit per window. The limiter is global
// (shared across all sessions) — a per-session limiter would require session
// context from mcp-go which is not available in the middleware chain.
// If limit <= 0, no rate limiting is applied.
func WriteRateLimitMiddleware(limit int, window time.Duration) mcpserver.ToolHandlerMiddleware {
	if limit <= 0 {
		return func(next mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc { return next }
	}
	limiter := newWriteRateLimiter(limit, window)

	return func(next mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if isWriteTool(request.Params.Name) {
				if !limiter.allow() {
					return &mcp.CallToolResult{
						Content: []mcp.Content{
							mcp.NewTextContent(fmt.Sprintf("rate limit exceeded: max %d write calls per %s", limit, window)),
						},
						IsError: true,
					}, nil
				}
			}
			return next(ctx, request)
		}
	}
}
