package channel

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// SlackConfig holds Slack bot configuration.
type SlackConfig struct {
	BotToken  string // xoxb-... bot token
	AppToken  string // xapp-... app-level token (Socket Mode)
	ChannelID string // Channel to listen on (empty = all channels)
}

// SlackAdapter implements Channel for Slack via Socket Mode.
type SlackAdapter struct {
	api        *slack.Client
	sm         *socketmode.Client
	channelID  string
	botUserID  string // resolved during Start to filter self-messages
	handler    MessageHandler
	logger     *slog.Logger
	replyStore *ReplyStore
}

// NewSlack creates a Slack channel adapter using Socket Mode.
func NewSlack(cfg SlackConfig, handler MessageHandler, logger *slog.Logger) (*SlackAdapter, error) {
	if cfg.BotToken == "" {
		return nil, fmt.Errorf("slack: bot token is required")
	}
	if cfg.AppToken == "" {
		return nil, fmt.Errorf("slack: app token is required (Socket Mode)")
	}
	if logger == nil {
		logger = slog.Default()
	}

	api := slack.New(cfg.BotToken, slack.OptionAppLevelToken(cfg.AppToken))
	sm := socketmode.New(api)

	return &SlackAdapter{
		api:       api,
		sm:        sm,
		channelID: cfg.ChannelID,
		handler:   handler,
		logger:    logger,
	}, nil
}

func (s *SlackAdapter) Name() string { return "slack" }

// Start connects via Socket Mode and begins listening. Blocks until ctx is cancelled.
func (s *SlackAdapter) Start(ctx context.Context) error {
	// Resolve bot user ID to filter own messages.
	auth, err := s.api.AuthTestContext(ctx)
	if err != nil {
		return fmt.Errorf("slack: auth test: %w", err)
	}
	s.botUserID = auth.UserID
	s.logger.Info("slack connected", "botUser", s.botUserID, "channelID", s.channelID)

	// Run Socket Mode client in background.
	go func() {
		if err := s.sm.RunContext(ctx); err != nil && ctx.Err() == nil {
			s.logger.Error("slack: socket mode error", "error", err)
		}
	}()

	// Process events.
	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-s.sm.Events:
			if !ok {
				return nil
			}
			s.handleEvent(ctx, evt)
		}
	}
}

// Send delivers a message to the configured channel.
func (s *SlackAdapter) Send(ctx context.Context, msg *OutgoingMessage) error {
	if s.channelID == "" {
		return fmt.Errorf("slack: no channel ID configured")
	}
	return s.sendResponse(ctx, s.channelID, msg)
}

func (s *SlackAdapter) Close() error {
	return nil // Socket Mode stops when context is cancelled
}

// SetReplyStore injects a ReplyStore for saving outgoing message timestamps.
func (s *SlackAdapter) SetReplyStore(rs *ReplyStore) {
	s.replyStore = rs
}

func (s *SlackAdapter) handleEvent(ctx context.Context, evt socketmode.Event) {
	switch evt.Type {
	case socketmode.EventTypeEventsAPI:
		eventsAPI, ok := evt.Data.(slackevents.EventsAPIEvent)
		if !ok {
			return
		}
		s.sm.Ack(*evt.Request)

		if eventsAPI.Type == slackevents.CallbackEvent {
			s.handleCallbackEvent(ctx, eventsAPI.InnerEvent)
		}

	case socketmode.EventTypeInteractive:
		cb, ok := evt.Data.(slack.InteractionCallback)
		if !ok {
			return
		}
		s.sm.Ack(*evt.Request)

		s.handleInteraction(ctx, cb)
	}
}

func (s *SlackAdapter) handleCallbackEvent(ctx context.Context, inner slackevents.EventsAPIInnerEvent) {
	ev, ok := inner.Data.(*slackevents.MessageEvent)
	if !ok {
		return
	}

	// Ignore bot messages and own messages.
	if ev.BotID != "" || ev.User == s.botUserID {
		return
	}
	// Ignore message subtypes (edits, deletes, etc.) — only handle new messages.
	if ev.SubType != "" {
		return
	}
	// Filter by channel if configured.
	if s.channelID != "" && ev.Channel != s.channelID {
		return
	}

	incoming := parseSlackMessage(ev)
	s.logger.Info("slack message received",
		"from", ev.User,
		"text", truncate(incoming.Text, 100),
		"command", incoming.Command,
	)

	if s.handler == nil {
		return
	}

	go func() {
		resp, err := s.handler(ctx, incoming)
		if err != nil {
			s.logger.Error("slack: handler error", "error", err)
			s.api.PostMessageContext(ctx, ev.Channel,
				slack.MsgOptionText("Error: "+err.Error(), false))
			return
		}
		if resp != nil {
			s.sendResponse(ctx, ev.Channel, resp)
		}
	}()
}

func (s *SlackAdapter) handleInteraction(ctx context.Context, cb slack.InteractionCallback) {
	if len(cb.ActionCallback.BlockActions) == 0 {
		return
	}

	// Filter by channel if configured.
	if s.channelID != "" && cb.Channel.ID != s.channelID {
		return
	}

	action := cb.ActionCallback.BlockActions[0]
	s.logger.Info("slack interaction", "actionID", action.ActionID)

	if s.handler == nil {
		return
	}

	chatID := cb.Channel.ID
	incoming := &IncomingMessage{
		ID:        fmt.Sprintf("sl_i_%s_%s", cb.TriggerID, action.ActionID),
		ChannelID: "slack",
		UserID:    cb.User.ID,
		ChatID:    chatID,
		Text:      action.ActionID,
		IsCommand: true,
		Command:   "callback",
		Args:      action.ActionID,
	}

	go func() {
		resp, err := s.handler(ctx, incoming)
		if err != nil {
			s.logger.Error("slack: interaction handler error", "error", err)
			return
		}
		if resp != nil {
			s.sendResponse(ctx, chatID, resp)
		}
	}()
}

func (s *SlackAdapter) sendResponse(ctx context.Context, channelID string, msg *OutgoingMessage) error {
	text := Normalize(msg.Text, "slack")
	if text == "" {
		return nil
	}

	blocks := buildSlackBlocks(text, msg.Actions)

	_, ts, err := s.api.PostMessageContext(ctx, channelID,
		slack.MsgOptionBlocks(blocks...),
		slack.MsgOptionText(text, false), // fallback for notifications
	)
	if err != nil {
		s.logger.Error("slack: send failed", "error", err)
		return err
	}
	// Save timestamp for reply context tracking.
	if s.replyStore != nil && ts != "" {
		saveText := msg.Text
		if len(saveText) > 2000 {
			saveText = saveText[:2000] + "..."
		}
		s.replyStore.Save("slack", channelID, ts, saveText)
	}
	return nil
}

// parseSlackMessage converts a Slack message event to a canonical IncomingMessage.
func parseSlackMessage(ev *slackevents.MessageEvent) *IncomingMessage {
	in := &IncomingMessage{
		ID:        fmt.Sprintf("sl_%s", ev.TimeStamp),
		ChannelID: "slack",
		UserID:    ev.User,
		ChatID:    ev.Channel,
		Text:      ev.Text,
		Metadata:  make(map[string]string),
	}

	if ev.ThreadTimeStamp != "" {
		in.Metadata["threadTs"] = ev.ThreadTimeStamp
	}

	// Extract reply-to reference for context injection.
	// A message posted as a thread reply has ThreadTimeStamp != TimeStamp.
	if ev.ThreadTimeStamp != "" && ev.ThreadTimeStamp != ev.TimeStamp {
		in.ReplyToMessageID = ev.ThreadTimeStamp
	}

	// Parse commands: /command args
	if strings.HasPrefix(ev.Text, "/") {
		in.IsCommand = true
		parts := strings.SplitN(ev.Text, " ", 2)
		in.Command = strings.TrimPrefix(parts[0], "/")
		if len(parts) > 1 {
			in.Args = parts[1]
		}
	}

	return in
}

// buildSlackBlocks creates Block Kit blocks from text and actions.
func buildSlackBlocks(text string, actions []ActionGroup) []slack.Block {
	var blocks []slack.Block

	// Text section.
	textBlock := slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", text, false, false),
		nil, nil,
	)
	blocks = append(blocks, textBlock)

	// Action buttons.
	for _, group := range actions {
		var elements []slack.BlockElement
		for _, action := range group.Actions {
			btn := slack.NewButtonBlockElement(action.ID, action.ID,
				slack.NewTextBlockObject("plain_text", action.Label, false, false))
			switch action.Style {
			case "primary":
				btn.Style = slack.StylePrimary
			case "danger":
				btn.Style = slack.StyleDanger
			}
			elements = append(elements, btn)
		}
		if len(elements) > 0 {
			blocks = append(blocks, slack.NewActionBlock("", elements...))
		}
	}

	return blocks
}
