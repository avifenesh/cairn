package channel

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// TelegramConfig holds Telegram bot configuration.
type TelegramConfig struct {
	BotToken string // from BotFather
	ChatID   int64  // single-user chat ID
}

// TelegramAdapter implements Channel for Telegram.
type TelegramAdapter struct {
	bot     *telego.Bot
	chatID  int64
	handler MessageHandler
	logger  *slog.Logger
}

// NewTelegram creates a Telegram channel adapter.
func NewTelegram(cfg TelegramConfig, handler MessageHandler, logger *slog.Logger) (*TelegramAdapter, error) {
	if cfg.BotToken == "" {
		return nil, fmt.Errorf("telegram: bot token is required")
	}
	if logger == nil {
		logger = slog.Default()
	}

	bot, err := telego.NewBot(cfg.BotToken, telego.WithDiscardLogger())
	if err != nil {
		return nil, fmt.Errorf("telegram: create bot: %w", err)
	}

	return &TelegramAdapter{
		bot:     bot,
		chatID:  cfg.ChatID,
		handler: handler,
		logger:  logger,
	}, nil
}

func (t *TelegramAdapter) Name() string { return "telegram" }

// Bot returns the underlying telego.Bot for direct API access.
func (t *TelegramAdapter) Bot() *telego.Bot { return t.bot }

// Start begins long-polling for updates. Blocks until ctx is cancelled.
func (t *TelegramAdapter) Start(ctx context.Context) error {
	updates, err := t.bot.UpdatesViaLongPolling(ctx, nil)
	if err != nil {
		return fmt.Errorf("telegram: start polling: %w", err)
	}

	t.logger.Info("telegram polling started", "chatID", t.chatID)

	for update := range updates {
		if update.Message == nil {
			// Handle callback queries (button clicks).
			if update.CallbackQuery != nil && update.CallbackQuery.Message != nil {
				t.handleCallback(ctx, update.CallbackQuery)
			}
			continue
		}

		msg := update.Message

		// Single-user mode: ignore messages from other chats.
		if t.chatID != 0 && msg.Chat.ID != t.chatID {
			t.logger.Debug("telegram: ignoring message from unknown chat", "chat", msg.Chat.ID)
			continue
		}

		incoming := parseMessage(msg)

		// Handle voice messages — download audio file.
		if msg.Voice != nil {
			audio, err := t.downloadFile(ctx, msg.Voice.FileID)
			if err != nil {
				t.logger.Error("telegram: download voice failed", "error", err)
			} else {
				incoming.Audio = audio
				incoming.AudioFilename = "voice.ogg"
				incoming.Metadata["voiceDuration"] = fmt.Sprintf("%d", msg.Voice.Duration)
			}
		}

		username := ""
		if msg.From != nil {
			username = msg.From.Username
		}
		isVoice := len(incoming.Audio) > 0
		t.logger.Info("telegram message received",
			"from", username,
			"text", truncate(incoming.Text, 100),
			"command", incoming.Command,
			"voice", isVoice,
		)

		if t.handler == nil {
			continue
		}

		// Process message and send response.
		go func(in *IncomingMessage, chatID int64) {
			resp, err := t.handler(ctx, in)
			if err != nil {
				t.logger.Error("telegram: handler error", "error", err)
				t.sendText(ctx, chatID, "Error: "+err.Error())
				return
			}
			if resp != nil {
				t.sendResponse(ctx, chatID, resp)
			}
		}(incoming, msg.Chat.ID)
	}

	return nil
}

// Send delivers a message to the configured chat.
func (t *TelegramAdapter) Send(ctx context.Context, msg *OutgoingMessage) error {
	if t.chatID == 0 {
		return fmt.Errorf("telegram: no chat ID configured")
	}
	return t.sendResponse(ctx, t.chatID, msg)
}

func (t *TelegramAdapter) Close() error {
	return nil // long polling stops when context is cancelled
}

func (t *TelegramAdapter) sendResponse(ctx context.Context, chatID int64, msg *OutgoingMessage) error {
	// Send voice note if audio is present.
	if len(msg.Audio) > 0 {
		voiceFile := tu.FileFromBytes(msg.Audio, "voice.mp3")
		voiceParams := &telego.SendVoiceParams{
			ChatID: tu.ID(chatID),
			Voice:  voiceFile,
		}
		if _, err := t.bot.SendVoice(ctx, voiceParams); err != nil {
			t.logger.Warn("telegram: voice send failed", "error", err, "audioBytes", len(msg.Audio))
		} else {
			t.logger.Info("telegram: voice note sent", "chatID", chatID, "audioBytes", len(msg.Audio))
		}
	}

	text := msg.Text
	if text == "" {
		return nil
	}

	// Build inline keyboard if there are actions.
	var replyMarkup *telego.InlineKeyboardMarkup
	if len(msg.Actions) > 0 {
		var rows [][]telego.InlineKeyboardButton
		for _, group := range msg.Actions {
			var row []telego.InlineKeyboardButton
			for _, action := range group.Actions {
				row = append(row, telego.InlineKeyboardButton{
					Text:         action.Label,
					CallbackData: action.ID,
				})
			}
			rows = append(rows, row)
		}
		replyMarkup = &telego.InlineKeyboardMarkup{InlineKeyboard: rows}
	}

	// Chunk the raw text so each piece fits within Telegram's limit
	// after MarkdownV2 escaping (which can expand special chars).
	chunks, parseMd := chunkTelegramMessage(text, telegramMaxMessageLen)

	for i, chunk := range chunks {
		params := tu.Message(tu.ID(chatID), chunk)
		if parseMd[i] {
			params = params.WithParseMode(telego.ModeMarkdownV2)
		}

		// Attach buttons only to the last chunk.
		if replyMarkup != nil && i == len(chunks)-1 {
			params = params.WithReplyMarkup(replyMarkup)
		}

		if _, err := t.bot.SendMessage(ctx, params); err != nil {
			// Fallback: try without markdown if parse fails, keep buttons.
			t.logger.Warn("telegram: markdown send failed, retrying plain", "error", err)
			plain := tu.Message(tu.ID(chatID), stripMarkdown(chunk))
			if replyMarkup != nil && i == len(chunks)-1 {
				plain = plain.WithReplyMarkup(replyMarkup)
			}
			if _, err2 := t.bot.SendMessage(ctx, plain); err2 != nil {
				t.logger.Error("telegram: send failed", "error", err2, "chunk", i+1, "total", len(chunks))
				return err2
			}
		}
	}
	return nil
}

func (t *TelegramAdapter) sendText(ctx context.Context, chatID int64, text string) {
	if len(text) <= telegramMaxMessageLen {
		params := tu.Message(tu.ID(chatID), text)
		if _, err := t.bot.SendMessage(ctx, params); err != nil {
			t.logger.Error("telegram: send text failed", "error", err)
		}
		return
	}
	// Chunk plain text on newlines.
	remaining := text
	safeLen := telegramMaxMessageLen * 3 / 5
	for len(remaining) > 0 {
		cut := safeLen
		if len(remaining) <= telegramMaxMessageLen {
			cut = len(remaining)
		} else if nl := strings.LastIndex(remaining[:safeLen], "\n"); nl > safeLen/2 {
			cut = nl + 1
		}
		params := tu.Message(tu.ID(chatID), remaining[:cut])
		if _, err := t.bot.SendMessage(ctx, params); err != nil {
			t.logger.Error("telegram: send text failed", "error", err)
		}
		remaining = remaining[cut:]
	}
}

func (t *TelegramAdapter) handleCallback(ctx context.Context, cb *telego.CallbackQuery) {
	t.logger.Info("telegram callback", "data", cb.Data, "from", cb.From.Username)

	// Acknowledge the callback to remove the loading indicator.
	t.bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
		CallbackQueryID: cb.ID,
	})

	if t.handler == nil {
		return
	}

	incoming := &IncomingMessage{
		ID:        fmt.Sprintf("cb_%s", cb.ID),
		ChannelID: "telegram",
		UserID:    fmt.Sprintf("%d", cb.From.ID),
		ChatID:    fmt.Sprintf("%d", cb.Message.GetChat().ID),
		Text:      cb.Data,
		IsCommand: true,
		Command:   "callback",
		Args:      cb.Data,
	}

	go func() {
		resp, err := t.handler(ctx, incoming)
		if err != nil {
			t.logger.Error("telegram: callback handler error", "error", err)
			return
		}
		if resp != nil {
			t.sendResponse(ctx, cb.Message.GetChat().ID, resp)
		}
	}()
}

// parseMessage converts a Telegram message to a canonical IncomingMessage.
func parseMessage(msg *telego.Message) *IncomingMessage {
	userID := "unknown"
	if msg.From != nil {
		userID = fmt.Sprintf("%d", msg.From.ID)
	}

	in := &IncomingMessage{
		ID:        fmt.Sprintf("tg_%d", msg.MessageID),
		ChannelID: "telegram",
		UserID:    userID,
		ChatID:    fmt.Sprintf("%d", msg.Chat.ID),
		Text:      msg.Text,
		Metadata:  make(map[string]string),
	}

	if msg.From != nil {
		in.Metadata["username"] = msg.From.Username
		in.Metadata["firstName"] = msg.From.FirstName
	}

	// Parse commands: /command args
	if strings.HasPrefix(msg.Text, "/") {
		in.IsCommand = true
		parts := strings.SplitN(msg.Text, " ", 2)
		in.Command = strings.TrimPrefix(parts[0], "/")
		// Strip @botname from command (e.g. /status@cairn_bot → status)
		if idx := strings.IndexByte(in.Command, '@'); idx >= 0 {
			in.Command = in.Command[:idx]
		}
		if len(parts) > 1 {
			in.Args = parts[1]
		}
	}

	return in
}

// downloadFile downloads a Telegram file by its file ID.
func (t *TelegramAdapter) downloadFile(ctx context.Context, fileID string) ([]byte, error) {
	file, err := t.bot.GetFile(ctx, &telego.GetFileParams{FileID: fileID})
	if err != nil {
		return nil, fmt.Errorf("get file: %w", err)
	}

	token := t.bot.Token()
	url := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", token, file.FilePath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create download request: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

const (
	// telegramMaxMessageLen is the maximum message length for Telegram's sendMessage API.
	// Telegram rejects messages exceeding this limit with a "message is too long" error.
	telegramMaxMessageLen = 4096
)

// chunkMessage splits text into chunks that fit within Telegram's message size limit.
// It splits on newline boundaries to avoid breaking mid-paragraph.
// Each chunk is Normalized to Telegram MarkdownV2 before being returned,
// so the caller can send chunks directly without further processing.
// The markdown flag indicates whether each chunk should use markdown parse mode.
func chunkTelegramMessage(text string, maxLen int) (chunks []string, parseMarkdown []bool) {
	if len(text) == 0 {
		return nil, nil
	}

	// Raw chunking with a conservative byte limit to leave room for escaping.
	// MarkdownV2 escaping can add up to ~1 backslash per byte in worst case.
	// We use ~60% of the limit as the raw chunk size, then check after escaping.
	safeRawLen := maxLen * 3 / 5

	var rawChunks []string
	remaining := text

	for len(remaining) > 0 {
		if len(remaining) <= safeRawLen {
			rawChunks = append(rawChunks, remaining)
			break
		}

		// Find a newline break point within the limit.
		cut := safeRawLen
		if nl := strings.LastIndex(remaining[:safeRawLen], "\n"); nl > safeRawLen/2 {
			cut = nl + 1 // include the newline
		}

		rawChunks = append(rawChunks, remaining[:cut])
		remaining = remaining[cut:]
	}

	// Normalize each chunk and verify it fits. If a chunk still exceeds the limit
	// after escaping (rare), split it further.
	for _, raw := range rawChunks {
		normalized := Normalize(raw, "telegram")
		if len(normalized) <= maxLen {
			chunks = append(chunks, normalized)
			parseMarkdown = append(parseMarkdown, true)
		} else {
			// Fallback: strip markdown and send plain (guaranteed shorter or equal).
			plain := stripMarkdown(raw)
			if len(plain) <= maxLen {
				chunks = append(chunks, plain)
				parseMarkdown = append(parseMarkdown, false)
			} else {
				// Last resort: byte-truncate the plain text.
				chunks = append(chunks, plain[:maxLen])
				parseMarkdown = append(parseMarkdown, false)
			}
		}
	}

	return chunks, parseMarkdown
}

// truncate shortens a string to max bytes for logging.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
