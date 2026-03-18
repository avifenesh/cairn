package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/avifenesh/cairn/internal/tool"
)

// webConfig holds web tool configuration, populated from env.
var webConfig struct {
	SearXNGURL   string
	FetchTimeout time.Duration
	FetchMaxSize int64
	HTTPClient   *http.Client
}

// SetWebConfig configures the web tools. Call once at startup.
func SetWebConfig(searxngURL string, fetchTimeout time.Duration, fetchMaxSize int64) {
	webConfig.SearXNGURL = searxngURL
	webConfig.FetchTimeout = fetchTimeout
	if webConfig.FetchTimeout == 0 {
		webConfig.FetchTimeout = 30 * time.Second
	}
	webConfig.FetchMaxSize = fetchMaxSize
	if webConfig.FetchMaxSize == 0 {
		webConfig.FetchMaxSize = 5 * 1024 * 1024 // 5MB
	}
	webConfig.HTTPClient = &http.Client{
		Timeout: webConfig.FetchTimeout,
	}
}

// webSearchParams are the parameters for cairn.webSearch.
type webSearchParams struct {
	Query      string `json:"query" desc:"Search query"`
	NumResults *int   `json:"numResults,omitempty" desc:"Number of results to return (default 5)"`
}

var webSearch = tool.Define("cairn.webSearch",
	"Search the web using SearXNG. Returns titles, URLs, and snippets.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p webSearchParams) (*tool.ToolResult, error) {
		if webConfig.SearXNGURL == "" {
			return &tool.ToolResult{Error: "web search not configured (SEARXNG_URL not set)"}, nil
		}
		if p.Query == "" {
			return &tool.ToolResult{Error: "query is required"}, nil
		}

		numResults := 5
		if p.NumResults != nil && *p.NumResults > 0 {
			numResults = *p.NumResults
		}
		if numResults > 20 {
			numResults = 20
		}

		results, err := doSearXNGSearch(ctx.Cancel, p.Query, numResults)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("web search failed: %v", err)}, nil
		}

		if len(results) == 0 {
			return &tool.ToolResult{Output: "No results found."}, nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Search results for %q:\n\n", p.Query)
		for i, r := range results {
			fmt.Fprintf(&b, "%d. %s\n   %s\n", i+1, r.Title, r.URL)
			if r.Snippet != "" {
				fmt.Fprintf(&b, "   %s\n", r.Snippet)
			}
			b.WriteString("\n")
		}

		return &tool.ToolResult{
			Output:   b.String(),
			Metadata: map[string]any{"count": len(results)},
		}, nil
	},
)

type searchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"content"`
}

func doSearXNGSearch(ctx context.Context, query string, limit int) ([]searchResult, error) {
	u, err := url.Parse(webConfig.SearXNGURL)
	if err != nil {
		return nil, fmt.Errorf("invalid SEARXNG_URL: %w", err)
	}
	u.Path = "/search"
	q := u.Query()
	q.Set("q", query)
	q.Set("format", "json")
	q.Set("pageno", "1")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	client := webConfig.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("SearXNG returned %d: %s", resp.StatusCode, string(body))
	}

	var parsed struct {
		Results []searchResult `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1024*1024)).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to parse SearXNG response: %w", err)
	}

	if len(parsed.Results) > limit {
		parsed.Results = parsed.Results[:limit]
	}
	return parsed.Results, nil
}

// webFetchParams are the parameters for cairn.webFetch.
type webFetchParams struct {
	URL    string `json:"url" desc:"URL to fetch"`
	Format string `json:"format,omitempty" desc:"Output format: text (default), or html"`
}

var webFetch = tool.Define("cairn.webFetch",
	"Fetch a web page and return its content. Truncated to 50K characters.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork},
	func(ctx *tool.ToolContext, p webFetchParams) (*tool.ToolResult, error) {
		if p.URL == "" {
			return &tool.ToolResult{Error: "url is required"}, nil
		}

		// Validate URL scheme.
		parsed, err := url.Parse(p.URL)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("invalid URL: %v", err)}, nil
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return &tool.ToolResult{Error: "only http and https URLs are supported"}, nil
		}

		content, contentType, err := doFetch(ctx.Cancel, p.URL)
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("fetch failed: %v", err)}, nil
		}

		// Truncate to 50K chars.
		const maxChars = 50000
		if runes := []rune(content); len(runes) > maxChars {
			content = string(runes[:maxChars]) + "\n\n[truncated]"
		}

		return &tool.ToolResult{
			Output: content,
			Metadata: map[string]any{
				"url":         p.URL,
				"contentType": contentType,
				"length":      len(content),
			},
		}, nil
	},
)

func doFetch(ctx context.Context, targetURL string) (string, string, error) {
	client := webConfig.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "Cairn/1.0 (personal agent)")
	req.Header.Set("Accept", "text/html, application/json, text/plain, */*")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	maxSize := webConfig.FetchMaxSize
	if maxSize == 0 {
		maxSize = 5 * 1024 * 1024
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSize))
	if err != nil {
		return "", "", fmt.Errorf("failed to read body: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	return string(body), contentType, nil
}
