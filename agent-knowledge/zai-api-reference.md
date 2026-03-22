# Z.ai API Reference (Comprehensive)

Source: https://docs.z.ai (fetched 2026-03-19)

## Base URLs

| Purpose | URL |
|---------|-----|
| Chat completions (coding plan) | `https://api.z.ai/api/coding/paas/v4` |
| Chat completions (pay-as-you-go) | `https://api.z.ai/api/paas/v4` |
| Claude Code proxy | `https://api.z.ai/api/anthropic` |
| MCP services | `https://api.z.ai/api/mcp` |
| Web search REST | `https://api.z.ai/api/paas/v4/web_search` |
| Web reader REST | `https://api.z.ai/api/paas/v4/reader` |

Auth: `Authorization: Bearer <api_key>` on all endpoints.

---

## 1. Chat Completions

**POST** `{base}/chat/completions`

### Models

| Model | Context | Max Output | Input $/1M | Output $/1M | Notes |
|-------|---------|------------|------------|-------------|-------|
| glm-5 | ? | ? | $1.00 | $3.20 | Premium |
| glm-5-turbo | 200K | 128K | $1.20 | $4.00 | Best for tool calling, ClawBench optimized |
| glm-5-code | ? | ? | $1.20 | $5.00 | Code-specific |
| glm-4.7 | ? | ? | $0.60 | $2.20 | Standard |
| glm-4.7-flash | ? | ? | Free | Free | Free tier |
| glm-4.7-flashx | ? | ? | $0.07 | $0.40 | Budget |
| glm-4.6 | ? | ? | $0.60 | $2.20 | Standard |
| glm-4.5 | ? | ? | $0.60 | $2.20 | Standard |
| glm-4.5-air | ? | ? | $0.20 | $1.10 | Budget, good for title gen |
| glm-4.5-airx | ? | ? | $1.10 | $4.50 | Enhanced air |
| glm-4.5-x | ? | ? | $2.20 | $8.90 | Premium |
| glm-4.5-flash | ? | ? | Free | Free | Free tier |
| glm-4-32b-0414-128k | 128K | ? | ? | ? | Open-weight |

Vision: glm-4.6v, glm-4.6v-flash, glm-4.6v-flashx, glm-4.5v, autoglm-phone-multilingual

### Coding Plan Models
Only these work on the coding plan subscription:
- **glm-5-turbo** (default, best)
- **glm-5**
- **glm-4.7**
- **glm-4.6**
- **glm-4.5**
- **glm-4.5-air** (cheapest, no thinking - good for title gen)

### Request Parameters

```json
{
  "model": "glm-5-turbo",
  "messages": [...],
  "stream": true,
  "max_tokens": 32768,
  "temperature": 0.7,
  "top_p": 0.7,
  "do_sample": true,
  "stop": ["<stop>"],
  "thinking": {"type": "enabled", "clear_thinking": true},
  "tools": [...],
  "tool_choice": "auto",
  "response_format": {"type": "text"},
  "request_id": "optional-uuid",
  "user_id": "optional-user-id"
}
```

### Thinking Parameter

```json
{"thinking": {"type": "enabled"}}              // enable deep thinking
{"thinking": {"type": "disabled"}}             // disable (direct answer)
{"thinking": {"type": "enabled", "clear_thinking": false}}  // preserve prior reasoning
```

- **MUST** be an object with `type` field, NOT a boolean (boolean causes HTTP 400)
- Response includes `reasoning_content` field alongside `content`
- Thinking consumes tokens from `max_tokens` budget
- GLM-5/4.7: thinking enabled by default
- GLM-4.5+: model auto-determines when to think

#### Thinking Modes
| Mode | Description | Default |
|------|-------------|---------|
| Default | Auto-activates on GLM-5, GLM-4.7 | Enabled |
| Interleaved | Reasons between tool calls | Enabled since GLM-4.5 |
| Preserved | Retains `reasoning_content` across turns | Disabled (API), enabled on coding plan |
| Turn-level | Control per-request | Available GLM-4.7+ |

**Important**: Return `reasoning_content` unmodified to API for cache efficiency.

### Temperature Defaults
- GLM-5, 4.7, 4.6, 4.5: 1.0 (text) / 0.8 (vision)
- GLM-4-32B-0414-128K: 0.75
- autoglm-phone-multilingual: 0.0

### Tool Calling

```json
{
  "tools": [{
    "type": "function",
    "function": {
      "name": "get_weather",
      "description": "Get current weather",
      "parameters": {
        "type": "object",
        "properties": {"location": {"type": "string"}},
        "required": ["location"]
      }
    }
  }],
  "tool_choice": "auto"
}
```

- `tool_choice` only supports `"auto"` (no forced calling)
- Max 128 tools per request
- `tool_stream`: streaming for function calls (GLM-4.6 only)
- Response `tool_calls[].function.arguments` is a JSON string (needs parsing)

### Built-in Web Search (in chat completion)

```json
{
  "tools": [{
    "type": "web_search",
    "web_search": {
      "enable": "True",
      "search_engine": "search-prime",
      "search_result": "True",
      "search_prompt": "custom instruction",
      "count": "5",
      "search_domain_filter": "example.com",
      "search_recency_filter": "noLimit",
      "content_size": "high"
    }
  }]
}
```

Response includes `web_search` array with citations. NOT currently used in Cairn.

### Streaming (SSE)
- `data:` prefixed JSON chunks
- `data: [DONE]` terminates stream
- `finish_reason`: `stop`, `tool_calls`, `length`, `sensitive`, `network_error`
- `network_error` = known GLM issue, retryable

### Response Schema

```json
{
  "id": "task_id",
  "choices": [{
    "index": 0,
    "message": {
      "role": "assistant",
      "content": "text",
      "reasoning_content": "thinking (if enabled)",
      "tool_calls": [{"id": "...", "type": "function", "function": {"name": "...", "arguments": "..."}}]
    },
    "finish_reason": "stop"
  }],
  "usage": {
    "prompt_tokens": 100,
    "completion_tokens": 200,
    "total_tokens": 300,
    "prompt_tokens_details": {"cached_tokens": 50}
  }
}
```

---

## 2. Web Search REST API

**POST** `https://api.z.ai/api/paas/v4/web_search`

### Request

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| search_engine | string | Yes | - | `"search-prime"` |
| search_query | string | Yes | - | Search query text |
| count | int | No | 10 | Results count (1-50) |
| search_domain_filter | string | No | - | Limit to domain |
| search_recency_filter | string | No | `"noLimit"` | `oneDay`, `oneWeek`, `oneMonth`, `oneYear`, `noLimit` |
| request_id | string | No | - | Custom request ID |
| user_id | string | No | - | End user ID (6-128 chars) |

Header: `Accept-Language: en-US,en` (optional)

### Response

```json
{
  "id": "task_id",
  "created": 1234567890,
  "search_result": [
    {
      "title": "Result Title",
      "content": "Summary text",
      "link": "https://example.com",
      "media": "Example.com",
      "icon": "https://example.com/favicon.ico",
      "refer": "1",
      "publish_date": "2024-01-15"
    }
  ]
}
```

**Pricing**: $0.01 per search call.

---

## 3. Web Reader REST API

**POST** `https://api.z.ai/api/paas/v4/reader`

### Request

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| url | string | Yes | - | Target webpage URL |
| timeout | int | No | 20 | Max seconds |
| no_cache | bool | No | false | Bypass cache |
| return_format | string | No | `"markdown"` | `"markdown"` or `"text"` |
| retain_images | bool | No | true | Include parsed images |
| no_gfm | bool | No | false | Disable GitHub Flavored Markdown |
| keep_img_data_url | bool | No | false | Preserve data: URLs |
| with_images_summary | bool | No | false | Add image descriptions |
| with_links_summary | bool | No | false | Add links compilation |

### Response

```json
{
  "id": "task_id",
  "created": 1234567890,
  "reader_result": {
    "content": "extracted markdown/text",
    "title": "Page Title",
    "description": "Meta description",
    "url": "original URL",
    "external": [...],
    "metadata": {...}
  }
}
```

---

## 4. MCP Services (Streamable HTTP)

All MCP endpoints use JSON-RPC 2.0 over HTTP with SSE responses.

### Protocol
1. **Initialize**: POST to endpoint with `method: "initialize"` → get `mcp-session-id` header
2. **Call tool**: POST with `method: "tools/call"`, include `mcp-session-id` header
3. **Response**: SSE format `id:N\nevent:message\ndata:{json}\n`
4. **Session expiry**: 404/401 → delete session, re-initialize

### 4a. Web Search MCP

- **Endpoint**: `https://api.z.ai/api/mcp/web_search_prime/mcp`
- **SSE fallback**: `https://api.z.ai/api/mcp/web_search_prime/sse`
- **Transport**: HTTP (remote, no local install)
- **Tool**: `web_search_prime` (docs say `webSearchPrime` but actual MCP tool name is snake_case)
- **Params**: (not documented — likely `search_query` string)
- **Returns**: page titles, URLs, summaries, site names, site icons

### 4b. Web Reader MCP

- **Endpoint**: `https://api.z.ai/api/mcp/web_reader/mcp`
- **SSE fallback**: `https://api.z.ai/api/mcp/web_reader/sse`
- **Transport**: HTTP (remote, no local install)
- **Tool**: `webReader`
- **Params**: `url` (string)
- **Returns**: page title, main content, metadata, list of links

### 4c. Zread MCP (Repository Intelligence)

- **Endpoint**: `https://api.z.ai/api/mcp/zread/mcp`
- **SSE fallback**: `https://api.z.ai/api/mcp/zread/sse`
- **Transport**: HTTP (remote, no local install)
- **Powered by**: zread.ai
- **Tools**:
  - `search_doc` — search repo docs, issues, PRs, contributors
    - Params: `query` (string), `repo_name` (string, optional, "owner/name")
  - `get_repo_structure` — directory tree
    - Params: `repo_name` (string), `dir_path` (string, optional)
  - `read_file` — read file content
    - Params: `repo_name` (string), `file_path` (string)
- **Note**: Only public/open-source repos supported. Check zread.ai for repo availability.

### 4d. Vision MCP (Local Subprocess - NOT HTTP)

- **Package**: `@z_ai/mcp-server` (npm, v0.1.2+)
- **Transport**: **stdio** (NOT HTTP like other MCP services)
- **Requires**: Node.js >= v22.0.0
- **Run**: `npx -y @z_ai/mcp-server`
- **Env vars**: `Z_AI_API_KEY`, `Z_AI_MODE=ZAI`
- **Tools**:
  - `ui_to_artifact` — UI screenshot → code/specs
  - `extract_text_from_screenshot` — OCR for code, terminals, docs
  - `diagnose_error_screenshot` — error analysis + fix recommendations
  - `understand_technical_diagram` — architecture/flow/UML diagrams
  - `analyze_data_visualization` — charts/dashboards
  - `ui_diff_check` — compare two UI screenshots
  - `image_analysis` — general image understanding
  - `video_analysis` — video inspection (<=8MB, MP4/MOV/M4V)

**Architecture difference**: Vision runs as a local subprocess communicating via stdio, not as a remote HTTP endpoint. It uses the GLM-4.6V model under the hood. Cannot be called via `callZaiMCP()` — needs a different subprocess-based integration.

---

## 5. Coding Plan Quotas

| Plan | Prompts/5hr | MCP calls/month | Price |
|------|-------------|-----------------|-------|
| Lite | ~80 | 100 | ? |
| Pro | ~400 | 1,000 | ? |
| Max | ~1,600 | 4,000 | ? |

- Weekly quota resets on 7-day cycles from purchase date
- GLM-5/GLM-5-Turbo consume 3x during peak hours, 2x off-peak
- **MCP calls are a SHARED pool**: web search + web reader + Zread all count together (Max=4000 total)
- Vision uses the "5-hour maximum prompt resource pool" (separate from web/zread count)
- When quota exhausts, system won't deduct balance — wait for cycle refresh

---

## 6. Error Codes

| Code | HTTP | Meaning |
|------|------|---------|
| 1000-1004 | 401 | Auth failures (missing, invalid, expired token) |
| 1110 | 429 | Account inactive |
| 1112 | 429 | Account locked |
| **1113** | **429** | **"Account in arrears" — insufficient balance/no resource package** |
| 1120-1121 | 429 | Access denied, irregular activity |
| 1210-1215 | 400 | Parameter/model errors |
| 1220-1222 | 400 | Permission/API availability |
| 1230-1234 | 500 | Processing/network errors |
| 1301 | 400 | Unsafe/sensitive content |
| 1302-1305 | 429 | Concurrency/frequency limits |
| 1308 | 429 | Usage limit reached (includes reset time) |
| 1309 | 429 | Subscription expired |
| 1310 | 429 | Weekly/monthly limit exhausted |

### Error 1113 Troubleshooting
Per FAQ, this typically means:
1. Wrong API endpoint (coding plan must use `/api/coding/paas/v4` not `/api/paas/v4`)
2. Wrong model (only GLM-4.5 through GLM-5-Turbo on coding plan)
3. MCP quota depleted for the month
4. Subscription not active

**IMPORTANT**: The ZAI_API_KEY for MCP tools may be different from the GLM_API_KEY for chat. MCP endpoints use the **MCP base URL** (`/api/mcp/...`), not the coding base URL. The REST web search API uses `/api/paas/v4/web_search` which is the pay-as-you-go endpoint, so it requires a key with balance, not just a coding plan subscription.

---

## 7. Other APIs (not yet used in Cairn)

| API | Endpoint | Purpose |
|-----|----------|---------|
| Image gen | `/api/paas/v4/images/generations` | GLM-Image, CogView-4 |
| Video gen | `/api/paas/v4/videos/generations` | CogVideoX-3, Vidu |
| Audio transcription | `/api/paas/v4/audio/transcriptions` | GLM-ASR-2512 |
| OCR/Layout | `/api/paas/v4/layout-parsing` | GLM-OCR document parsing |
| Tokenizer | `/api/paas/v4/tokenizer` | Token counting |
| Translation | `/api/paas/v4/agents` | Translation agent |
| Slides | `/api/paas/v4/agents` | Slide/poster agent |

---

## 8. Cairn Implementation Status

### What We Have
| Tool | Backend | Method | Status |
|------|---------|--------|--------|
| cairn.webSearch | Z.ai REST API | `callZaiWebSearchREST` | Works, but error 1113 (quota) |
| cairn.webFetch | Z.ai MCP web_reader | `callZaiMCP("web_reader", "webReader")` | Works |
| cairn.searchDoc | Z.ai MCP zread | `callZaiMCP("zread", "search_doc")` | Works |
| cairn.repoStructure | Z.ai MCP zread | `callZaiMCP("zread", "get_repo_structure")` | Works |
| cairn.readRepoFile | Z.ai MCP zread | `callZaiMCP("zread", "read_file")` | Works |

### What's Missing
1. **Web search REST using correct key** — REST API (`/api/paas/v4/web_search`) is pay-as-you-go, needs balance. MCP search (`webSearchPrime`) uses coding plan quota. Should try MCP search as primary, REST as fallback.
2. **Web reader REST API** — could use `/api/paas/v4/reader` as REST alternative with more options (return_format, no_cache, etc.)
3. **Vision MCP** — needs subprocess integration (stdio transport), not HTTP. Requires Node.js >= 22, `@z_ai/mcp-server` package.
4. **Built-in chat web search** — could add `web_search` tool to chat completion request for RAG without separate API call
5. **Additional search params** — missing `search_recency_filter`, `search_domain_filter` in our web search
6. **Additional reader params** — missing `return_format`, `no_cache`, `retain_images` in our web reader

### Key Findings (verified 2026-03-19)

**Tool name mismatch**: Docs say `webSearchPrime` (camelCase) but actual MCP tool is `web_search_prime` (snake_case). Verified via `tools/list`.

**All search endpoints return empty**: Both REST and MCP return empty results for this API key. The account's web search quota is either exhausted or not provisioned. Web reader works fine on the same key.

**Error 1213 "prompt not received normally"**: Happens when agent loop retries tasks with corrupted tool call history. Not a search-specific bug — it's about malformed message history accumulating in sessions.

**Fix applied**: `callZaiWebSearchREST` → `callZaiMCP("web_search_prime", "web_search_prime", ...)` with REST as fallback. Empty `"[]"` responses now return "No search results found" instead of raw JSON.

**Remaining**: Web search quota needs activation on the Z.ai account. Code is correct — the endpoint works, session handshake succeeds, tool call goes through, but quota returns empty.
