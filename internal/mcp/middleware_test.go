package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func TestIsWriteTool(t *testing.T) {
	writes := []string{
		"cairn.createTask", "cairn.createMemory",
		"cairn.manageMemory", "cairn.markRead",
		"cairn.completeTask", "cairn.compose",
		"cairn.writeFile", "cairn.editFile", "cairn.deleteFile",
		"cairn.shell", "cairn.gitRun",
	}
	for _, name := range writes {
		if !isWriteTool(name) {
			t.Errorf("expected %q to be a write tool", name)
		}
	}

	reads := []string{
		"cairn.searchMemory", "cairn.readFeed", "cairn.listTasks",
		"cairn.getStatus", "cairn.listSkills", "cairn.loadSkill",
		"cairn.webSearch", "cairn.webFetch", "cairn.digest",
		"cairn.journalSearch", "cairn.readFile", "cairn.listFiles",
		"cairn.searchFiles",
	}
	for _, name := range reads {
		if isWriteTool(name) {
			t.Errorf("expected %q to NOT be a write tool", name)
		}
	}
}

func TestWriteRateLimiter(t *testing.T) {
	limiter := newWriteRateLimiter(3, time.Second)

	// First 3 calls should succeed.
	for i := 0; i < 3; i++ {
		if !limiter.allow() {
			t.Fatalf("call %d should be allowed", i)
		}
	}

	// 4th call should be rate limited.
	if limiter.allow() {
		t.Fatal("4th call should be rate limited")
	}

	// After waiting, should be allowed again.
	time.Sleep(1100 * time.Millisecond)
	if !limiter.allow() {
		t.Fatal("call after window should be allowed")
	}
}

func TestWriteRateLimitMiddleware(t *testing.T) {
	middleware := WriteRateLimitMiddleware(2, time.Second)

	callCount := 0
	inner := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		callCount++
		return mcp.NewToolResultText("ok"), nil
	}

	handler := middleware(mcpserver.ToolHandlerFunc(inner))

	// Write tool — should be rate limited after 2 calls.
	writeReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "cairn.createTask"},
	}
	ctx := context.Background()

	r1, _ := handler(ctx, writeReq)
	if r1.IsError {
		t.Fatal("first write should succeed")
	}
	r2, _ := handler(ctx, writeReq)
	if r2.IsError {
		t.Fatal("second write should succeed")
	}
	r3, _ := handler(ctx, writeReq)
	if !r3.IsError {
		t.Fatal("third write should be rate limited")
	}

	// Read tool — should not be rate limited.
	readReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "cairn.readFeed"},
	}
	r4, _ := handler(ctx, readReq)
	if r4.IsError {
		t.Fatal("read tool should not be rate limited")
	}
}
