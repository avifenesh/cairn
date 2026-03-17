# Piece 11: Channel Adapters

> Every action available on every surface. Web, Telegram, Slack, CLI, API, Voice.
> The agent core has ZERO channel awareness. Adapters translate both ways.

## The Principle

The web dashboard is ONE channel. Telegram is another. Slack is another. CLI is another.
The agent core emits `OutgoingMessage` and receives `IncomingMessage` — it never knows
which channel is active. The channel adapter does all translation.

## Core Interfaces

```go
// The channel contract — every adapter implements this
type Channel interface {
    Name() string                                          // "web", "telegram", "slack", "cli"
    Start(ctx context.Context, handler IncomingHandler) error // Start receiving
    Send(ctx context.Context, msg OutgoingMessage) error     // Deliver to user
    Capabilities() Capabilities                              // What this channel supports
    Close() error
}

type IncomingHandler func(ctx context.Context, msg IncomingMessage) error

// What the channel can do
type Capabilities struct {
    Markdown       bool  // Can render markdown
    InlineButtons  bool  // Interactive buttons (Telegram InlineKeyboard, Slack Block Kit)
    FileUpload     bool  // User can send files
    FileDownload   bool  // Agent can send files
    Streaming      bool  // Token-by-token streaming (SSE, message edit)
    Voice          bool  // Audio input/output
    Threads        bool  // Reply threads
    Reactions      bool  // Emoji reactions
}
```

## Canonical Message Envelope

```go
// Inbound: any channel → agent core
type IncomingMessage struct {
    ID          string            // globally unique
    SessionID   string            // ongoing conversation
    UserID      string            // channel-scoped user ID
    ChannelID   string            // which channel
    Kind        MessageKind       // Text, File, Action, Voice, Command
    Text        string            // normalized plain text
    Attachments []Attachment      // files, images
    Action      *ActionPayload    // button clicks, approvals
    Metadata    map[string]string // channel-specific extras
    ReceivedAt  time.Time
}

// Outbound: agent core → any channel
type OutgoingMessage struct {
    SessionID  string
    ChannelID  string            // destination (or "" for ActiveChannel)
    Kind       MessageKind       // Text, StreamingText, File, ActionGroup, Error
    Text       string            // CommonMark source — adapters downgrade per dialect
    StreamCh   <-chan string     // non-nil for streaming (web SSE, Telegram edit-in-place)
    Files      []File
    Actions    []ActionGroup     // semantic buttons — rendered per channel
    Metadata   map[string]string
}

type ActionGroup struct {
    ID      string
    Prompt  string    // "Deploy to production?"
    Actions []Action
}

type Action struct {
    ID    string
    Label string      // "Approve" / "Deny" / "View Diff"
    Style ActionStyle // Primary, Danger, Secondary
}
```

## Channel Router

```go
type Router struct {
    channels map[string]Channel
    sessions SessionStore
    agent    AgentCore
}

func (r *Router) HandleIncoming(ctx context.Context, msg IncomingMessage) error {
    session, _ := r.sessions.GetOrCreate(ctx, msg.UserID, msg.ChannelID)

    // Update active channel — responses follow the user
    if session.ActiveChannel != msg.ChannelID {
        session.ActiveChannel = msg.ChannelID
        r.sessions.UpdateActiveChannel(ctx, session.ID, msg.ChannelID)
    }

    return r.agent.Handle(ctx, session, msg)
}

func (r *Router) SendToUser(ctx context.Context, msg OutgoingMessage) error {
    channelID := msg.ChannelID
    if channelID == "" {
        // Route to user's active channel
        session, _ := r.sessions.Get(ctx, msg.SessionID)
        channelID = session.ActiveChannel
    }
    adapter := r.channels[channelID]
    if adapter == nil {
        return fmt.Errorf("channel %s not registered", channelID)
    }
    return adapter.Send(ctx, msg)
}
```

## Markdown Normalization

```go
func DowngradeMarkdown(text string, dialect Dialect) string {
    switch dialect {
    case DialectHTML:       return markdownToHTML(text)          // web
    case DialectTelegramV2: return markdownToTelegramV2(text)   // escape . ! ( ) etc.
    case DialectSlack:      return markdownToSlackMrkdwn(text)  // *bold* not **bold**
    case DialectPlain:      return stripMarkdown(text)          // CLI, voice
    default:                return text
    }
}
```

## Capability Matrix — Every Action on Every Channel

| Action | Web (Svelte) | Telegram | Slack | CLI | API |
|--------|-------------|----------|-------|-----|-----|
| **Chat** | SSE stream → Svelte $state | Long-poll, edit message for streaming | WebSocket, blocks | stdout streaming | POST + SSE |
| **Approve/Deny** | Button click | InlineKeyboard callback | Block Kit action | y/n prompt | POST /approve |
| **File view** | Diff viewer component | Send as Document | Snippet in thread | cat to stdout | GET /files |
| **Code review** | Split diff panel | Markdown code block | Snippet + comment | patch output | GET /diffs |
| **Voice input** | MediaRecorder → whisper | Voice message → whisper | — | — | multipart audio |
| **Voice output** | Audio element (Polly) | Send voice message | — | — | GET /tts |
| **Settings** | Settings page | /settings command → inline menu | App Home tab | config file | PUT /config |
| **Dashboard** | Full Svelte SPA | Summary text + Mini App link | Home tab | ASCII table | GET /dashboard |
| **Memory search** | Search component | /memory query → results | Slash command | CLI flag | GET /memories |
| **Task list** | Task board | /tasks → formatted list | Home tab section | Table output | GET /tasks |
| **Create memory** | Form modal | Reply with /remember | Shortcut | CLI command | POST /memories |
| **Digest** | Digest card | Formatted message | DM | stdout | GET /digest |
| **Deploy** | Button (approval-gated) | /deploy → confirm keyboard | Slash + modal | CLI command | POST /deploy |

## Notification Routing

```go
type NotificationPriority int
const (
    PriorityCritical NotificationPriority = 0  // All channels, ignore quiet hours
    PriorityHigh     NotificationPriority = 1  // Preferred channel, wake from quiet
    PriorityMedium   NotificationPriority = 2  // Preferred channel, respect quiet
    PriorityLow      NotificationPriority = 3  // Queue for digest
)

type NotificationRouter struct {
    channels   map[string]Channel
    presence   PresenceStore       // which channels user is active on
    preferences UserPreferences    // preferred channel, quiet hours
}

func (n *NotificationRouter) Route(ctx context.Context, msg OutgoingMessage, priority NotificationPriority) error {
    switch priority {
    case PriorityCritical:
        // Fan out to ALL active channels
        for _, ch := range n.channels { ch.Send(ctx, msg) }
    case PriorityHigh:
        // Preferred channel, ignore quiet hours
        ch := n.channels[n.preferences.PreferredChannel]
        return ch.Send(ctx, msg)
    case PriorityMedium:
        if n.preferences.IsQuietHours(time.Now()) { return nil } // skip
        ch := n.channels[n.preferences.PreferredChannel]
        return ch.Send(ctx, msg)
    case PriorityLow:
        // Queue for next digest
        return n.queueForDigest(ctx, msg)
    }
    return nil
}
```

## Adapters (Phase 1)

### Web Adapter (SSE)
Already exists in v1. Wraps the SSE broadcaster + REST API.
Translates `OutgoingMessage` → SSE events.
Translates REST POST → `IncomingMessage`.

### Telegram Adapter
```go
type TelegramAdapter struct {
    bot      *telego.Bot          // telego library
    chatID   int64                // single-user: known chat ID
    router   *Router
}

// Streaming: edit message every 500ms with accumulated text
// Approvals: InlineKeyboardMarkup with callback_data
// Voice: receive voice message → download → whisper STT
// Files: send as Document with caption
// Commands: /chat, /tasks, /memory, /approve, /settings, /digest
```

### CLI Adapter (future)
BubbleTea TUI or simple stdin/stdout.
For scripting: `pub chat "hello"` → stdout.

### Slack Adapter (future)
Socket Mode. Block Kit for rich messages. App Home for dashboard.

## Session Continuity

```go
type Session struct {
    ID            string
    UserID        string
    ActiveChannel string    // updated on every inbound message
    // ... rest of session fields
}

// Start conversation on web → switch to Telegram → agent responds on Telegram
// Switch back to web → agent responds on web
// The session (messages, tools, state) is the same throughout
```

## Subphases

| # | Subphase | Depends On |
|---|----------|------------|
| 11.1 | Channel interface + message types | Nothing |
| 11.2 | Router + session ActiveChannel tracking | 11.1, 4 (sessions) |
| 11.3 | Markdown normalization (per dialect) | Nothing |
| 11.4 | Web adapter (wrap existing SSE + REST) | 11.1, 9 (server) |
| 11.5 | Telegram adapter (telego, commands, keyboards) | 11.1, 11.2, 11.3 |
| 11.6 | Notification router (priority × presence) | 11.1, 11.2 |
| 11.7 | Voice pipeline integration (whisper STT in, Polly TTS out) | 11.5 |
| 11.8 | CLI adapter (BubbleTea or simple stdout) | 11.1 |
| 11.9 | Tests (mock channel, message round-trip) | All |

## What This Enables

With channel adapters in place:
- You can manage Pub entirely from Telegram while on your phone
- Approvals come as inline keyboards — tap to approve
- Digests arrive as formatted messages
- Voice notes get transcribed and processed
- The web dashboard is for when you want the full picture
- The API is for automation and external integrations
- Adding a new channel (Discord, Slack, WhatsApp) = one new adapter file
