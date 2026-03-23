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
	bot        *telego.Bot
	chatID     int64
	handler    MessageHandler
	logger     *slog.Logger
	replyStore *ReplyStore
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

// SetReplyStore injects a ReplyStore for saving outgoing message IDs.
func (t *TelegramAdapter) SetReplyStore(rs *ReplyStore) {
	t.replyStore = rs
}

// Telegram's sendMessage limit is 4096 characters (runes).
// MarkdownV2 escaping can inflate text significantly (special chars get backslash-prefixed),
// so we use a conservative limit for MarkdownV2 and the full 4096 for plain text.
const (
	tgMaxMessageChars   = 4096
	tgChunkHeaderMaxLen = 40 // "─── Part X/Y ───\n" (max chars for part header)
)

// tgRawChunkLimit estimates a raw-text rune limit that, after MarkdownV2 escaping,
// stays within the Telegram message size limit. Since escaping adds one backslash per
// special character, we conservatively assume 15% inflation for typical agent output.
// Computed as (tgMaxMessageChars - tgChunkHeaderMaxLen) / 1.15 ≈ 3526.
var tgRawChunkLimit = (tgMaxMessageChars - tgChunkHeaderMaxLen) * 100 / 115

// sendChunks splits text into chunks respecting Telegram's message size limit
// and sends them sequentially with a small delay between chunks.
// When chunking is needed (multiple chunks), the chunk limit is reduced by
// tgChunkHeaderMaxLen so that the per-part header fits within the total limit.
// This is used for plain-text sends where the text is already in final form.
// Returns the sent message IDs for reply context tracking.
func (t *TelegramAdapter) sendChunks(ctx context.Context, chatID int64, text string, parseMode string, replyMarkup *telego.InlineKeyboardMarkup) (int, []int, []string, error) {
	chunks := splitMessage(text, tgMaxMessageChars)

	if len(chunks) == 1 {
		params := tu.Message(tu.ID(chatID), chunks[0])
		if parseMode != "" {
			params = params.WithParseMode(parseMode)
		}
		if replyMarkup != nil {
			params = params.WithReplyMarkup(replyMarkup)
		}
		sent, err := t.bot.SendMessage(ctx, params)
		if err != nil {
			return 0, nil, nil, err
		}
		return 1, []int{sent.MessageID}, []string{chunks[0]}, nil
	}

	// Multiple chunks: reduce limit to account for per-part headers, re-split.
	chunkLimit := tgMaxMessageChars - tgChunkHeaderMaxLen
	chunks = splitMessage(text, chunkLimit)

	n, ids, texts, err := t.sendChunkedMessages(ctx, chatID, chunks, parseMode, replyMarkup)
	return n, ids, texts, err
}

// sendChunksMarkdownV2 splits raw (unescaped) text into chunks, escapes each chunk
// independently for MarkdownV2, then sends them. Chunking raw text before escaping
// prevents escape sequences introduced by Normalize from being split across chunk
// boundaries (e.g., a backslash separated from the character it escapes). This reduces
// the risk of malformed MarkdownV2 that Telegram rejects, but does not guarantee that
// arbitrary higher-level Markdown constructs (such as very long links, code blocks, or
// emphasis spans) will remain entirely within a single chunk.
//
// Returns (true, messageIDs, nil) if at least one chunk was delivered successfully, so callers
// can avoid duplicate plain-text retries after partial sends.
func (t *TelegramAdapter) sendChunksMarkdownV2(ctx context.Context, chatID int64, rawText string, replyMarkup *telego.InlineKeyboardMarkup) (sent bool, msgIDs []int, chunkTexts []string, err error) {
	// Estimate chunk limit for raw text that won't exceed Telegram limit after escaping.
	// Use a per-rune limit that accounts for escaping inflation and header overhead.
	rawLimit := tgRawChunkLimit

	// Optimization: if raw text fits in one message, try sending it directly
	// without pre-splitting. This avoids unnecessary chunking for messages that
	// fit after escaping (e.g., 3500-char prose with few special characters).
	if len([]rune(rawText)) <= tgRawChunkLimit {
		escaped := Normalize(rawText, "telegram")
		if len([]rune(escaped)) <= tgMaxMessageChars {
			params := tu.Message(tu.ID(chatID), escaped).WithParseMode(telego.ModeMarkdownV2)
			if replyMarkup != nil {
				params = params.WithReplyMarkup(replyMarkup)
			}
			sent, err := t.bot.SendMessage(ctx, params)
			if err != nil {
				return false, nil, nil, err
			}
			return true, []int{sent.MessageID}, []string{rawText}, nil
		}
		// Escaped text too large — fall through to chunking path.
	}

	chunks := splitMessage(rawText, rawLimit)

	if len(chunks) == 1 {
		escaped := Normalize(chunks[0], "telegram")
		// If single escaped chunk fits, send it directly.
		if len([]rune(escaped)) <= tgMaxMessageChars {
			params := tu.Message(tu.ID(chatID), escaped).WithParseMode(telego.ModeMarkdownV2)
			if replyMarkup != nil {
				params = params.WithReplyMarkup(replyMarkup)
			}
			sent, err := t.bot.SendMessage(ctx, params)
			if err != nil {
				return false, nil, nil, err
			}
			return true, []int{sent.MessageID}, []string{chunks[0]}, nil
		}
		// Even a single chunk escaped is too large — re-split with tighter limit.
		retryLimit := tgRawChunkLimit - tgChunkHeaderMaxLen
		chunks = splitMessage(rawText, retryLimit)
	}

	sentCount, ids, texts, err := t.sendChunkedMessages(ctx, chatID, chunks, telego.ModeMarkdownV2, replyMarkup)
	return sentCount > 0, ids, texts, err
}

// sendChunkedMessages sends pre-split text chunks with per-part headers and rate-limit delays.
// Each chunk is escaped for MarkdownV2 if parseMode is set; otherwise sent as-is (plain text).
// Returns the number of chunks successfully delivered, their platform message IDs,
// and the text content of each delivered chunk (for reply context storage).
func (t *TelegramAdapter) sendChunkedMessages(ctx context.Context, chatID int64, chunks []string, parseMode string, replyMarkup *telego.InlineKeyboardMarkup) (int, []int, []string, error) {
	sent := 0
	var msgIDs []int
	var chunkTexts []string
	for i, chunk := range chunks {
		header := fmt.Sprintf("─── Part %d/%d ───\n", i+1, len(chunks))

		// For MarkdownV2, escape the chunk content independently.
		text := chunk
		effectiveParseMode := parseMode
		if parseMode != "" {
			text = Normalize(chunk, "telegram")
		}
		full := header + text

		// Safety check: if escaped chunk still exceeds limit, fall back to plain text
		// instead of truncating mid-escape (which would produce invalid MarkdownV2).
		if len([]rune(full)) > tgMaxMessageChars {
			if parseMode != "" {
				full = header + chunk
				effectiveParseMode = ""
			} else {
				rs := []rune(full)
				full = string(rs[:tgMaxMessageChars])
			}
		}

		params := tu.Message(tu.ID(chatID), full)
		if effectiveParseMode != "" {
			params = params.WithParseMode(effectiveParseMode)
		}
		// Only attach keyboard to the last chunk.
		if i == len(chunks)-1 && replyMarkup != nil {
			params = params.WithReplyMarkup(replyMarkup)
		}

		sentMsg, err := t.bot.SendMessage(ctx, params)
		if err != nil {
			t.logger.Error("telegram: chunk send failed",
				"chunk", i+1, "total", len(chunks),
				"chars", len([]rune(full)), "error", err)
			return sent, msgIDs, chunkTexts, err
		}
		msgIDs = append(msgIDs, sentMsg.MessageID)
		chunkTexts = append(chunkTexts, full)
		sent++

		// Brief delay between chunks to avoid rate limiting.
		if i < len(chunks)-1 {
			timer := time.NewTimer(300 * time.Millisecond)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return sent, msgIDs, chunkTexts, ctx.Err()
			}
		}
	}

	return sent, msgIDs, chunkTexts, nil
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

	if msg.Text == "" {
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

	// Try MarkdownV2 first with per-chunk escaping to avoid mid-entity splits.
	sent, ids, texts, err := t.sendChunksMarkdownV2(ctx, chatID, msg.Text, replyMarkup)
	if err != nil {
		// Only fall back to plain text if nothing was delivered yet.
		// If any chunk was sent, retrying from scratch would duplicate content in the chat.
		if sent {
			t.logger.Warn("telegram: markdown chunked send failed after partial delivery, skipping plain-text fallback",
				"error", err)
			t.saveReplyIDs(chatID, ids, texts)
			return err
		}
		// No chunks were delivered — safe to retry as plain text.
		t.logger.Warn("telegram: markdown send failed, retrying plain", "error", err)
		plain := stripMarkdown(msg.Text)
		_, plainIDs, plainTexts, err2 := t.sendChunks(ctx, chatID, plain, "", replyMarkup)
		if err2 != nil {
			t.logger.Error("telegram: send failed", "error", err2)
			return err2
		}
		ids = plainIDs
		// Save the actual delivered text (plain) so reply context matches what users saw.
		t.saveReplyIDs(chatID, ids, plainTexts)
		return nil
	}
	t.saveReplyIDs(chatID, ids, texts)
	return nil
}

// saveReplyIDs saves outgoing message IDs to the ReplyStore so users can
// reply with context. Each text[i] corresponds to msgIDs[i], enabling
// per-chunk reply context for chunked messages. Only saves if a ReplyStore
// is configured. Uses rune-based truncation to preserve valid UTF-8.
func (t *TelegramAdapter) saveReplyIDs(chatID int64, msgIDs []int, texts []string) {
	if t.replyStore == nil || len(msgIDs) == 0 {
		return
	}
	chatIDStr := fmt.Sprintf("%d", chatID)
	for i, id := range msgIDs {
		text := ""
		if i < len(texts) {
			text = TruncateRune(texts[i], 2000)
		}
		if text != "" {
			t.replyStore.Save("telegram", chatIDStr, fmt.Sprintf("%d", id), text)
		}
	}
}

func (t *TelegramAdapter) sendText(ctx context.Context, chatID int64, text string) {
	if _, _, _, err := t.sendChunks(ctx, chatID, text, "", nil); err != nil {
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

	// Extract reply-to reference for context injection.
	if msg.ReplyToMessage != nil {
		in.ReplyToMessageID = fmt.Sprintf("%d", msg.ReplyToMessage.MessageID)
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
