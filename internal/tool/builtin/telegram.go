package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/avifenesh/cairn/internal/tool"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// TelegramBot is set at startup when a Telegram adapter is configured.
var telegramBot *telego.Bot
var telegramDefaultChatID int64

// SetTelegramBot configures the Telegram bot instance for agent tools.
func SetTelegramBot(bot *telego.Bot, defaultChatID int64) {
	telegramBot = bot
	telegramDefaultChatID = defaultChatID
}

// TelegramEnabled returns true when the Telegram bot is configured.
func TelegramEnabled() bool { return telegramBot != nil }

type telegramActionParams struct {
	Action string  `json:"action" desc:"Action: sendPhoto, sendDocument, sendPoll, sendInvoice, pinMessage, unpinMessage, setCommands, deleteCommands, getCommands, createInviteLink, setMenuButton, promoteMember, restrictMember, banMember, unbanMember, createForumTopic, closeForumTopic, getChat, getMemberCount, sendSticker, editMessage, deleteMessage, forwardMessage"`
	ChatID *int64  `json:"chatID" desc:"Target chat ID. Omit to use default."`
	Params *string `json:"params" desc:"JSON object with action-specific parameters."`
}

var telegramAction = tool.Define("cairn.telegram",
	"Execute Telegram Bot API actions: send media/polls/invoices, manage groups, set commands, "+
		"pin messages, manage members, create forum topics, and more. "+
		"Params JSON varies by action - see action list in description.",
	[]tool.Mode{tool.ModeTalk, tool.ModeWork, tool.ModeCoding},
	func(ctx *tool.ToolContext, p telegramActionParams) (*tool.ToolResult, error) {
		if telegramBot == nil {
			return &tool.ToolResult{Error: "Telegram bot not configured"}, nil
		}

		chatID := telegramDefaultChatID
		if p.ChatID != nil {
			chatID = *p.ChatID
		}
		if chatID == 0 && needsChatID(p.Action) {
			return &tool.ToolResult{Error: "chatID required"}, nil
		}

		var params map[string]any
		if p.Params != nil {
			if err := json.Unmarshal([]byte(*p.Params), &params); err != nil {
				return &tool.ToolResult{Error: fmt.Sprintf("invalid params JSON: %v", err)}, nil
			}
		}
		if params == nil {
			params = make(map[string]any)
		}

		c := context.Background()

		switch p.Action {
		case "sendPhoto":
			return tgSendPhoto(c, chatID, params)
		case "sendDocument":
			return tgSendDocument(c, chatID, params)
		case "sendPoll":
			return tgSendPoll(c, chatID, params)
		case "sendInvoice":
			return tgSendInvoice(c, chatID, params)
		case "sendSticker":
			return tgSendSticker(c, chatID, params)
		case "pinMessage":
			return tgPinMessage(c, chatID, params)
		case "unpinMessage":
			return tgUnpinMessage(c, chatID, params)
		case "setCommands":
			return tgSetCommands(c, params)
		case "deleteCommands":
			return tgDeleteCommands(c)
		case "getCommands":
			return tgGetCommands(c)
		case "createInviteLink":
			return tgCreateInviteLink(c, chatID, params)
		case "setMenuButton":
			return tgSetMenuButton(c, chatID, params)
		case "promoteMember":
			return tgPromoteMember(c, chatID, params)
		case "restrictMember":
			return tgRestrictMember(c, chatID, params)
		case "banMember":
			return tgBanMember(c, chatID, params)
		case "unbanMember":
			return tgUnbanMember(c, chatID, params)
		case "createForumTopic":
			return tgCreateForumTopic(c, chatID, params)
		case "closeForumTopic":
			return tgCloseForumTopic(c, chatID, params)
		case "getChat":
			return tgGetChat(c, chatID)
		case "getMemberCount":
			return tgGetMemberCount(c, chatID)
		case "editMessage":
			return tgEditMessage(c, chatID, params)
		case "deleteMessage":
			return tgDeleteMessage(c, chatID, params)
		case "forwardMessage":
			return tgForwardMessage(c, chatID, params)
		default:
			return &tool.ToolResult{Error: fmt.Sprintf("unknown action %q", p.Action)}, nil
		}
	},
)

func needsChatID(action string) bool {
	switch action {
	case "setCommands", "deleteCommands", "getCommands":
		return false
	default:
		return true
	}
}

func getStr(m map[string]any, key string) string { v, _ := m[key].(string); return v }
func getInt(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	default:
		return 0
	}
}
func getInt64(m map[string]any, key string) int64 {
	switch v := m[key].(type) {
	case float64:
		return int64(v)
	default:
		return 0
	}
}
func getBool(m map[string]any, key string) bool { v, _ := m[key].(bool); return v }
func boolPtr(b bool) *bool                      { return &b }

func okResult(msg string) *tool.ToolResult { return &tool.ToolResult{Output: msg} }
func jsonResult(v any) *tool.ToolResult {
	b, _ := json.Marshal(v)
	return &tool.ToolResult{Output: string(b)}
}

// --- Actions ---

func tgSendPhoto(ctx context.Context, chatID int64, p map[string]any) (*tool.ToolResult, error) {
	url := getStr(p, "photoURL")
	if url == "" {
		return &tool.ToolResult{Error: "photoURL required"}, nil
	}
	params := tu.Photo(tu.ID(chatID), tu.FileFromURL(url))
	if c := getStr(p, "caption"); c != "" {
		params = params.WithCaption(c).WithParseMode(telego.ModeHTML)
	}
	msg, err := telegramBot.SendPhoto(ctx, params)
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("sendPhoto: %v", err)}, nil
	}
	return okResult(fmt.Sprintf("Photo sent (message %d)", msg.MessageID)), nil
}

func tgSendDocument(ctx context.Context, chatID int64, p map[string]any) (*tool.ToolResult, error) {
	url := getStr(p, "documentURL")
	if url == "" {
		return &tool.ToolResult{Error: "documentURL required"}, nil
	}
	params := tu.Document(tu.ID(chatID), tu.FileFromURL(url))
	if c := getStr(p, "caption"); c != "" {
		params = params.WithCaption(c).WithParseMode(telego.ModeHTML)
	}
	msg, err := telegramBot.SendDocument(ctx, params)
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("sendDocument: %v", err)}, nil
	}
	return okResult(fmt.Sprintf("Document sent (message %d)", msg.MessageID)), nil
}

func tgSendPoll(ctx context.Context, chatID int64, p map[string]any) (*tool.ToolResult, error) {
	question := getStr(p, "question")
	if question == "" {
		return &tool.ToolResult{Error: "question required"}, nil
	}
	optionsRaw, ok := p["options"].([]any)
	if !ok || len(optionsRaw) < 2 {
		return &tool.ToolResult{Error: "options (array, min 2) required"}, nil
	}
	var options []telego.InputPollOption
	for _, o := range optionsRaw {
		if s, ok := o.(string); ok {
			options = append(options, telego.InputPollOption{Text: s})
		}
	}
	params := &telego.SendPollParams{
		ChatID:   tu.ID(chatID),
		Question: question,
		Options:  options,
	}
	if getBool(p, "isAnonymous") {
		params.IsAnonymous = boolPtr(true)
	}
	if getStr(p, "type") == "quiz" {
		params.Type = "quiz"
		cid := getInt(p, "correctOptionID")
		params.CorrectOptionID = &cid
	}
	msg, err := telegramBot.SendPoll(ctx, params)
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("sendPoll: %v", err)}, nil
	}
	return okResult(fmt.Sprintf("Poll sent (message %d)", msg.MessageID)), nil
}

func tgSendInvoice(ctx context.Context, chatID int64, p map[string]any) (*tool.ToolResult, error) {
	title := getStr(p, "title")
	desc := getStr(p, "description")
	currency := getStr(p, "currency")
	if title == "" || desc == "" || currency == "" {
		return &tool.ToolResult{Error: "title, description, currency required"}, nil
	}
	payload := getStr(p, "payload")
	if payload == "" {
		payload = "inv_" + title
	}
	pricesRaw, ok := p["prices"].([]any)
	if !ok || len(pricesRaw) == 0 {
		return &tool.ToolResult{Error: "prices [{label,amount}] required"}, nil
	}
	var prices []telego.LabeledPrice
	for _, pr := range pricesRaw {
		pm, ok := pr.(map[string]any)
		if !ok {
			continue
		}
		prices = append(prices, telego.LabeledPrice{Label: getStr(pm, "label"), Amount: getInt(pm, "amount")})
	}
	params := &telego.SendInvoiceParams{
		ChatID:        tu.ID(chatID),
		Title:         title,
		Description:   desc,
		Payload:       payload,
		Currency:      currency,
		Prices:        prices,
		ProviderToken: getStr(p, "providerToken"),
	}
	msg, err := telegramBot.SendInvoice(ctx, params)
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("sendInvoice: %v", err)}, nil
	}
	return okResult(fmt.Sprintf("Invoice sent (message %d)", msg.MessageID)), nil
}

func tgSendSticker(ctx context.Context, chatID int64, p map[string]any) (*tool.ToolResult, error) {
	url := getStr(p, "stickerURL")
	if url == "" {
		return &tool.ToolResult{Error: "stickerURL required"}, nil
	}
	msg, err := telegramBot.SendSticker(ctx, tu.Sticker(tu.ID(chatID), tu.FileFromURL(url)))
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("sendSticker: %v", err)}, nil
	}
	return okResult(fmt.Sprintf("Sticker sent (message %d)", msg.MessageID)), nil
}

func tgPinMessage(ctx context.Context, chatID int64, p map[string]any) (*tool.ToolResult, error) {
	msgID := getInt(p, "messageID")
	if msgID == 0 {
		return &tool.ToolResult{Error: "messageID required"}, nil
	}
	err := telegramBot.PinChatMessage(ctx, &telego.PinChatMessageParams{ChatID: tu.ID(chatID), MessageID: msgID})
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("pinMessage: %v", err)}, nil
	}
	return okResult(fmt.Sprintf("Message %d pinned", msgID)), nil
}

func tgUnpinMessage(ctx context.Context, chatID int64, p map[string]any) (*tool.ToolResult, error) {
	msgID := getInt(p, "messageID")
	if msgID == 0 {
		err := telegramBot.UnpinAllChatMessages(ctx, &telego.UnpinAllChatMessagesParams{ChatID: tu.ID(chatID)})
		if err != nil {
			return &tool.ToolResult{Error: fmt.Sprintf("unpinAll: %v", err)}, nil
		}
		return okResult("All messages unpinned"), nil
	}
	err := telegramBot.UnpinChatMessage(ctx, &telego.UnpinChatMessageParams{ChatID: tu.ID(chatID), MessageID: msgID})
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("unpinMessage: %v", err)}, nil
	}
	return okResult(fmt.Sprintf("Message %d unpinned", msgID)), nil
}

func tgSetCommands(ctx context.Context, p map[string]any) (*tool.ToolResult, error) {
	cmdsRaw, ok := p["commands"].([]any)
	if !ok || len(cmdsRaw) == 0 {
		return &tool.ToolResult{Error: "commands [{command,description}] required"}, nil
	}
	var cmds []telego.BotCommand
	for _, c := range cmdsRaw {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		cmds = append(cmds, telego.BotCommand{Command: getStr(cm, "command"), Description: getStr(cm, "description")})
	}
	err := telegramBot.SetMyCommands(ctx, &telego.SetMyCommandsParams{Commands: cmds})
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("setCommands: %v", err)}, nil
	}
	names := make([]string, len(cmds))
	for i, c := range cmds {
		names[i] = "/" + c.Command
	}
	return okResult(fmt.Sprintf("Commands set: %s", strings.Join(names, ", "))), nil
}

func tgDeleteCommands(ctx context.Context) (*tool.ToolResult, error) {
	err := telegramBot.DeleteMyCommands(ctx, nil)
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("deleteCommands: %v", err)}, nil
	}
	return okResult("Bot commands deleted"), nil
}

func tgGetCommands(ctx context.Context) (*tool.ToolResult, error) {
	cmds, err := telegramBot.GetMyCommands(ctx, nil)
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("getCommands: %v", err)}, nil
	}
	return jsonResult(cmds), nil
}

func tgCreateInviteLink(ctx context.Context, chatID int64, p map[string]any) (*tool.ToolResult, error) {
	params := &telego.CreateChatInviteLinkParams{ChatID: tu.ID(chatID)}
	if name := getStr(p, "name"); name != "" {
		params.Name = name
	}
	if limit := getInt(p, "memberLimit"); limit > 0 {
		params.MemberLimit = limit
	}
	if exp := getInt64(p, "expireDate"); exp > 0 {
		params.ExpireDate = exp
	}
	link, err := telegramBot.CreateChatInviteLink(ctx, params)
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("createInviteLink: %v", err)}, nil
	}
	return jsonResult(map[string]string{"inviteLink": link.InviteLink, "name": link.Name}), nil
}

func tgSetMenuButton(ctx context.Context, chatID int64, p map[string]any) (*tool.ToolResult, error) {
	text := getStr(p, "text")
	url := getStr(p, "url")
	if text == "" || url == "" {
		return &tool.ToolResult{Error: "text and url required"}, nil
	}
	err := telegramBot.SetChatMenuButton(ctx, &telego.SetChatMenuButtonParams{
		ChatID:     chatID,
		MenuButton: &telego.MenuButtonWebApp{Type: "web_app", Text: text, WebApp: telego.WebAppInfo{URL: url}},
	})
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("setMenuButton: %v", err)}, nil
	}
	return okResult(fmt.Sprintf("Menu button set: %s -> %s", text, url)), nil
}

func tgPromoteMember(ctx context.Context, chatID int64, p map[string]any) (*tool.ToolResult, error) {
	userID := getInt64(p, "userID")
	if userID == 0 {
		return &tool.ToolResult{Error: "userID required"}, nil
	}
	perms, _ := p["permissions"].(map[string]any)
	params := &telego.PromoteChatMemberParams{
		ChatID: tu.ID(chatID),
		UserID: userID,
	}
	if perms != nil {
		if getBool(perms, "canChangeInfo") {
			params.CanChangeInfo = boolPtr(true)
		}
		if getBool(perms, "canPostMessages") {
			params.CanPostMessages = boolPtr(true)
		}
		if getBool(perms, "canEditMessages") {
			params.CanEditMessages = boolPtr(true)
		}
		if getBool(perms, "canDeleteMessages") {
			params.CanDeleteMessages = boolPtr(true)
		}
		if getBool(perms, "canInviteUsers") {
			params.CanInviteUsers = boolPtr(true)
		}
		if getBool(perms, "canRestrictMembers") {
			params.CanRestrictMembers = boolPtr(true)
		}
		if getBool(perms, "canPinMessages") {
			params.CanPinMessages = boolPtr(true)
		}
		if getBool(perms, "canPromoteMembers") {
			params.CanPromoteMembers = boolPtr(true)
		}
		if getBool(perms, "canManageChat") {
			params.CanManageChat = boolPtr(true)
		}
		if getBool(perms, "canManageTopics") {
			params.CanManageTopics = boolPtr(true)
		}
	}
	err := telegramBot.PromoteChatMember(ctx, params)
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("promoteMember: %v", err)}, nil
	}
	return okResult(fmt.Sprintf("User %d promoted", userID)), nil
}

func tgRestrictMember(ctx context.Context, chatID int64, p map[string]any) (*tool.ToolResult, error) {
	userID := getInt64(p, "userID")
	if userID == 0 {
		return &tool.ToolResult{Error: "userID required"}, nil
	}
	perms, _ := p["permissions"].(map[string]any)
	cp := telego.ChatPermissions{}
	if perms != nil {
		if getBool(perms, "canSendMessages") {
			cp.CanSendMessages = boolPtr(true)
		}
		if getBool(perms, "canSendPhotos") {
			cp.CanSendPhotos = boolPtr(true)
		}
		if getBool(perms, "canSendVideos") {
			cp.CanSendVideos = boolPtr(true)
		}
		if getBool(perms, "canSendDocuments") {
			cp.CanSendDocuments = boolPtr(true)
		}
		if getBool(perms, "canSendAudios") {
			cp.CanSendAudios = boolPtr(true)
		}
		if getBool(perms, "canSendPolls") {
			cp.CanSendPolls = boolPtr(true)
		}
		if getBool(perms, "canSendOtherMessages") {
			cp.CanSendOtherMessages = boolPtr(true)
		}
		if getBool(perms, "canAddWebPagePreviews") {
			cp.CanAddWebPagePreviews = boolPtr(true)
		}
		if getBool(perms, "canChangeInfo") {
			cp.CanChangeInfo = boolPtr(true)
		}
		if getBool(perms, "canInviteUsers") {
			cp.CanInviteUsers = boolPtr(true)
		}
		if getBool(perms, "canPinMessages") {
			cp.CanPinMessages = boolPtr(true)
		}
		if getBool(perms, "canManageTopics") {
			cp.CanManageTopics = boolPtr(true)
		}
	}
	params := &telego.RestrictChatMemberParams{
		ChatID:      tu.ID(chatID),
		UserID:      userID,
		Permissions: cp,
	}
	if until := getInt64(p, "untilDate"); until > 0 {
		params.UntilDate = until
	}
	err := telegramBot.RestrictChatMember(ctx, params)
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("restrictMember: %v", err)}, nil
	}
	return okResult(fmt.Sprintf("User %d restricted", userID)), nil
}

func tgBanMember(ctx context.Context, chatID int64, p map[string]any) (*tool.ToolResult, error) {
	userID := getInt64(p, "userID")
	if userID == 0 {
		return &tool.ToolResult{Error: "userID required"}, nil
	}
	params := &telego.BanChatMemberParams{ChatID: tu.ID(chatID), UserID: userID}
	if until := getInt64(p, "untilDate"); until > 0 {
		params.UntilDate = until
	}
	err := telegramBot.BanChatMember(ctx, params)
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("banMember: %v", err)}, nil
	}
	return okResult(fmt.Sprintf("User %d banned", userID)), nil
}

func tgUnbanMember(ctx context.Context, chatID int64, p map[string]any) (*tool.ToolResult, error) {
	userID := getInt64(p, "userID")
	if userID == 0 {
		return &tool.ToolResult{Error: "userID required"}, nil
	}
	err := telegramBot.UnbanChatMember(ctx, &telego.UnbanChatMemberParams{ChatID: tu.ID(chatID), UserID: userID, OnlyIfBanned: true})
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("unbanMember: %v", err)}, nil
	}
	return okResult(fmt.Sprintf("User %d unbanned", userID)), nil
}

func tgCreateForumTopic(ctx context.Context, chatID int64, p map[string]any) (*tool.ToolResult, error) {
	name := getStr(p, "name")
	if name == "" {
		return &tool.ToolResult{Error: "name required"}, nil
	}
	params := &telego.CreateForumTopicParams{ChatID: tu.ID(chatID), Name: name}
	if color := getInt(p, "iconColor"); color > 0 {
		params.IconColor = color
	}
	topic, err := telegramBot.CreateForumTopic(ctx, params)
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("createForumTopic: %v", err)}, nil
	}
	return jsonResult(map[string]any{"topicID": topic.MessageThreadID, "name": topic.Name}), nil
}

func tgCloseForumTopic(ctx context.Context, chatID int64, p map[string]any) (*tool.ToolResult, error) {
	topicID := getInt(p, "topicID")
	if topicID == 0 {
		return &tool.ToolResult{Error: "topicID required"}, nil
	}
	err := telegramBot.CloseForumTopic(ctx, &telego.CloseForumTopicParams{ChatID: tu.ID(chatID), MessageThreadID: topicID})
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("closeForumTopic: %v", err)}, nil
	}
	return okResult(fmt.Sprintf("Topic %d closed", topicID)), nil
}

func tgGetChat(ctx context.Context, chatID int64) (*tool.ToolResult, error) {
	chat, err := telegramBot.GetChat(ctx, &telego.GetChatParams{ChatID: tu.ID(chatID)})
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("getChat: %v", err)}, nil
	}
	return jsonResult(map[string]any{"id": chat.ID, "type": chat.Type, "title": chat.Title, "username": chat.Username, "description": chat.Description}), nil
}

func tgGetMemberCount(ctx context.Context, chatID int64) (*tool.ToolResult, error) {
	count, err := telegramBot.GetChatMemberCount(ctx, &telego.GetChatMemberCountParams{ChatID: tu.ID(chatID)})
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("getMemberCount: %v", err)}, nil
	}
	return okResult(fmt.Sprintf("%d members", count)), nil
}

func tgEditMessage(ctx context.Context, chatID int64, p map[string]any) (*tool.ToolResult, error) {
	msgID := getInt(p, "messageID")
	text := getStr(p, "text")
	if msgID == 0 || text == "" {
		return &tool.ToolResult{Error: "messageID and text required"}, nil
	}
	_, err := telegramBot.EditMessageText(ctx, &telego.EditMessageTextParams{ChatID: tu.ID(chatID), MessageID: msgID, Text: text, ParseMode: telego.ModeHTML})
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("editMessage: %v", err)}, nil
	}
	return okResult(fmt.Sprintf("Message %d edited", msgID)), nil
}

func tgDeleteMessage(ctx context.Context, chatID int64, p map[string]any) (*tool.ToolResult, error) {
	msgID := getInt(p, "messageID")
	if msgID == 0 {
		return &tool.ToolResult{Error: "messageID required"}, nil
	}
	err := telegramBot.DeleteMessage(ctx, &telego.DeleteMessageParams{ChatID: tu.ID(chatID), MessageID: msgID})
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("deleteMessage: %v", err)}, nil
	}
	return okResult(fmt.Sprintf("Message %d deleted", msgID)), nil
}

func tgForwardMessage(ctx context.Context, chatID int64, p map[string]any) (*tool.ToolResult, error) {
	fromChatID := getInt64(p, "fromChatID")
	msgID := getInt(p, "messageID")
	if fromChatID == 0 || msgID == 0 {
		return &tool.ToolResult{Error: "fromChatID and messageID required"}, nil
	}
	msg, err := telegramBot.ForwardMessage(ctx, &telego.ForwardMessageParams{ChatID: tu.ID(chatID), FromChatID: tu.ID(fromChatID), MessageID: msgID})
	if err != nil {
		return &tool.ToolResult{Error: fmt.Sprintf("forwardMessage: %v", err)}, nil
	}
	return okResult(fmt.Sprintf("Forwarded (new ID: %d)", msg.MessageID)), nil
}
