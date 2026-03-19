package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/avifenesh/cairn/internal/tool"
)

// zaiConfig holds Z.ai MCP service configuration.
var zaiConfig struct {
	APIKey     string
	BaseURL    string // https://api.z.ai/api/mcp
	HTTPClient *http.Client
	enabled    atomic.Bool
	sessions   sync.Map // service name → session ID
}

// SetZaiConfig configures the Z.ai MCP tools. Call once at startup.
// When enabled and APIKey is set, Z.ai tools replace SearXNG-based tools.
func SetZaiConfig(apiKey, baseURL string) {
	zaiConfig.APIKey = apiKey
	zaiConfig.BaseURL = baseURL
	if zaiConfig.BaseURL == "" {
		zaiConfig.BaseURL = "https://api.z.ai/api/mcp"
	}
	zaiConfig.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	zaiConfig.enabled.Store(apiKey != "")
}

// ZaiEnabled returns true if Z.ai tools are configured.
func ZaiEnabled() bool {
	return zaiConfig.enabled.Load()
}

// --- JSON-RPC types for Z.ai MCP HTTP transport ---

type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	ID      int    `json:"id"`
	Params  any    `json:"params"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// initSession does the MCP initialize handshake and returns the session ID.
func initSession(ctx context.Context, endpoint string) (string, error) {
	body := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		ID:      1,
		Params: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "cairn", "version": "0.1.0"},
		},
	}
	data, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+zaiConfig.APIKey)

	resp, err := zaiConfig.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body) // drain

	sessionID := resp.Header.Get("mcp-session-id")
	if sessionID == "" {
		return "", fmt.Errorf("no mcp-session-id in response")
	}
	return sessionID, nil
}

// getSession returns a cached MCP session for the service, or creates one.
func getSession(ctx context.Context, service, endpoint string) (string, error) {
	if sid, ok := zaiConfig.sessions.Load(service); ok {
		return sid.(string), nil
	}
	sid, err := initSession(ctx, endpoint)
	if err != nil {
		return "", fmt.Errorf("zai: session init: %w", err)
	}
	zaiConfig.sessions.Store(service, sid)
	return sid, nil
}

// callZaiMCP makes a JSON-RPC call to a Z.ai MCP endpoint with session management.
func callZaiMCP(ctx context.Context, service, toolName string, args map[string]any) (string, error) {
	base := strings.TrimRight(zaiConfig.BaseURL, "/")
	endpoint := base + "/" + service + "/mcp"

	sessionID, err := getSession(ctx, service, endpoint)
	if err != nil {
		return "", err
	}

	body := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		ID:      2,
		Params: map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	}

	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("zai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("zai: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+zaiConfig.APIKey)
	req.Header.Set("mcp-session-id", sessionID)

	resp, err := zaiConfig.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("zai: http request: %w", err)
	}
	defer resp.Body.Close()

	// Check for session expiry — retry with new session once.
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		zaiConfig.sessions.Delete(service)
		return callZaiMCP(ctx, service, toolName, args)
	}

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return "", fmt.Errorf("zai: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("zai: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	// Z.ai returns SSE format: "id:N\nevent:message\ndata:{json}\n"
	jsonData := extractSSEData(string(respBody))

	var rpcResp jsonRPCResponse
	if err := json.Unmarshal([]byte(jsonData), &rpcResp); err != nil {
		return "", fmt.Errorf("zai: parse response: %w (raw: %.200s)", err, string(respBody))
	}

	if rpcResp.Error != nil {
		return "", fmt.Errorf("zai: RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	// Check for MCP-level errors (isError flag in CallToolResult).
	var mcpCheck struct {
		IsError bool `json:"isError"`
	}
	if json.Unmarshal(rpcResp.Result, &mcpCheck) == nil && mcpCheck.IsError {
		return "", fmt.Errorf("zai: %s", extractMCPText(rpcResp.Result))
	}

	// Extract text content from MCP CallToolResult format.
	return extractMCPText(rpcResp.Result), nil
}

// extractSSEData extracts the JSON payload from an SSE-formatted response.
// Z.ai MCP endpoints return: "id:N\nevent:message\ndata:{json}\n"
func extractSSEData(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data:") {
			return strings.TrimPrefix(line, "data:")
		}
	}
	// Not SSE format — return as-is (might be plain JSON).
	return body
}

// extractMCPText extracts text from an MCP CallToolResult JSON.
func extractMCPText(raw json.RawMessage) string {
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return string(raw)
	}
	var texts []string
	for _, c := range result.Content {
		if c.Type == "text" && c.Text != "" {
			texts = append(texts, c.Text)
		}
	}
	if len(texts) == 0 {
		return string(raw)
	}
	return strings.Join(texts, "\n")
}

// --- Z.ai Web Search ---

type zaiWebSearchParams struct {
	Query      string `json:"query" desc:"Search query"`
	NumResults *int   `json:"numResults,omitempty" desc:"Number of results (default 5)"`
}

var zaiWebSearch = tool.Define("cairn.webSearch",
	"Search the web using Z.ai. Returns titles, URLs, and summaries.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p zaiWebSearchParams) (*tool.ToolResult, error) {
		if !ZaiEnabled() {
			return &tool.ToolResult{Error: "Z.ai web search not configured (no API key)"}, nil
		}
		if p.Query == "" {
			return &tool.ToolResult{Error: "query is required"}, nil
		}

		// Use MCP web_search_prime (coding plan quota) as primary.
		// REST API at /api/paas/v4/web_search is pay-as-you-go and returns error 1113
		// for coding plan users without separate balance.
		// Note: actual tool name is "web_search_prime" (snake_case), NOT "webSearchPrime".
		args := map[string]any{
			"search_query": p.Query,
			"location":     "us",
			"content_size": "medium",
		}
		text, err := callZaiMCP(safeCtx(ctx.Cancel), "web_search_prime", "web_search_prime", args)
		if err != nil {
			// MCP failed — try SearXNG, then REST, then give up.
			if result := trySearXNG(safeCtx(ctx.Cancel), p.Query, p.NumResults); result != nil {
				return result, nil
			}
			text, restErr := callZaiWebSearchREST(safeCtx(ctx.Cancel), p.Query, p.NumResults)
			if restErr != nil {
				return &tool.ToolResult{Error: fmt.Sprintf("web search failed: %v (REST: %v)", err, restErr)}, nil
			}
			return &tool.ToolResult{
				Output:   text,
				Metadata: map[string]any{"provider": "zai-rest"},
			}, nil
		}

		// MCP returns "[]" when quota is exhausted or platform bug.
		trimmed := strings.TrimSpace(text)
		if trimmed == "" || trimmed == "\"[]\"" || trimmed == "[]" {
			if result := trySearXNG(safeCtx(ctx.Cancel), p.Query, p.NumResults); result != nil {
				return result, nil
			}
			return &tool.ToolResult{Output: "No search results found."}, nil
		}

		return &tool.ToolResult{
			Output:   text,
			Metadata: map[string]any{"provider": "zai"},
		}, nil
	},
)

// --- Z.ai Web Reader ---

type zaiWebReaderParams struct {
	URL string `json:"url" desc:"URL to read"`
}

// callZaiWebSearchREST calls the Z.ai REST web search API directly.
// POST /api/paas/v4/web_search with search_engine=search-prime.
func callZaiWebSearchREST(ctx context.Context, query string, numResults *int) (string, error) {
	count := 10
	if numResults != nil && *numResults > 0 {
		count = *numResults
		if count > 50 {
			count = 50
		}
	}

	body := map[string]any{
		"search_engine": "search-prime",
		"search_query":  query,
		"count":         count,
	}
	data, _ := json.Marshal(body)

	// Use the coding plan base URL for REST API
	restURL := strings.Replace(zaiConfig.BaseURL, "/api/mcp", "/api/paas/v4/web_search", 1)
	if !strings.Contains(restURL, "web_search") {
		restURL = "https://api.z.ai/api/paas/v4/web_search"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, restURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("zai search: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+zaiConfig.APIKey)

	resp, err := zaiConfig.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("zai search: http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return "", fmt.Errorf("zai search: read response: %w", err)
	}

	// Check for API error
	var errResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Code != "" {
		return "", fmt.Errorf("zai search: %s — %s", errResp.Error.Code, errResp.Error.Message)
	}

	// Parse search results
	var searchResp struct {
		SearchResult []struct {
			Title   string `json:"title"`
			Content string `json:"content"`
			Link    string `json:"link"`
			Media   string `json:"media"`
		} `json:"search_result"`
	}
	if err := json.Unmarshal(respBody, &searchResp); err != nil {
		return "", fmt.Errorf("zai search: parse: %w", err)
	}

	if len(searchResp.SearchResult) == 0 {
		return "No search results found.", nil
	}

	var sb strings.Builder
	for i, r := range searchResp.SearchResult {
		fmt.Fprintf(&sb, "%d. **%s**\n   %s\n   URL: %s\n   Source: %s\n\n", i+1, r.Title, r.Content, r.Link, r.Media)
	}
	return sb.String(), nil
}

var zaiWebReader = tool.Define("cairn.webFetch",
	"Fetch and extract content from a web page using Z.ai.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p zaiWebReaderParams) (*tool.ToolResult, error) {
		if !ZaiEnabled() {
			return &tool.ToolResult{Error: "Z.ai web reader not configured (no API key)"}, nil
		}
		if p.URL == "" {
			return &tool.ToolResult{Error: "url is required"}, nil
		}

		// Validate URL scheme and block SSRF (same as direct fetch).
		parsed, err := url.Parse(p.URL)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("invalid URL: %v", err)}, nil
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return &tool.ToolResult{Error: "only http and https URLs are supported"}, nil
		}
		if err := validateHost(parsed.Hostname()); err != nil {
			return &tool.ToolResult{Error: err.Error()}, nil
		}

		text, err := callZaiMCP(safeCtx(ctx.Cancel), "web_reader", "webReader", map[string]any{"url": p.URL})
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("web fetch failed: %v", err)}, nil
		}

		// Truncate to 50K chars.
		const maxChars = 50000
		if len(text) > maxChars {
			if runes := []rune(text); len(runes) > maxChars {
				text = string(runes[:maxChars]) + "\n\n[truncated]"
			}
		}

		return &tool.ToolResult{
			Output: text,
			Metadata: map[string]any{
				"provider": "zai",
				"url":      p.URL,
				"length":   len(text),
			},
		}, nil
	},
)

// --- Z.ai Zread (repo docs/code) ---

type zaiSearchDocParams struct {
	Query string `json:"query" desc:"Search query for repository documentation, issues, PRs"`
	Repo  string `json:"repo,omitempty" desc:"Repository in owner/name format (e.g. 'facebook/react')"`
}

var zaiSearchDoc = tool.Define("cairn.searchDoc",
	"Search open-source repository documentation, issues, and PRs using Z.ai.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p zaiSearchDocParams) (*tool.ToolResult, error) {
		if !ZaiEnabled() {
			return &tool.ToolResult{Error: "Z.ai not configured"}, nil
		}
		if p.Query == "" {
			return &tool.ToolResult{Error: "query is required"}, nil
		}

		args := map[string]any{"query": p.Query}
		if p.Repo != "" {
			args["repo_name"] = p.Repo
		}

		text, err := callZaiMCP(safeCtx(ctx.Cancel), "zread", "search_doc", args)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("search doc failed: %v", err)}, nil
		}

		return &tool.ToolResult{
			Output:   text,
			Metadata: map[string]any{"provider": "zai"},
		}, nil
	},
)

type zaiRepoStructureParams struct {
	Repo string `json:"repo" desc:"Repository in owner/name format (e.g. 'facebook/react')"`
	Path string `json:"path,omitempty" desc:"Subdirectory path (default: root)"`
}

var zaiRepoStructure = tool.Define("cairn.repoStructure",
	"Get directory structure of an open-source repository using Z.ai.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p zaiRepoStructureParams) (*tool.ToolResult, error) {
		if !ZaiEnabled() {
			return &tool.ToolResult{Error: "Z.ai not configured"}, nil
		}
		if p.Repo == "" {
			return &tool.ToolResult{Error: "repo is required"}, nil
		}

		args := map[string]any{"repo_name": p.Repo}
		if p.Path != "" {
			args["dir_path"] = p.Path
		}

		text, err := callZaiMCP(safeCtx(ctx.Cancel), "zread", "get_repo_structure", args)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("repo structure failed: %v", err)}, nil
		}

		return &tool.ToolResult{
			Output:   text,
			Metadata: map[string]any{"provider": "zai"},
		}, nil
	},
)

type zaiReadFileParams struct {
	Repo string `json:"repo" desc:"Repository in owner/name format"`
	Path string `json:"path" desc:"File path within the repository"`
}

var zaiReadRepoFile = tool.Define("cairn.readRepoFile",
	"Read a file from an open-source repository using Z.ai.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p zaiReadFileParams) (*tool.ToolResult, error) {
		if !ZaiEnabled() {
			return &tool.ToolResult{Error: "Z.ai not configured"}, nil
		}
		if p.Repo == "" || p.Path == "" {
			return &tool.ToolResult{Error: "repo and path are required"}, nil
		}

		text, err := callZaiMCP(safeCtx(ctx.Cancel), "zread", "read_file", map[string]any{
			"repo_name": p.Repo,
			"file_path": p.Path,
		})
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("read file failed: %v", err)}, nil
		}

		return &tool.ToolResult{
			Output:   text,
			Metadata: map[string]any{"provider": "zai", "repo": p.Repo, "path": p.Path},
		}, nil
	},
)

// trySearXNG attempts a SearXNG search if configured. Returns nil if unavailable or no results.
func trySearXNG(ctx context.Context, query string, numResults *int) *tool.ToolResult {
	if webConfig.SearXNGURL == "" {
		return nil
	}
	count := 10
	if numResults != nil && *numResults > 0 {
		count = *numResults
	}
	results, err := doSearXNGSearch(ctx, query, count)
	if err != nil || len(results) == 0 {
		return nil
	}
	var sb strings.Builder
	for i, r := range results {
		fmt.Fprintf(&sb, "%d. **%s**\n   %s\n   URL: %s\n\n", i+1, r.Title, r.Snippet, r.URL)
	}
	return &tool.ToolResult{
		Output:   sb.String(),
		Metadata: map[string]any{"provider": "searxng", "count": len(results)},
	}
}
