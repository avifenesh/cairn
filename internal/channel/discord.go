package channel

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
)

// DiscordConfig holds Discord bot configuration.
type DiscordConfig struct {
	BotToken  string // Discord bot token
	ChannelID string // Channel ID to listen on (empty = all channels)
}

// DiscordAdapter implements Channel for Discord.
type DiscordAdapter struct {
	session   *discordgo.Session
	channelID string
	handler   MessageHandler
	logger    *slog.Logger
	done      chan struct{}
}

// NewDiscord creates a Discord channel adapter.
func NewDiscord(cfg DiscordConfig, handler MessageHandler, logger *slog.Logger) (*DiscordAdapter, error) {
	if cfg.BotToken == "" {
		return nil, fmt.Errorf("discord: bot token is required")
	}
	if logger == nil {
		logger = slog.Default()
	}

	s, err := discordgo.New("Bot " + cfg.BotToken)
	if err != nil {
		return nil, fmt.Errorf("discord: create session: %w", err)
	}

	return &DiscordAdapter{
		session:   s,
		channelID: cfg.ChannelID,
		handler:   handler,
		logger:    logger,
		done:      make(chan struct{}),
	}, nil
}

func (d *DiscordAdapter) Name() string { return "discord" }

// Start connects to Discord and begins listening. Blocks until ctx is cancelled.
func (d *DiscordAdapter) Start(ctx context.Context) error {
	d.session.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentMessageContent

	// Handle incoming messages.
	d.session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// Ignore own messages.
		if m.Author.ID == s.State.User.ID {
			return
		}
		// Filter by channel if configured.
		if d.channelID != "" && m.ChannelID != d.channelID {
			return
		}

		incoming := parseDiscordMessage(m)
		d.logger.Info("discord message received",
			"from", m.Author.Username,
			"text", truncate(incoming.Text, 100),
			"command", incoming.Command,
		)

		if d.handler == nil {
			return
		}

		go func() {
			resp, err := d.handler(ctx, incoming)
			if err != nil {
				d.logger.Error("discord: handler error", "error", err)
				d.sendText(m.ChannelID, "Error: "+err.Error())
				return
			}
			if resp != nil {
				if sendErr := d.sendResponse(m.ChannelID, resp); sendErr != nil {
					d.logger.Error("discord: send response failed", "error", sendErr)
				}
			}
		}()
	})

	// Handle button interactions.
	d.session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionMessageComponent {
			return
		}

		// Filter by channel if configured.
		if d.channelID != "" && i.ChannelID != d.channelID {
			return
		}

		data := i.MessageComponentData()
		d.logger.Info("discord interaction", "customID", data.CustomID)

		// Acknowledge the interaction.
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredMessageUpdate,
		})

		if d.handler == nil {
			return
		}

		chatID := i.ChannelID
		userID := ""
		if i.Member != nil && i.Member.User != nil {
			userID = i.Member.User.ID
		} else if i.User != nil {
			userID = i.User.ID
		}

		incoming := &IncomingMessage{
			ID:        fmt.Sprintf("dc_i_%s", i.ID),
			ChannelID: "discord",
			UserID:    userID,
			ChatID:    chatID,
			Text:      data.CustomID,
			IsCommand: true,
			Command:   "callback",
			Args:      data.CustomID,
		}

		go func() {
			resp, err := d.handler(ctx, incoming)
			if err != nil {
				d.logger.Error("discord: interaction handler error", "error", err)
				return
			}
			if resp != nil {
				if sendErr := d.sendResponse(chatID, resp); sendErr != nil {
					d.logger.Error("discord: send interaction response failed", "error", sendErr)
				}
			}
		}()
	})

	if err := d.session.Open(); err != nil {
		return fmt.Errorf("discord: open connection: %w", err)
	}
	d.logger.Info("discord connected", "channelID", d.channelID)

	// Block until context cancelled or Close called.
	select {
	case <-ctx.Done():
	case <-d.done:
	}

	d.session.Close()
	return nil
}

// Send delivers a message to the configured channel.
func (d *DiscordAdapter) Send(_ context.Context, msg *OutgoingMessage) error {
	if d.channelID == "" {
		return fmt.Errorf("discord: no channel ID configured")
	}
	return d.sendResponse(d.channelID, msg)
}

func (d *DiscordAdapter) Close() error {
	select {
	case <-d.done:
	default:
		close(d.done)
	}
	return nil
}

func (d *DiscordAdapter) sendResponse(channelID string, msg *OutgoingMessage) error {
	text := Normalize(msg.Text, "discord")
	if text == "" {
		return nil
	}

	components := buildDiscordComponents(msg.Actions)

	// Discord has a 2000-char limit. Split if needed.
	chunks := splitMessage(text, 2000)
	for i, chunk := range chunks {
		send := &discordgo.MessageSend{Content: chunk}
		// Attach buttons only to the last chunk.
		if i == len(chunks)-1 && len(components) > 0 {
			send.Components = components
		}
		if _, err := d.session.ChannelMessageSendComplex(channelID, send); err != nil {
			d.logger.Error("discord: send failed", "error", err)
			return err
		}
	}
	return nil
}

func (d *DiscordAdapter) sendText(channelID, text string) {
	if _, err := d.session.ChannelMessageSend(channelID, text); err != nil {
		d.logger.Error("discord: send text failed", "error", err)
	}
}

// parseDiscordMessage converts a Discord message to a canonical IncomingMessage.
func parseDiscordMessage(m *discordgo.MessageCreate) *IncomingMessage {
	in := &IncomingMessage{
		ID:        fmt.Sprintf("dc_%s", m.ID),
		ChannelID: "discord",
		UserID:    m.Author.ID,
		ChatID:    m.ChannelID,
		Text:      m.Content,
		Metadata:  make(map[string]string),
	}

	in.Metadata["username"] = m.Author.Username

	// Parse commands: /command args
	if strings.HasPrefix(m.Content, "/") {
		in.IsCommand = true
		parts := strings.SplitN(m.Content, " ", 2)
		in.Command = strings.TrimPrefix(parts[0], "/")
		// Strip @mentions from command.
		if idx := strings.IndexByte(in.Command, '@'); idx >= 0 {
			in.Command = in.Command[:idx]
		}
		if len(parts) > 1 {
			in.Args = parts[1]
		}
	}

	return in
}

// buildDiscordComponents converts ActionGroups to Discord message components.
func buildDiscordComponents(actions []ActionGroup) []discordgo.MessageComponent {
	if len(actions) == 0 {
		return nil
	}

	var components []discordgo.MessageComponent
	for _, group := range actions {
		var buttons []discordgo.MessageComponent
		for _, action := range group.Actions {
			style := discordgo.SecondaryButton
			switch action.Style {
			case "primary":
				style = discordgo.PrimaryButton
			case "danger":
				style = discordgo.DangerButton
			}
			buttons = append(buttons, discordgo.Button{
				Label:    action.Label,
				Style:    style,
				CustomID: action.ID,
			})
		}
		if len(buttons) > 0 {
			components = append(components, discordgo.ActionsRow{Components: buttons})
		}
	}
	return components
}

// splitMessage splits text into chunks of at most maxLen runes,
// preferring to split on newline boundaries, then space boundaries.
// Uses rune counting to avoid splitting multi-byte UTF-8 characters.
func splitMessage(text string, maxLen int) []string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return []string{text}
	}

	var chunks []string
	for len(runes) > 0 {
		if len(runes) <= maxLen {
			chunks = append(chunks, string(runes))
			break
		}

		segment := string(runes[:maxLen])

		// Find a good split point: prefer newline, then space, then hard cut.
		cut := maxLen
		if idx := strings.LastIndex(segment, "\n"); idx > 0 {
			cut = utf8.RuneCountInString(segment[:idx+1])
		} else if idx := strings.LastIndex(segment, " "); idx > 0 {
			cut = utf8.RuneCountInString(segment[:idx+1])
		}

		chunks = append(chunks, string(runes[:cut]))
		runes = runes[cut:]
	}
	return chunks
}
