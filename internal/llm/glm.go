package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

const (
	defaultGLMModel     = "glm-5-turbo"
	defaultGLMBaseURL   = "https://api.z.ai/api/coding/paas/v4"
	defaultGLMMaxTokens = 32768
	defaultGLMTemp      = 0.7
)

// GLMProvider implements Provider for Z.ai's GLM API (OpenAI-compatible with extensions).
type GLMProvider struct {
	apiKey     string // format: "id.secret", sent as Bearer token
	baseURL    string // e.g. https://api.z.ai/api/coding/paas/v4
	httpClient *http.Client
	model      string // default: "glm-5-turbo"
	logger     *slog.Logger
}

// NewGLMProvider creates a GLM provider. apiKey is required (format "id.secret").
// If baseURL is empty, defaults to the coding plan endpoint.
// If model is empty, defaults to "glm-5-turbo".
func NewGLMProvider(apiKey, baseURL, model string) *GLMProvider {
	if baseURL == "" {
		baseURL = defaultGLMBaseURL
	}
	if model == "" {
		model = defaultGLMModel
	}
	return &GLMProvider{
		apiKey:     apiKey,
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{},
		model:      model,
		logger:     slog.Default(),
	}
}

// SetLogger sets a custom logger for the provider.
func (p *GLMProvider) SetLogger(l *slog.Logger) {
	p.logger = l
}

// ID returns "glm".
func (p *GLMProvider) ID() string { return "glm" }

// Models returns the list of models this provider supports.
func (p *GLMProvider) Models() []ModelInfo {
	return []ModelInfo{
		{
			ID:              "glm-5-turbo",
			DisplayName:     "GLM-5 Turbo",
			MaxTokens:       128000,
			CostPer1MInput:  0.0, // included in subscription
			CostPer1MOutput: 0.0,
		},
		{
			ID:              "glm-4.7",
			DisplayName:     "GLM-4.7",
			MaxTokens:       128000,
			CostPer1MInput:  0.0,
			CostPer1MOutput: 0.0,
		},
	}
}

// glmRequest is the JSON body sent to the GLM chat completions endpoint.
type glmRequest struct {
	Model       string       `json:"model"`
	Messages    []glmMessage `json:"messages"`
	Stream      bool         `json:"stream"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	Temperature *float64     `json:"temperature,omitempty"`
	Stop        []string     `json:"stop,omitempty"`
	Tools       []any        `json:"tools,omitempty"` // glmToolDef (function) or glmWebSearchTool
	Thinking    *glmThinking `json:"thinking,omitempty"`
}

// glmThinking represents the thinking/reasoning parameter for GLM models.
// Must be {"type": "enabled"}, NOT a boolean — boolean causes HTTP 400.
type glmThinking struct {
	Type string `json:"type"`
}

type glmMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolCalls  []glmToolCall   `json:"tool_calls,omitempty"`
}

type glmToolDef struct {
	Type     string         `json:"type"`
	Function glmToolDefFunc `json:"function"`
}

// glmWebSearchTool is the built-in web search tool for GLM chat completions.
// Unlike function tools, search results are returned inline in the response.
type glmWebSearchTool struct {
	Type      string              `json:"type"`
	WebSearch glmWebSearchOptions `json:"web_search"`
}

type glmWebSearchOptions struct {
	Enable              string `json:"enable"`
	SearchEngine        string `json:"search_engine"`
	SearchResult        string `json:"search_result"`
	ContentSize         string `json:"content_size,omitempty"`
	SearchRecencyFilter string `json:"search_recency_filter,omitempty"`
}

type glmToolDefFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type glmToolCall struct {
	Index    int             `json:"index"`
	ID       string          `json:"id,omitempty"`
	Type     string          `json:"type,omitempty"`
	Function glmToolCallFunc `json:"function"`
}

type glmToolCallFunc struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// glmChunk represents a single SSE chunk from the GLM streaming response.
type glmChunk struct {
	ID      string      `json:"id"`
	Model   string      `json:"model"`
	Choices []glmChoice `json:"choices"`
	Usage   *glmUsage   `json:"usage,omitempty"`
}

type glmChoice struct {
	Index        int      `json:"index"`
	Delta        glmDelta `json:"delta"`
	FinishReason *string  `json:"finish_reason,omitempty"`
}

type glmDelta struct {
	Role             string        `json:"role,omitempty"`
	Content          *string       `json:"content,omitempty"`
	ReasoningContent *string       `json:"reasoning_content,omitempty"`
	ToolCalls        []glmToolCall `json:"tool_calls,omitempty"`
}

type glmUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Stream sends a request to GLM and returns a channel of streaming events.
func (p *GLMProvider) Stream(ctx context.Context, req *Request) (<-chan Event, error) {
	body, err := p.buildRequestBody(req)
	if err != nil {
		return nil, fmt.Errorf("glm: build request: %w", err)
	}

	endpoint := p.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("glm: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("glm: http request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("glm: http %d: %s", resp.StatusCode, string(errBody))
	}

	ch := make(chan Event, 32)
	go p.processStream(ctx, resp.Body, ch, req.Model)
	return ch, nil
}

// buildRequestBody marshals the request into GLM's JSON format.
func (p *GLMProvider) buildRequestBody(req *Request) ([]byte, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultGLMMaxTokens
	}

	temp := req.Temperature
	if temp == nil {
		t := defaultGLMTemp
		temp = &t
	}

	// Enable thinking for capable models unless explicitly disabled.
	var thinking *glmThinking
	if req.DisableThinking {
		thinking = &glmThinking{Type: "disabled"}
	} else if strings.Contains(model, "turbo") || strings.HasPrefix(model, "glm-4.") {
		thinking = &glmThinking{Type: "enabled"}
	}

	glmReq := glmRequest{
		Model:       model,
		Stream:      true,
		MaxTokens:   maxTokens,
		Temperature: temp,
		Stop:        req.Stop,
		Thinking:    thinking,
	}

	// Build messages.
	var msgs []glmMessage

	// System message first (if provided).
	if req.System != "" {
		sysContent, _ := json.Marshal(req.System)
		msgs = append(msgs, glmMessage{
			Role:    "system",
			Content: sysContent,
		})
	}

	// Convert request messages.
	for _, m := range req.Messages {
		glmMsg := convertMessage(m)
		msgs = append(msgs, glmMsg...)
	}

	glmReq.Messages = msgs

	// Convert tools.
	if len(req.Tools) > 0 {
		for _, t := range req.Tools {
			glmReq.Tools = append(glmReq.Tools, glmToolDef{
				Type: "function",
				Function: glmToolDefFunc{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  t.Parameters,
				},
			})
		}
	}

	// Add built-in web search tool when enabled.
	// This uses GLM's native search — results come back inline in the response,
	// consuming prompt quota instead of MCP quota.
	if req.EnableWebSearch {
		glmReq.Tools = append(glmReq.Tools, glmWebSearchTool{
			Type: "web_search",
			WebSearch: glmWebSearchOptions{
				Enable:       "True",
				SearchEngine: "search-prime",
				SearchResult: "True",
				ContentSize:  "medium",
			},
		})
	}

	return json.Marshal(glmReq)
}

// convertMessage converts an internal Message to one or more GLM messages.
func convertMessage(m Message) []glmMessage {
	switch m.Role {
	case RoleTool:
		// Tool result messages.
		var msgs []glmMessage
		for _, block := range m.Content {
			if tr, ok := block.(ToolResultBlock); ok {
				content, _ := json.Marshal(tr.Content)
				msgs = append(msgs, glmMessage{
					Role:       "tool",
					Content:    content,
					ToolCallID: tr.ToolUseID,
				})
			}
		}
		return msgs

	case RoleAssistant:
		// Assistant messages may contain text + tool calls.
		msg := glmMessage{Role: "assistant"}
		var textParts []string
		var toolCalls []glmToolCall

		for _, block := range m.Content {
			switch b := block.(type) {
			case TextBlock:
				textParts = append(textParts, b.Text)
			case ToolUseBlock:
				toolCalls = append(toolCalls, glmToolCall{
					Index: len(toolCalls),
					ID:    b.ID,
					Type:  "function",
					Function: glmToolCallFunc{
						Name:      b.Name,
						Arguments: string(b.Input),
					},
				})
			}
		}

		if len(textParts) > 0 {
			joined, _ := json.Marshal(strings.Join(textParts, ""))
			msg.Content = joined
		}
		if len(toolCalls) > 0 {
			msg.ToolCalls = toolCalls
		}
		return []glmMessage{msg}

	default:
		// User and system messages — concatenate text blocks.
		msg := glmMessage{Role: string(m.Role)}
		var textParts []string
		for _, block := range m.Content {
			if tb, ok := block.(TextBlock); ok {
				textParts = append(textParts, tb.Text)
			}
		}
		if len(textParts) > 0 {
			joined, _ := json.Marshal(strings.Join(textParts, ""))
			msg.Content = joined
		}
		return []glmMessage{msg}
	}
}

// processStream reads SSE events from the response body and emits Event values.
func (p *GLMProvider) processStream(ctx context.Context, body io.ReadCloser, ch chan<- Event, modelHint string) {
	defer close(ch)
	defer body.Close()

	// Track accumulated tool calls keyed by index.
	type toolCallAcc struct {
		id   string
		name string
		args strings.Builder
	}
	toolCalls := make(map[int]*toolCallAcc)

	sseEvents := ParseSSE(ctx, body)

	for sse := range sseEvents {
		// Check context cancellation.
		select {
		case <-ctx.Done():
			return
		default:
		}

		if sse.Err != nil {
			sendEvent(ctx, ch, StreamError{Err: sse.Err, Retryable: false})
			return
		}

		// Parse the JSON chunk.
		var chunk glmChunk
		if err := json.Unmarshal([]byte(sse.Data), &chunk); err != nil {
			p.logger.Warn("glm: failed to parse chunk", "error", err, "data", sse.Data)
			continue
		}

		if modelHint == "" && chunk.Model != "" {
			modelHint = chunk.Model
		}

		for _, choice := range chunk.Choices {
			delta := choice.Delta

			// Reasoning content (thinking trace).
			if delta.ReasoningContent != nil && *delta.ReasoningContent != "" {
				sendEvent(ctx, ch, ReasoningDelta{Text: *delta.ReasoningContent})
			}

			// Text content.
			if delta.Content != nil && *delta.Content != "" {
				sendEvent(ctx, ch, TextDelta{Text: *delta.Content})
			}

			// Tool calls — accumulate fragments.
			for _, tc := range delta.ToolCalls {
				acc, exists := toolCalls[tc.Index]
				if !exists {
					acc = &toolCallAcc{}
					toolCalls[tc.Index] = acc
				}
				if tc.ID != "" {
					acc.id = tc.ID
				}
				if tc.Function.Name != "" {
					acc.name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					acc.args.WriteString(tc.Function.Arguments)
				}
			}

			// Finish reason.
			if choice.FinishReason != nil {
				reason := *choice.FinishReason

				// network_error is a known GLM issue — retryable.
				if reason == "network_error" {
					sendEvent(ctx, ch, StreamError{
						Err:       fmt.Errorf("glm: finish_reason=network_error"),
						Retryable: true,
					})
					return
				}

				// Flush accumulated tool calls before message end.
				for idx := 0; idx < len(toolCalls); idx++ {
					if acc, ok := toolCalls[idx]; ok {
						var input json.RawMessage
						args := acc.args.String()
						if args != "" {
							input = json.RawMessage(args)
						}
						sendEvent(ctx, ch, ToolCallDelta{
							ID:    acc.id,
							Name:  acc.name,
							Input: input,
						})
					}
				}

				// Gather usage info.
				inputTokens := 0
				outputTokens := 0
				if chunk.Usage != nil {
					inputTokens = chunk.Usage.PromptTokens
					outputTokens = chunk.Usage.CompletionTokens
				}

				sendEvent(ctx, ch, MessageEnd{
					InputTokens:  inputTokens,
					OutputTokens: outputTokens,
					FinishReason: reason,
					Model:        modelHint,
				})
			}
		}
	}
}

// sendEvent sends an event to the channel, respecting context cancellation.
func sendEvent(ctx context.Context, ch chan<- Event, ev Event) {
	select {
	case ch <- ev:
	case <-ctx.Done():
	}
}
