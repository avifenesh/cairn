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

// callZaiMCP makes a JSON-RPC call to a Z.ai MCP endpoint.
func callZaiMCP(ctx context.Context, service, toolName string, args map[string]any) (string, error) {
	base := strings.TrimRight(zaiConfig.BaseURL, "/")
	endpoint := base + "/" + service + "/mcp"

	body := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		ID:      1,
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

	resp, err := zaiConfig.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("zai: http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return "", fmt.Errorf("zai: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("zai: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	// Z.ai returns SSE format: "id:N\nevent:message\ndata:{json}\n"
	// Extract the JSON from the data: line.
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

		args := map[string]any{"search_query": p.Query}

		text, err := callZaiMCP(safeCtx(ctx.Cancel), "web_search_prime", "web_search_prime", args)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("web search failed: %v", err)}, nil
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
