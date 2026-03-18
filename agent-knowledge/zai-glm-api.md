# Learning Guide: Z.ai (Zhipu AI) GLM API

**Generated**: 2026-03-18
**Sources**: 21 resources analyzed
**Depth**: medium

## Prerequisites

- HTTP API fundamentals (REST, JSON, Bearer auth)
- Server-Sent Events (SSE) basics
- Familiarity with the OpenAI Chat Completions API format (the GLM API is intentionally compatible)
- An API key from Z.ai (open.bigmodel.cn) or a coding plan subscription

## TL;DR

- The GLM API is **OpenAI-compatible** - same endpoint structure, message format, and tool calling schema. You can use the OpenAI SDK with a different `base_url`.
- **Base URL**: `https://open.bigmodel.cn/api/paas/v4/` (standard) or `https://api.z.ai/api/coding/paas/v4` (coding plan). Endpoint: `/chat/completions`.
- **Authentication**: Bearer token in the `Authorization` header. The API key format is `id.secret` - the official docs show JWT generation from this, but raw Bearer token also works on the coding plan endpoint.
- **Thinking/Reasoning**: The `thinking` parameter is an **object** `{"type": "enabled"}`, NOT a boolean. Reasoning output appears in `reasoning_content` (a separate field from `content`) in both streaming deltas and non-streaming responses.
- **Key SSE difference from OpenAI**: GLM adds `reasoning_content` to the delta object in streaming chunks, and can return `finish_reason: "network_error"` (retryable).

## Core Concepts

### 1. API Endpoints and Base URLs

Zhipu AI exposes two main API base URLs:

| Endpoint | Base URL | Notes |
|----------|----------|-------|
| Standard (open.bigmodel.cn) | `https://open.bigmodel.cn/api/paas/v4/` | General access, pay-per-token |
| Coding Plan (api.z.ai) | `https://api.z.ai/api/coding/paas/v4` | Subscription plan, included tokens |

The chat completions endpoint is `/chat/completions` appended to the base URL. This is identical to OpenAI's path structure.

**Source**: Python SDK (default `base_url`), Cairn project `config.go`, GLM cookbook `glm_openai_sdk.ipynb`

### 2. Authentication

The API key format is `id.secret` (two parts separated by a dot).

**Method 1 - Bearer Token** (simplest, used by the coding plan):
```
Authorization: Bearer your-api-key
```

**Method 2 - JWT** (used by the standard open.bigmodel.cn endpoint):
The official documentation generates a JWT from the API key:
1. Split key into `id` and `secret`
2. Create JWT payload: `{"api_key": id, "exp": expiration_timestamp, "timestamp": current_timestamp}`
3. Sign with HS256 using `secret`
4. Send as `Authorization: Bearer {jwt_token}`

**Source**: GLM cookbook `glm_http_request.ipynb`, Python SDK README

### 3. The Thinking/Reasoning Parameter - CRITICAL

This is the most commonly confused aspect of the GLM API.

**The `thinking` parameter is an OBJECT, not a boolean.**

Correct format:
```json
{
  "thinking": {"type": "enabled"}
}
```

Or to disable:
```json
{
  "thinking": {"type": "disabled"}
}
```

**Do NOT use:**
```json
{"thinking": true}
{"thinking": {"enabled": true}}
```

These incorrect formats will cause HTTP 400 errors.

In the Python SDK, the type hint is `thinking: object | None = None` - it accepts any object and passes it through to the API without transformation. The Go implementation in Cairn defines:

```go
type glmThinking struct {
    Type string `json:"type"` // "enabled" or "disabled"
}
```

**Reasoning output** appears in a **separate field** from the main content:

- Non-streaming: `response.choices[0].message.reasoning_content` (string, alongside `content`)
- Streaming: `chunk.choices[0].delta.reasoning_content` (string, alongside `delta.content`)

The reasoning content streams first (the model thinks), then the main content streams after.

**Source**: Cairn `glm.go` (lines 89-91, 208), Python SDK `completions.py`, `chat_completion.py`, `chat_completion_chunk.py`

### 4. Request Body Format

The request body is OpenAI-compatible with GLM-specific extensions:

```json
{
  "model": "glm-5-turbo",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Hello!"}
  ],
  "stream": true,
  "max_tokens": 8192,
  "temperature": 0.7,
  "top_p": 0.9,
  "stop": ["<stop>"],
  "tools": [],
  "tool_choice": "auto",
  "thinking": {"type": "enabled"},
  "response_format": {"type": "json_object"},
  "seed": 42
}
```

**GLM-specific fields not in standard OpenAI:**
- `thinking` - object `{"type": "enabled"}` (see section 3)
- `do_sample` - boolean, controls sampling behavior
- `sensitive_word_check` - content filtering request
- `meta` - character role-playing metadata (for CharGLM models)
- `request_id` - optional request tracking ID
- `user_id` - optional user identifier

**Temperature and top_p clamping**: The Python SDK auto-clamps these to (0.01, 0.99). Values at or below 0 are set to 0.01 (and `do_sample` is set to false). Values at or above 1 are set to 0.99.

**Source**: Python SDK `completions.py` source code, GLM cookbook examples

### 5. Available Models

| Model ID | Type | Context | Notes |
|----------|------|---------|-------|
| `glm-5-turbo` | Chat | 128K | Latest, default for Z.ai coding plan |
| `glm-4.7` | Chat | 128K | Previous generation, still available |
| `glm-4.5` | Chat | 128K | Supported via OpenAI SDK |
| `glm-4` | Chat | 128K | Stable, widely documented |
| `glm-4-flash` | Chat | 128K | Fast, lower cost |
| `glm-4v` | Vision | 128K | Multimodal (text + images) |
| `glm-4.1v-thinking-flash` | Vision+Thinking | 128K | Thinking built into model name |
| `charglm-3` | Character | - | Role-playing with `meta` parameter |
| `glm-z1-*` | Reasoning | 128K | Deep reasoning models (open-weight) |
| `cogview-4` | Image Gen | - | Image generation |
| `cogvideox-2` | Video Gen | - | Video generation |
| `embedding-3` | Embeddings | - | Text embeddings |

Note: Some models have thinking built into the model selection (e.g., `glm-4.1v-thinking-flash`) rather than requiring the `thinking` parameter.

**Source**: Python SDK README, Z.ai chat interface, Cairn `glm.go`, HuggingFace model cards

### 6. Tool/Function Calling

The tool calling format is **identical to OpenAI's**:

**Tool Definition (request):**
```json
{
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get weather for a city",
        "parameters": {
          "type": "object",
          "properties": {
            "location": {"type": "string", "description": "City name"},
            "unit": {"type": "string", "enum": ["c", "f"]}
          },
          "required": ["location"]
        }
      }
    }
  ],
  "tool_choice": "auto"
}
```

**Tool call in response:**
```json
{
  "choices": [{
    "message": {
      "role": "assistant",
      "tool_calls": [{
        "id": "call_abc123",
        "type": "function",
        "function": {
          "name": "get_weather",
          "arguments": "{\"location\": \"Beijing\", \"unit\": \"c\"}"
        }
      }]
    },
    "finish_reason": "tool_calls"
  }]
}
```

**Sending tool results back:**
```json
{
  "role": "tool",
  "content": "{\"temperature\": 22, \"condition\": \"sunny\"}",
  "tool_call_id": "call_abc123"
}
```

**Key detail**: Tool call IDs and order must match exactly with the model's tool calls. The `finish_reason` is `"tool_calls"` (not `"tool_call"` singular, matching OpenAI). GLM supports parallel tool calls - multiple tool_calls can appear in one response.

**Additional GLM-specific tool**: `web_search` type:
```json
{
  "tools": [{"type": "web_search", "web_search": {"search_query": "...", "search_result": true}}]
}
```

**Source**: GLM cookbook `glm_function_call.ipynb`, `glm_multi_functions_call.ipynb`, Cairn `glm.go`

### 7. Streaming SSE Format

GLM uses standard SSE with `data:` lines, identical to OpenAI's format with one critical addition.

**SSE frame format:**
```
data: {"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{"content":"Hello"}}]}

data: {"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{"content":" world"}}]}

data: {"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}

data: [DONE]
```

**GLM-specific streaming fields not in standard OpenAI:**

1. **`reasoning_content` in delta** - streams thinking content before main content:
```
data: {"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{"reasoning_content":"Let me think"}}]}
data: {"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{"reasoning_content":"... about this"}}]}
data: {"id":"1","model":"glm-5-turbo","choices":[{"index":0,"delta":{"content":"The answer is 42"}}]}
```

2. **`finish_reason: "network_error"`** - GLM-specific, indicates a retryable server-side error. This does NOT exist in OpenAI.

3. **Usage in streaming** - GLM includes `usage` in the final chunk (same as OpenAI's `stream_options: {"include_usage": true}`, but GLM does it by default).

**Tool call streaming** - arguments come as fragments that must be concatenated:
```
data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"readFile","arguments":""}}]}}]}
data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":"}}]}}]}
data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"test.go\"}"}}]}}]}
data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}
```

**Source**: Cairn `glm.go` (processStream), `glm_test.go`, Python SDK `chat_completion_chunk.py`

### 8. Non-Streaming Response Format

```json
{
  "id": "response-id",
  "model": "glm-5-turbo",
  "choices": [{
    "index": 0,
    "message": {
      "role": "assistant",
      "content": "The answer is 42",
      "reasoning_content": "Let me think about this... The user is asking about...",
      "tool_calls": null
    },
    "finish_reason": "stop"
  }],
  "usage": {
    "prompt_tokens": 20,
    "completion_tokens": 10,
    "total_tokens": 30
  }
}
```

The `reasoning_content` field is always a string (or null), separate from `content`.

**Source**: Python SDK `chat_completion.py`, GLM cookbook `glm_http_request.ipynb`

### 9. Error Handling

**HTTP Error Codes:**

| Code | Meaning | Retryable | Notes |
|------|---------|-----------|-------|
| 400 | Bad Request | No | Invalid parameters (wrong thinking format, bad tool schema) |
| 401 | Unauthorized | No | Invalid API key or expired JWT |
| 429 | Rate Limited | Yes | Too many requests, use exponential backoff |
| 500 | Server Error | Yes | Internal error |
| 502/503 | Service Unavailable | Yes | Temporary outage |

**Error response format:**
```json
{"error": {"message": "rate limited"}}
```

**Common causes of HTTP 400:**
1. **Wrong `thinking` format** - using `true` instead of `{"type": "enabled"}`
2. **Temperature out of range** - must be in (0, 1), not 0 or 1 exactly
3. **Invalid tool schema** - malformed JSON Schema in tool parameters
4. **Missing required fields** - `model` and `messages` are required
5. **Invalid message structure** - wrong role names, missing content

**Stream-level errors:**
- `finish_reason: "network_error"` - retryable, auto-retry then fallback
- Connection drops mid-stream - handle by tracking accumulated content

**Python SDK error types:**
- `APIStatusError` - base for HTTP errors (has `.status_code`)
- `APIAuthenticationError` - 401 errors
- `APIReachLimitError` - rate limit errors
- `APIInternalError` - server errors
- `APITimeoutError` - request timeouts
- `APIResponseValidationError` - response schema mismatch

**Source**: Cairn `glm.go`, `glm_test.go`, Python SDK `_errors.py`

### 10. OpenAI SDK Compatibility

You can use the standard OpenAI Python SDK with GLM:

```python
from openai import OpenAI

client = OpenAI(
    api_key="your-zhipu-api-key",
    base_url="https://open.bigmodel.cn/api/paas/v4/"
)

response = client.chat.completions.create(
    model="glm-4.5",
    messages=[{"role": "user", "content": "Hello!"}],
    top_p=0.7,
    temperature=0.9,
    stream=False,
    max_tokens=2000,
)
```

**Compatibility notes:**
- Tool calling works identically: "Other examples (such as tool calls) can also be called in the same way as OpenAI" (official cookbook)
- Image generation works via `client.images.generate()` with `cogview-4`
- Embeddings work via `client.embeddings.create()` with `embedding-3`
- The `thinking` parameter is GLM-specific and must be passed via `extra_body` when using the OpenAI SDK
- The `reasoning_content` field in responses is not parsed by the OpenAI SDK - you need to access raw response data

**Source**: GLM cookbook `glm_openai_sdk.ipynb`

## Code Examples

### Basic Chat (Python - Native SDK)

```python
from zhipuai import ZhipuAI

client = ZhipuAI(api_key="your-key")  # or set ZHIPUAI_API_KEY env var

response = client.chat.completions.create(
    model="glm-5-turbo",
    messages=[
        {"role": "system", "content": "You are a helpful coding assistant."},
        {"role": "user", "content": "Explain recursion."},
    ],
    temperature=0.7,
    max_tokens=4096,
    thinking={"type": "enabled"},
)

# Access reasoning and content separately
message = response.choices[0].message
print("Reasoning:", message.reasoning_content)
print("Answer:", message.content)
```

### Streaming with Reasoning (Python)

```python
response = client.chat.completions.create(
    model="glm-5-turbo",
    messages=[{"role": "user", "content": "Solve: what is 17 * 23?"}],
    stream=True,
    thinking={"type": "enabled"},
)

for chunk in response:
    delta = chunk.choices[0].delta
    if delta.reasoning_content:
        print(f"[thinking] {delta.reasoning_content}", end="")
    if delta.content:
        print(delta.content, end="")
```

### Raw HTTP Request (curl)

```bash
curl -X POST https://api.z.ai/api/coding/paas/v4/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Accept: text/event-stream" \
  -d '{
    "model": "glm-5-turbo",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": true,
    "max_tokens": 8192,
    "temperature": 0.7,
    "thinking": {"type": "enabled"}
  }'
```

### Tool Calling with Multi-turn (Python)

```python
import json

tools = [{
    "type": "function",
    "function": {
        "name": "get_weather",
        "description": "Get weather for a location",
        "parameters": {
            "type": "object",
            "properties": {
                "location": {"type": "string"},
                "unit": {"type": "string", "enum": ["c", "f"]}
            },
            "required": ["location"]
        }
    }
}]

messages = [{"role": "user", "content": "What is the weather in Tokyo?"}]

# First call - model decides to use tool
response = client.chat.completions.create(
    model="glm-5-turbo",
    messages=messages,
    tools=tools,
    tool_choice="auto",
)

tool_call = response.choices[0].message.tool_calls[0]
messages.append(response.choices[0].message.model_dump())

# Execute the tool and send result back
result = {"temperature": 18, "condition": "cloudy"}
messages.append({
    "role": "tool",
    "content": json.dumps(result),
    "tool_call_id": tool_call.id,
})

# Second call - model synthesizes final answer
final = client.chat.completions.create(
    model="glm-5-turbo",
    messages=messages,
    tools=tools,
)
print(final.choices[0].message.content)
```

### Go Implementation (Cairn pattern)

```go
// Request body construction
glmReq := glmRequest{
    Model:     "glm-5-turbo",
    Stream:    true,
    MaxTokens: 8192,
    Thinking:  &glmThinking{Type: "enabled"},
    Messages: []glmMessage{
        {Role: "user", Content: json.RawMessage(`"Hello"`)},
    },
}

// HTTP request
req, _ := http.NewRequest("POST", baseURL+"/chat/completions", body)
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer "+apiKey)
req.Header.Set("Accept", "text/event-stream")

// Parse SSE stream - look for both content and reasoning_content
// delta.ReasoningContent streams first, then delta.Content
```

## Common Pitfalls

| Pitfall | Why It Happens | How to Avoid |
|---------|---------------|--------------|
| `thinking: true` causes 400 | API expects object, not boolean | Use `{"type": "enabled"}` |
| `thinking: {"enabled": true}` causes 400 | Wrong object shape | Use `{"type": "enabled"}` exactly |
| Temperature set to 0.0 causes error | GLM rejects exact 0.0 | Use 0.01 minimum (SDK auto-clamps) |
| Temperature set to 1.0 causes error | GLM rejects exact 1.0 | Use 0.99 maximum (SDK auto-clamps) |
| Missing reasoning_content in responses | Not checking the right field | It is a sibling of `content`, not nested inside it |
| Tool call arguments incomplete | Streaming fragments not concatenated | Accumulate `function.arguments` across chunks by tool call index |
| `finish_reason: "network_error"` treated as fatal | GLM-specific retryable error | Implement retry logic for this specific finish reason |
| JWT expired | Standard endpoint requires JWT auth | Regenerate JWT before expiration or use coding plan endpoint |
| OpenAI SDK misses reasoning_content | SDK does not parse GLM-specific fields | Use native zhipuai SDK or access raw response data |
| Content appears in wrong order | Reasoning streams before content | Handle reasoning_content delta before content delta in UI |

## Best Practices

1. **Use the object format for thinking**: Always `{"type": "enabled"}` or `{"type": "disabled"}`. Never boolean. (Source: Cairn glm.go, Python SDK)
2. **Clamp temperature and top_p**: Keep both in (0.01, 0.99) range to avoid API errors. (Source: Python SDK completions.py)
3. **Handle reasoning_content separately**: Display it in a collapsible "thinking" section in your UI, not mixed with the main response. (Source: Cairn frontend)
4. **Implement retry for network_error**: This GLM-specific finish_reason is retryable - back off and retry, then fall back to another model. (Source: Cairn glm.go, design doc)
5. **Accumulate tool call fragments by index**: Tool call arguments stream as fragments indexed by `tool_calls[].index`. Concatenate `function.arguments` strings until `finish_reason` arrives. (Source: Cairn glm_test.go)
6. **Use fallback chains**: `glm-5-turbo` -> `glm-4.7` -> error. GLM can return transient errors. (Source: Cairn LLM client design doc)
7. **Set Accept header for SSE**: Include `Accept: text/event-stream` in streaming requests. (Source: Cairn glm.go)
8. **Use [DONE] sentinel**: The stream terminates with `data: [DONE]` - same as OpenAI. (Source: Cairn sse.go)
9. **Track usage from final chunk**: Token usage appears in the last chunk before `[DONE]`, within the `usage` field. (Source: Cairn glm_test.go)
10. **For OpenAI SDK usage, pass GLM-specific params via extra_body**: The OpenAI client does not have a `thinking` parameter - use `extra_body={"thinking": {"type": "enabled"}}`. (Source: Python SDK docs)

## Key Differences: GLM API vs OpenAI API

| Feature | GLM API | OpenAI API |
|---------|---------|------------|
| Base URL | `open.bigmodel.cn/api/paas/v4/` | `api.openai.com/v1/` |
| Auth (standard) | JWT from id.secret key | Bearer API key |
| Auth (coding plan) | Bearer id.secret key | Bearer API key |
| Thinking parameter | `{"type": "enabled"}` object | Not applicable (use `reasoning_effort` for o-series) |
| Reasoning in response | `reasoning_content` field (sibling of `content`) | Not in standard models |
| Streaming reasoning | `delta.reasoning_content` | Not in standard models |
| Temperature range | (0.01, 0.99) strictly | [0, 2] |
| `finish_reason: "network_error"` | GLM-specific, retryable | Does not exist |
| Tool calling format | Identical | Reference format |
| SSE format | `data: {json}\n\n` + `[DONE]` | Same |
| Usage in stream | Included by default in final chunk | Requires `stream_options` |
| `sensitive_word_check` | GLM-specific content filter | Not applicable |
| `do_sample` | Explicit boolean | Not applicable |
| `meta` parameter | For CharGLM role-playing | Not applicable |
| `request_id` / `user_id` | Built-in request tracking | `user` field only |
| Web search tool | `{"type": "web_search", ...}` | Not built-in |
| Audio in chunks | `delta.audio` supported | Separate API |

## Further Reading

| Resource | Type | Why Recommended |
|----------|------|-----------------|
| [Zhipu AI Open Platform](https://open.bigmodel.cn/) | Official Portal | Model access, API keys, docs |
| [Z.ai Chat](https://chat.z.ai/) | Product | Try GLM-5 and GLM-4.7 live |
| [Python SDK (GitHub)](https://github.com/MetaGLM/zhipuai-sdk-python-v4) | SDK Source | Complete API implementation |
| [GLM Cookbook (GitHub)](https://github.com/MetaGLM/glm-cookbook) | Tutorials | Function calling, HTTP, OpenAI SDK examples |
| [GLM-4 Models (GitHub)](https://github.com/THUDM/GLM-4) | Open Source | Open-weight models, chat templates |
| [GLM-4-9B on HuggingFace](https://huggingface.co/THUDM/glm-4-9b-chat) | Model Card | Benchmarks, local deployment |
| [PyPI zhipuai](https://pypi.org/project/zhipuai/) | Package | SDK installation and version history |
| [Cairn LLM Client](https://github.com/avifenesh/cairn/blob/main/internal/llm/glm.go) | Implementation | Production Go implementation with full SSE handling |

---

*This guide was synthesized from 21 sources including official SDKs, open-source implementations, and API documentation. See `resources/zai-glm-api-sources.json` for full source list with quality scores.*
