# Piece 9: Server & Protocols

> HTTP server, SSE streaming, MCP server, A2A server, ACP support, auth, permissions.

## HTTP Server

```go
// Chi or Echo router — lightweight, fast, idiomatic
type Server struct {
    router     chi.Router
    sse        *SSEBroadcaster
    bus        *eventbus.Bus
    agent      agent.Agent
    taskEngine task.Engine
    sessions   session.SessionStore
    config     *config.Config
}

func (s *Server) Start(addr string) error
func (s *Server) Shutdown(ctx context.Context) error
```

## REST API Routes

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | /health | Health check | none |
| GET | /ready | Readiness probe | none |
| GET | /v1/feed | Paginated feed | read |
| GET | /v1/dashboard | Dashboard summary | read |
| GET | /v1/stream | SSE event stream | read |
| POST | /v1/assistant/message | Send chat message | write |
| POST | /v1/assistant/voice | Voice upload → transcribe → chat | write |
| GET | /v1/assistant/voice/tts | Text-to-speech | read |
| GET | /v1/tasks | List tasks | read |
| POST | /v1/tasks/:id/cancel | Cancel task | write |
| GET | /v1/memories | List memories | read |
| POST | /v1/memories | Create memory | write |
| GET | /v1/approvals | List pending approvals | read |
| POST | /v1/approvals/:id | Approve/deny | write |
| GET | /v1/sessions | List chat sessions | read |
| GET | /v1/sessions/:id | Get session + events | read |
| GET | /v1/agents | List registered agents | read |
| GET | /v1/skills | List skills | read |
| POST | /v1/poll/run | Trigger manual poll | write |
| POST | /v1/webhooks/:name | Receive webhook | signature |

## SSE Broadcasting

```go
type SSEBroadcaster struct {
    clients   sync.Map           // clientID → *SSEClient
    bus       *eventbus.Bus
    replay    *ReplayBuffer      // last 1000 events for reconnection
}

type SSEClient struct {
    id      string
    writer  http.ResponseWriter
    flusher http.Flusher
    events  chan []byte
    done    chan struct{}
}

// Subscribe to bus events, format as SSE, fan out to clients
// Support Last-Event-ID reconnection via replay buffer
```

## MCP Server (via mcp-go)

```go
// Expose Pub's tools and resources as MCP server
type MCPServer struct {
    server *mcpserver.MCPServer
    tools  *tool.Registry
    bus    *eventbus.Bus
}

// Tools: all built-in tools exposed via MCP
// Resources: feed events, memories, sessions
// Transport: stdio (for local) + HTTP/SSE (for remote)
```

## A2A Server (ADK-Go inspired)

```go
// Agent-to-Agent protocol server
// Allows external agents to send tasks to Pub
type A2AServer struct {
    taskEngine task.Engine
    agent      agent.Agent
}

// POST /.well-known/agent.json → agent card
// POST /a2a/tasks → submit task
// GET  /a2a/tasks/:id → task status
// POST /a2a/tasks/:id/cancel → cancel task
```

## Auth

```go
type AuthMiddleware struct {
    readTokens  []string          // optional read protection
    writeTokens []string          // required for writes
    sessionValidator func(string) bool  // WebAuthn session check
}

// Token sources (precedence):
// 1. x-api-token header
// 2. Authorization: Bearer header
// 3. ?token= query param (for EventSource)
// 4. cairn_session cookie (WebAuthn)
```

## Static File Server

```go
// Serve frontend files (index.html, app.js, styles.css)
// Go's net/http.FileServer or chi.FileServer
// Caddy no longer needed for static files — Go handles it directly
// Caddy remains only for TLS termination (Cloudflare origin cert)
```

## Subphases

| # | Subphase | Depends On |
|---|----------|------------|
| 9.1 | HTTP server + router setup | Nothing |
| 9.2 | Auth middleware (tokens + WebAuthn) | 9.1 |
| 9.3 | REST routes (feed, tasks, memories, sessions) | 9.1, 9.2, all stores |
| 9.4 | SSE broadcaster with replay buffer | 9.1, 1 (event bus) |
| 9.5 | Assistant message endpoint (→ agent) | 9.1, 9.2, 4 (agent) |
| 9.6 | Voice endpoints (whisper STT + Polly TTS) | 9.1, 9.2 |
| 9.7 | MCP server (via mcp-go) | 9.1, 3 (tools) |
| 9.8 | A2A server | 9.1, 5 (task engine) |
| 9.9 | Static file server | 9.1 |
| 9.10 | Rate limiting + CORS | 9.1 |
| 9.11 | Tests | All |
