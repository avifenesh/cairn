package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/avifenesh/cairn/internal/tool"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// registerResources adds Cairn's feed and memory resources to the MCP server.
func registerResources(srv *mcpserver.MCPServer, events tool.EventService, memories tool.MemoryService) {
	if events != nil {
		srv.AddResource(
			mcp.NewResource("cairn://feed", "Feed Events",
				mcp.WithResourceDescription("Recent signal plane events from GitHub, HN, Reddit, npm, crates, and agent messages"),
				mcp.WithMIMEType("application/json"),
			),
			feedResourceHandler(events),
		)
	}

	if memories != nil {
		srv.AddResource(
			mcp.NewResource("cairn://memories", "Memories",
				mcp.WithResourceDescription("Accepted semantic memories — facts, preferences, hard rules, decisions"),
				mcp.WithMIMEType("application/json"),
			),
			memoriesResourceHandler(memories),
		)
	}
}

// feedResourceHandler returns recent feed events as JSON.
func feedResourceHandler(events tool.EventService) mcpserver.ResourceHandlerFunc {
	return func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		evts, err := events.List(ctx, tool.EventFilter{
			UnreadOnly: false,
			Limit:      50,
		})
		if err != nil {
			return nil, fmt.Errorf("feed resource: %w", err)
		}

		type feedItem struct {
			ID     string `json:"id"`
			Source string `json:"source"`
			Kind   string `json:"kind"`
			Title  string `json:"title"`
			URL    string `json:"url,omitempty"`
			Actor  string `json:"actor,omitempty"`
		}

		items := make([]feedItem, len(evts))
		for i, ev := range evts {
			items[i] = feedItem{
				ID:     ev.ID,
				Source: ev.Source,
				Kind:   ev.Kind,
				Title:  ev.Title,
				URL:    ev.URL,
				Actor:  ev.Actor,
			}
		}

		data, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return nil, err
		}

		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "cairn://feed",
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	}
}

// memoriesResourceHandler returns accepted memories as JSON.
func memoriesResourceHandler(memories tool.MemoryService) mcpserver.ResourceHandlerFunc {
	return func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Search with empty query returns all (keyword search fallback).
		results, err := memories.Search(ctx, "", 50)
		if err != nil {
			return nil, fmt.Errorf("memories resource: %w", err)
		}

		type memItem struct {
			ID       string  `json:"id"`
			Content  string  `json:"content"`
			Category string  `json:"category"`
			Scope    string  `json:"scope"`
			Score    float64 `json:"score"`
		}

		items := make([]memItem, len(results))
		for i, r := range results {
			items[i] = memItem{
				ID:       r.Memory.ID,
				Content:  r.Memory.Content,
				Category: r.Memory.Category,
				Scope:    r.Memory.Scope,
				Score:    r.Score,
			}
		}

		data, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return nil, err
		}

		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "cairn://memories",
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	}
}
