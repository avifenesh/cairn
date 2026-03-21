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
	BotToken          string // from BotFather
	ChatID            int64  // single-user chat ID (commands)
	NotificationChatID int64 // separate chat for notifications (0 = use ChatID)
}

// TelegramAdapter implements Channel for Telegram.
type TelegramAdapter struct {
	bot               *telego.Bot
	chatID            int64 // command chat (inbound messages)
	notificationChatID int64 // notification chat (outbound only, 0 = fallback to chatID)
	handler           MessageHandler
	logger            *slog.Logger
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
		bot:               bot,
		chatID:            cfg.ChatID,
		notificationChatID: cfg.NotificationChatID,
		handler:           handler,
		logger:            logger,
	}, nil
}

func (t *TelegramAdapter) Name() string { return "telegram" }

// Start begins long-polling for updates. Blocks until ctx is cancelled.
func (t *TelegramAdapter) Start(ctx context.Context) error {
	updates, err := t.bot.UpdatesViaLongPolling(ctx, nil)
	if err != nil {
		return fmt.Errorf("telegram: start polling: %w", err)
	}

	t.logger.Info("telegram polling started", "chatID", t.chatID, "notificationChatID", t.notificationChatID)

	// Security: if notificationChatID is set but chatID is 0, inbound auth is disabled.
	// Warn loudly — commands would be accepted from any chat.
	if t.chatID == 0 && t.notificationChatID != 0 {
		t.logger.Error("telegram: TELEGRAM_CHAT_ID is 0 but TELEGRAM_NOTIFICATION_CHAT_ID is set — inbound command auth is disabled, bot will accept messages from any chat")
	}

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

// Send delivers a message to the configured notification chat.
// If NotificationChatID is set, notifications go there instead of ChatID.
func (t *TelegramAdapter) Send(ctx context.Context, msg *OutgoingMessage) error {
	targetChat := t.chatID
	if t.notificationChatID != 0 {
		targetChat = t.notificationChatID
	}
	if targetChat == 0 {
		return fmt.Errorf("telegram: no chat ID configured")
	}
	return t.sendResponse(ctx, targetChat, msg)
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

	text := Normalize(msg.Text, "telegram")
	if text == "" {
		return nil
	}

	params := tu.Message(tu.ID(chatID), text).
		WithParseMode(telego.ModeMarkdownV2)

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
		params = params.WithReplyMarkup(replyMarkup)
	}

	if _, err := t.bot.SendMessage(ctx, params); err != nil {
		// Fallback: try without markdown if parse fails, keep buttons.
		t.logger.Warn("telegram: markdown send failed, retrying plain", "error", err)
		plain := tu.Message(tu.ID(chatID), stripMarkdown(msg.Text))
		if replyMarkup != nil {
			plain = plain.WithReplyMarkup(replyMarkup)
		}
		if _, err2 := t.bot.SendMessage(ctx, plain); err2 != nil {
			t.logger.Error("telegram: send failed", "error", err2)
			return err2
		}
	}
	return nil
}

func (t *TelegramAdapter) sendText(ctx context.Context, chatID int64, text string) {
	params := tu.Message(tu.ID(chatID), text)
	if _, err := t.bot.SendMessage(ctx, params); err != nil {
		t.logger.Error("telegram: send text failed", "error", err)
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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
