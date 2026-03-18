package channel

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestDiscordParseMessage(t *testing.T) {
	m := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "123456789",
			ChannelID: "channel-1",
			Content:   "hello world",
			Author: &discordgo.User{
				ID:       "user-1",
				Username: "testuser",
			},
		},
	}

	in := parseDiscordMessage(m)
	if in.ID != "dc_123456789" {
		t.Fatalf("expected ID dc_123456789, got %s", in.ID)
	}
	if in.ChannelID != "discord" {
		t.Fatalf("expected channelID discord, got %s", in.ChannelID)
	}
	if in.UserID != "user-1" {
		t.Fatalf("expected userID user-1, got %s", in.UserID)
	}
	if in.ChatID != "channel-1" {
		t.Fatalf("expected chatID channel-1, got %s", in.ChatID)
	}
	if in.Text != "hello world" {
		t.Fatalf("expected text 'hello world', got %q", in.Text)
	}
	if in.IsCommand {
		t.Fatal("expected not a command")
	}
	if in.Metadata["username"] != "testuser" {
		t.Fatalf("expected username testuser, got %s", in.Metadata["username"])
	}
}

func TestDiscordParseCommand(t *testing.T) {
	m := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "2",
			ChannelID: "ch",
			Content:   "/status check now",
			Author:    &discordgo.User{ID: "u1", Username: "user"},
		},
	}

	in := parseDiscordMessage(m)
	if !in.IsCommand {
		t.Fatal("expected command")
	}
	if in.Command != "status" {
		t.Fatalf("expected command 'status', got %q", in.Command)
	}
	if in.Args != "check now" {
		t.Fatalf("expected args 'check now', got %q", in.Args)
	}
}

func TestDiscordParseCommandNoArgs(t *testing.T) {
	m := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "3",
			ChannelID: "ch",
			Content:   "/digest",
			Author:    &discordgo.User{ID: "u1", Username: "user"},
		},
	}

	in := parseDiscordMessage(m)
	if !in.IsCommand {
		t.Fatal("expected command")
	}
	if in.Command != "digest" {
		t.Fatalf("expected command 'digest', got %q", in.Command)
	}
	if in.Args != "" {
		t.Fatalf("expected empty args, got %q", in.Args)
	}
}

func TestDiscordBuildComponents(t *testing.T) {
	actions := []ActionGroup{
		{
			Label: "Approve?",
			Actions: []Action{
				{ID: "approve", Label: "Approve", Style: "primary"},
				{ID: "deny", Label: "Deny", Style: "danger"},
			},
		},
	}

	components := buildDiscordComponents(actions)
	if len(components) != 1 {
		t.Fatalf("expected 1 action row, got %d", len(components))
	}

	row, ok := components[0].(discordgo.ActionsRow)
	if !ok {
		t.Fatal("expected ActionsRow")
	}
	if len(row.Components) != 2 {
		t.Fatalf("expected 2 buttons, got %d", len(row.Components))
	}

	btn1, ok := row.Components[0].(discordgo.Button)
	if !ok {
		t.Fatal("expected Button")
	}
	if btn1.Label != "Approve" || btn1.CustomID != "approve" || btn1.Style != discordgo.PrimaryButton {
		t.Fatalf("unexpected button: %+v", btn1)
	}

	btn2, ok := row.Components[1].(discordgo.Button)
	if !ok {
		t.Fatal("expected Button")
	}
	if btn2.Label != "Deny" || btn2.CustomID != "deny" || btn2.Style != discordgo.DangerButton {
		t.Fatalf("unexpected button: %+v", btn2)
	}
}

func TestDiscordBuildComponentsEmpty(t *testing.T) {
	components := buildDiscordComponents(nil)
	if components != nil {
		t.Fatalf("expected nil components, got %v", components)
	}
}

func TestSplitMessage(t *testing.T) {
	// Short message — no split.
	chunks := splitMessage("hello", 2000)
	if len(chunks) != 1 || chunks[0] != "hello" {
		t.Fatalf("expected 1 chunk, got %v", chunks)
	}

	// Long message — split on newline.
	long := "line1\nline2\nline3"
	chunks = splitMessage(long, 10)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d: %v", len(chunks), chunks)
	}
	// Verify all content is preserved.
	joined := ""
	for _, c := range chunks {
		joined += c
	}
	if joined != long {
		t.Fatalf("content lost: expected %q, got %q", long, joined)
	}
}
