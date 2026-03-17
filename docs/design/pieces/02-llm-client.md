# Piece 2: LLM Client

> Multi-provider streaming with retry, fallback, budget tracking, and reasoning support.

## Purpose

The LLM client is the mouth and ears of the agent. It sends structured messages + tools to language models and yields streaming events back. Must handle: multiple providers, network errors, rate limits, budget caps, reasoning traces, and tool call assembly.

## Interface

```go
// Provider-agnostic streaming interface
// NOTE: Implementation uses channel-based streaming (<-chan Event, error)
// instead of iter.Seq2 - channels are more practical for concurrent SSE fan-out.
type Provider interface {
    ID() string
    Stream(ctx context.Context, req *Request) (<-chan Event, error)
    Models() []ModelInfo
}

type Request struct {
    Model       string
    Messages    []Message
    System      string
    Tools       []ToolDef
    MaxTokens   int
    Temperature *float64
    Stop        []string
}

// Streaming events — union type via interface
type Event interface { eventMarker() }

type TextDelta struct { Text string }
type ReasoningDelta struct { Text string }
type ToolCall struct { ID, Name string; Input json.RawMessage }
type MessageEnd struct { InputTokens, OutputTokens int; FinishReason string }
type StreamError struct { Err error; Retryable bool }

// Message types
type Message struct {
    Role    Role // user, assistant, system, tool
    Content []ContentBlock
}

type ContentBlock interface { blockMarker() }
type TextBlock struct { Text string }
type ToolUseBlock struct { ID, Name string; Input json.RawMessage }
type ToolResultBlock struct { ToolUseID string; Content string; IsError bool }
type ReasoningBlock struct { Text string }
```

## Provider Abstraction

```go
// Each provider implements this (see above)

// Registry — implemented in registry.go
type Registry struct {
    providers map[string]Provider
    models    map[string]string  // modelID → providerID
    fallbacks map[string]string  // model → fallback model
}

func NewRegistry(logger *slog.Logger) *Registry
func (r *Registry) Register(p Provider)
func (r *Registry) Provider(id string) (Provider, bool)
func (r *Registry) Resolve(modelID string) (Provider, string, error)
func (r *Registry) SetFallback(modelID, fallbackModelID string)
func (r *Registry) WithRetryAndFallback(modelID string, config RetryConfig) (Provider, string, error)
func (r *Registry) RegisterFromConfig(cfg ProviderConfig) error
```

## Providers to Support (Phase 1)

| Provider | SDK/Approach | Priority |
|----------|-------------|----------|
| **GLM (Z.ai)** | Raw HTTP + SSE parsing (current approach, proven) | P0 |
| **OpenAI-compatible** | go-openai SDK | P1 |
| **Anthropic** | anthropic-sdk-go | P1 |
| **Ollama** | HTTP API (OpenAI-compatible) | P2 |
| **Google** | google.golang.org/genai | P2 |

## Retry & Fallback

```go
type RetryConfig struct {
    MaxRetries     int           // default: 3
    BaseBackoff    time.Duration // default: 1s
    MaxBackoff     time.Duration // default: 30s
    JitterFraction float64       // default: 0.2
    RetryableStatus []int        // 429, 500, 502, 503
}

// Fallback chain: glm-5-turbo → glm-4.7 → error
// On network_error finish_reason (GLM-specific): auto-retry then fallback
```

## Budget Tracking

```go
type Budget struct {
    DailyLimit   float64
    WeeklyLimit  float64
    DailySpent   float64
    WeeklySpent  float64
    LastResetDay int
    mu           sync.Mutex
}

func (b *Budget) CanAfford(model string, estInputTokens int) bool
func (b *Budget) Record(model string, inputTokens, outputTokens int)
func (b *Budget) MidStreamCheck(model string, estOutputChars int) bool
```

## Subphases

| # | Subphase | Description | Depends On |
|---|----------|-------------|------------|
| 2.1 | Types & interfaces | Message, Event, Request, Provider | Nothing |
| 2.2 | SSE parser | Parse `data: {json}\n\n` streams into Event iterator | 2.1 |
| 2.3 | GLM provider | Z.ai streaming with reasoning_content + network_error handling | 2.1, 2.2 |
| 2.4 | OpenAI-compatible provider | Raw HTTP + SSE (same approach as GLM, no SDK dep) | 2.1 |
| 2.5 | Retry + fallback wrapper | Wraps any Provider with retry/backoff/fallback chain | 2.1 |
| 2.6 | Budget tracker | Daily/weekly spend tracking with mid-stream abort | 2.1 |
| 2.7 | Provider registry | Multi-provider resolution, config-driven | 2.1, 2.3, 2.4 |
| 2.8 | Tests | Streaming tests with mock SSE server | All |

## Tasks

### 2.1 Types & interfaces
- [x] Define Message, ContentBlock variants, Role enum
- [x] Define Event variants (TextDelta, ReasoningDelta, ToolCall, MessageEnd, StreamError)
- [x] Define Request struct with all LLM parameters
- [x] Define Provider interface and Registry

### 2.2 SSE parser
- [x] Implement `ParseSSEStream(reader io.Reader) iter.Seq2[string, error]` — yields `data:` lines
- [x] Handle `[DONE]` sentinel
- [x] Handle connection drops with context cancellation

### 2.3 GLM provider
- [x] Implement `glm.Provider` with Z.ai endpoint
- [x] Handle `reasoning_content` field → ReasoningDelta events
- [x] Handle `network_error` finish_reason → auto-retry
- [x] Handle `thinking` parameter (enabled by default)
- [x] Tool call assembly from streamed fragments
- [x] Auth: `id.secret` Bearer token

### 2.4 OpenAI-compatible provider
- [x] Raw HTTP + SSE provider for OpenAI-compatible APIs (no SDK dependency)
- [x] Map OpenAI streaming events to our Event types
- [x] Support custom base URL (for Ollama, local models)

### 2.5 Retry + fallback wrapper
- [x] `WithRetry(provider Provider, config RetryConfig) Provider`
- [x] Exponential backoff with jitter
- [x] Fallback to secondary model on persistent failure
- [x] Log retry attempts

### 2.6 Budget tracker
- [x] Implement Budget struct with thread-safe spend tracking
- [x] Cost-per-million lookup table by model
- [x] Mid-stream budget check (abort if over limit)
- [x] Daily/weekly reset logic

### 2.7 Provider registry
- [x] Config-driven provider initialization
- [x] Model → provider resolution
- [x] Hot-reload support (re-read config without restart)

### 2.8 Tests
- [x] Mock SSE server that streams events with delays
- [x] Test retry behavior on 429/500
- [x] Test fallback chain activation
- [x] Test budget enforcement mid-stream
- [x] Test tool call assembly from fragmented chunks
