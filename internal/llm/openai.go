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
	defaultOpenAIBaseURL   = "https://api.openai.com/v1"
	defaultOpenAIModel     = "gpt-4o"
	defaultOpenAIMaxTokens = 4096
)

// OpenAIProvider implements Provider for any OpenAI-compatible API.
// Works with: OpenAI, Ollama, vLLM, LiteLLM, Together, Groq, etc.
type OpenAIProvider struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewOpenAIProvider creates a provider for any OpenAI-compatible endpoint.
// If baseURL is empty, defaults to https://api.openai.com/v1.
// If model is empty, defaults to "gpt-4o".
func NewOpenAIProvider(apiKey, baseURL, model string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	if model == "" {
		model = defaultOpenAIModel
	}
	return &OpenAIProvider{
		apiKey:     apiKey,
		baseURL:    strings.TrimRight(baseURL, "/"),
		model:      model,
		httpClient: &http.Client{},
		logger:     slog.Default(),
	}
}

func (p *OpenAIProvider) ID() string { return "openai" }

func (p *OpenAIProvider) Models() []ModelInfo {
	return []ModelInfo{
		{ID: p.model, DisplayName: p.model, MaxTokens: 128000},
	}
}

// Stream sends a streaming chat completion request and returns events on a channel.
func (p *OpenAIProvider) Stream(ctx context.Context, req *Request) (<-chan Event, error) {
	body, err := p.buildRequestBody(req)
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %w", err)
	}

	endpoint := p.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: http request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		retryable := resp.StatusCode == 429 || resp.StatusCode >= 500
		if retryable {
			return nil, &retryableError{
				statusCode: resp.StatusCode,
				message:    string(errBody),
			}
		}
		return nil, fmt.Errorf("openai: http %d: %s", resp.StatusCode, string(errBody))
	}

	ch := make(chan Event, 32)
	go p.processStream(ctx, resp.Body, ch, req.Model)
	return ch, nil
}

type retryableError struct {
	statusCode int
	message    string
}

func (e *retryableError) Error() string {
	return fmt.Sprintf("http %d: %s", e.statusCode, e.message)
}

// buildRequestBody marshals the request into OpenAI's JSON format.
// This is identical to GLM's format since GLM is OpenAI-compatible.
func (p *OpenAIProvider) buildRequestBody(req *Request) ([]byte, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultOpenAIMaxTokens
	}

	oaiReq := oaiRequest{
		Model:     model,
		Stream:    true,
		MaxTokens: maxTokens,
		Stop:      req.Stop,
	}

	if req.Temperature != nil {
		oaiReq.Temperature = req.Temperature
	}

	// Build messages.
	if req.System != "" {
		sysContent, _ := json.Marshal(req.System)
		oaiReq.Messages = append(oaiReq.Messages, oaiMessage{
			Role:    "system",
			Content: sysContent,
		})
	}

	for _, m := range req.Messages {
		oaiReq.Messages = append(oaiReq.Messages, convertToOAIMessages(m)...)
	}

	// Convert tools.
	for _, t := range req.Tools {
		oaiReq.Tools = append(oaiReq.Tools, oaiToolDef{
			Type: "function",
			Function: oaiToolDefFunc{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}

	return json.Marshal(oaiReq)
}

// OpenAI request types (shared format with GLM since GLM is OpenAI-compatible).
type oaiRequest struct {
	Model       string         `json:"model"`
	Messages    []oaiMessage   `json:"messages"`
	Stream      bool           `json:"stream"`
	MaxTokens   int            `json:"max_tokens,omitempty"`
	Temperature *float64       `json:"temperature,omitempty"`
	Stop        []string       `json:"stop,omitempty"`
	Tools       []oaiToolDef   `json:"tools,omitempty"`
}

type oaiMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolCalls  []oaiToolCall   `json:"tool_calls,omitempty"`
}

type oaiToolDef struct {
	Type     string          `json:"type"`
	Function oaiToolDefFunc  `json:"function"`
}

type oaiToolDefFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type oaiToolCall struct {
	Index    int             `json:"index"`
	ID       string          `json:"id,omitempty"`
	Type     string          `json:"type,omitempty"`
	Function oaiToolCallFunc `json:"function"`
}

type oaiToolCallFunc struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type oaiChunk struct {
	ID      string      `json:"id"`
	Model   string      `json:"model"`
	Choices []oaiChoice `json:"choices"`
	Usage   *oaiUsage   `json:"usage,omitempty"`
}

type oaiChoice struct {
	Index        int      `json:"index"`
	Delta        oaiDelta `json:"delta"`
	FinishReason *string  `json:"finish_reason,omitempty"`
}

type oaiDelta struct {
	Role      string        `json:"role,omitempty"`
	Content   *string       `json:"content,omitempty"`
	ToolCalls []oaiToolCall `json:"tool_calls,omitempty"`
}

type oaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

func convertToOAIMessages(m Message) []oaiMessage {
	switch m.Role {
	case RoleTool:
		var msgs []oaiMessage
		for _, block := range m.Content {
			if tr, ok := block.(ToolResultBlock); ok {
				content, _ := json.Marshal(tr.Content)
				msgs = append(msgs, oaiMessage{
					Role:       "tool",
					Content:    content,
					ToolCallID: tr.ToolUseID,
				})
			}
		}
		return msgs

	case RoleAssistant:
		msg := oaiMessage{Role: "assistant"}
		var textParts []string
		var toolCalls []oaiToolCall

		for _, block := range m.Content {
			switch b := block.(type) {
			case TextBlock:
				textParts = append(textParts, b.Text)
			case ToolUseBlock:
				toolCalls = append(toolCalls, oaiToolCall{
					Index: len(toolCalls),
					ID:    b.ID,
					Type:  "function",
					Function: oaiToolCallFunc{
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
		return []oaiMessage{msg}

	default:
		msg := oaiMessage{Role: string(m.Role)}
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
		return []oaiMessage{msg}
	}
}

// processStream reads SSE events and emits Event values on the channel.
func (p *OpenAIProvider) processStream(ctx context.Context, body io.ReadCloser, ch chan<- Event, modelHint string) {
	defer close(ch)
	defer body.Close()

	type toolCallAcc struct {
		id   string
		name string
		args strings.Builder
	}
	toolCalls := make(map[int]*toolCallAcc)

	sseEvents := ParseSSE(ctx, body)

	for sse := range sseEvents {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if sse.Err != nil {
			sendEvent(ctx, ch, StreamError{Err: sse.Err, Retryable: false})
			return
		}

		var chunk oaiChunk
		if err := json.Unmarshal([]byte(sse.Data), &chunk); err != nil {
			p.logger.Warn("openai: failed to parse chunk", "error", err, "data", sse.Data)
			continue
		}

		if modelHint == "" && chunk.Model != "" {
			modelHint = chunk.Model
		}

		for _, choice := range chunk.Choices {
			delta := choice.Delta

			if delta.Content != nil && *delta.Content != "" {
				sendEvent(ctx, ch, TextDelta{Text: *delta.Content})
			}

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

			if choice.FinishReason != nil {
				// Flush tool calls.
				for idx := 0; idx < len(toolCalls); idx++ {
					if acc, ok := toolCalls[idx]; ok {
						var input json.RawMessage
						if args := acc.args.String(); args != "" {
							input = json.RawMessage(args)
						}
						sendEvent(ctx, ch, ToolCallDelta{
							ID:    acc.id,
							Name:  acc.name,
							Input: input,
						})
					}
				}

				inputTokens, outputTokens := 0, 0
				if chunk.Usage != nil {
					inputTokens = chunk.Usage.PromptTokens
					outputTokens = chunk.Usage.CompletionTokens
				}

				sendEvent(ctx, ch, MessageEnd{
					InputTokens:  inputTokens,
					OutputTokens: outputTokens,
					FinishReason: *choice.FinishReason,
					Model:        modelHint,
				})
			}
		}
	}
}
