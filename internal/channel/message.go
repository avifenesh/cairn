package channel

// IncomingMessage is the canonical representation of a message from any channel.
type IncomingMessage struct {
	ID               string            // globally unique message ID
	ChannelID        string            // "telegram", "discord", "slack", "matrix"
	UserID           string            // channel-scoped user identifier
	ChatID           string            // channel-scoped conversation ID
	SessionID        string            // Cairn session ID (set by router from channel_sessions)
	Text             string            // normalized plain text
	IsCommand        bool              // starts with /
	Command          string            // command name without /
	Args             string            // command arguments after command name
	Audio            []byte            // voice message audio data (nil = text only)
	AudioFilename    string            // original filename with extension (e.g. "voice.ogg")
	ReplyToMessageID string            // platform message ID this message is replying to (e.g. Telegram reply_to_message_id)
	Metadata         map[string]string // channel-specific extras
}

// OutgoingMessage is the canonical representation of a response to send.
type OutgoingMessage struct {
	Text     string        // markdown source (CommonMark)
	Audio    []byte        // voice reply audio (MP3, nil = text only)
	Actions  []ActionGroup // interactive buttons/approval prompts
	Priority Priority      // routing priority
}

// ActionGroup groups related actions (e.g. approve/deny pair).
type ActionGroup struct {
	Label   string   // group label (optional)
	Actions []Action // individual buttons
}

// Action represents an interactive button.
type Action struct {
	ID    string // callback data identifier
	Label string // button text
	Style string // "primary", "danger", "default"
}

// Priority controls notification routing behavior.
type Priority int

const (
	PriorityLow      Priority = iota // queue for digest
	PriorityMedium                   // preferred channel, respect quiet hours
	PriorityHigh                     // preferred channel, bypass quiet hours
	PriorityCritical                 // all channels simultaneously
)
