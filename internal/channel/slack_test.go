package channel

import (
	"testing"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

func TestSlackParseMessage(t *testing.T) {
	ev := &slackevents.MessageEvent{
		User:      "U123",
		Channel:   "C456",
		Text:      "hello world",
		TimeStamp: "1234567890.123456",
	}

	in := parseSlackMessage(ev)
	if in.ID != "sl_1234567890.123456" {
		t.Fatalf("expected ID sl_1234567890.123456, got %s", in.ID)
	}
	if in.ChannelID != "slack" {
		t.Fatalf("expected channelID slack, got %s", in.ChannelID)
	}
	if in.UserID != "U123" {
		t.Fatalf("expected userID U123, got %s", in.UserID)
	}
	if in.ChatID != "C456" {
		t.Fatalf("expected chatID C456, got %s", in.ChatID)
	}
	if in.Text != "hello world" {
		t.Fatalf("expected text 'hello world', got %q", in.Text)
	}
	if in.IsCommand {
		t.Fatal("expected not a command")
	}
}

func TestSlackParseCommand(t *testing.T) {
	ev := &slackevents.MessageEvent{
		User:      "U123",
		Channel:   "C456",
		Text:      "/digest last 24h",
		TimeStamp: "1234567890.000001",
	}

	in := parseSlackMessage(ev)
	if !in.IsCommand {
		t.Fatal("expected command")
	}
	if in.Command != "digest" {
		t.Fatalf("expected command 'digest', got %q", in.Command)
	}
	if in.Args != "last 24h" {
		t.Fatalf("expected args 'last 24h', got %q", in.Args)
	}
}

func TestSlackParseThread(t *testing.T) {
	ev := &slackevents.MessageEvent{
		User:            "U123",
		Channel:         "C456",
		Text:            "reply",
		TimeStamp:       "1234567890.000002",
		ThreadTimeStamp: "1234567890.000001",
	}

	in := parseSlackMessage(ev)
	if in.Metadata["threadTs"] != "1234567890.000001" {
		t.Fatalf("expected threadTs, got %q", in.Metadata["threadTs"])
	}
}

func TestSlackBuildBlocks(t *testing.T) {
	actions := []ActionGroup{
		{
			Actions: []Action{
				{ID: "approve", Label: "Approve", Style: "primary"},
				{ID: "deny", Label: "Deny", Style: "danger"},
			},
		},
	}

	blocks := buildSlackBlocks("Test message", actions)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks (section + actions), got %d", len(blocks))
	}

	// First block is section with text.
	section, ok := blocks[0].(*slack.SectionBlock)
	if !ok {
		t.Fatalf("expected SectionBlock, got %T", blocks[0])
	}
	if section.Text.Text != "Test message" {
		t.Fatalf("expected text 'Test message', got %q", section.Text.Text)
	}

	// Second block is actions with buttons.
	actionBlock, ok := blocks[1].(*slack.ActionBlock)
	if !ok {
		t.Fatalf("expected ActionBlock, got %T", blocks[1])
	}
	if len(actionBlock.Elements.ElementSet) != 2 {
		t.Fatalf("expected 2 buttons, got %d", len(actionBlock.Elements.ElementSet))
	}
}

func TestSlackBuildBlocksNoActions(t *testing.T) {
	blocks := buildSlackBlocks("Just text", nil)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block (text only), got %d", len(blocks))
	}
}
