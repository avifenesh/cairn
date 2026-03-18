package channel

import (
	"context"
	"sync"
	"testing"
)

// mockChannel implements Channel for testing.
type mockChannel struct {
	name     string
	started  bool
	sent     []*OutgoingMessage
	mu       sync.Mutex
	startErr error
}

func (m *mockChannel) Name() string { return m.name }
func (m *mockChannel) Start(ctx context.Context) error {
	m.started = true
	if m.startErr != nil {
		return m.startErr
	}
	<-ctx.Done()
	return ctx.Err()
}
func (m *mockChannel) Send(_ context.Context, msg *OutgoingMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}
func (m *mockChannel) Close() error { return nil }

func TestRouterRegister(t *testing.T) {
	handler := func(_ context.Context, _ *IncomingMessage) (*OutgoingMessage, error) {
		return &OutgoingMessage{Text: "ok"}, nil
	}
	r := NewRouter(handler, nil)
	r.Register(&mockChannel{name: "test"})

	names := r.Channels()
	if len(names) != 1 || names[0] != "test" {
		t.Fatalf("expected [test], got %v", names)
	}
}

func TestRouterBroadcast(t *testing.T) {
	handler := func(_ context.Context, _ *IncomingMessage) (*OutgoingMessage, error) {
		return nil, nil
	}
	r := NewRouter(handler, nil)

	ch1 := &mockChannel{name: "ch1"}
	ch2 := &mockChannel{name: "ch2"}
	r.Register(ch1)
	r.Register(ch2)

	msg := &OutgoingMessage{Text: "hello"}
	r.Broadcast(context.Background(), msg)

	if len(ch1.sent) != 1 {
		t.Fatalf("ch1: expected 1 message, got %d", len(ch1.sent))
	}
	if len(ch2.sent) != 1 {
		t.Fatalf("ch2: expected 1 message, got %d", len(ch2.sent))
	}
}

func TestRouterSendTo(t *testing.T) {
	handler := func(_ context.Context, _ *IncomingMessage) (*OutgoingMessage, error) {
		return nil, nil
	}
	r := NewRouter(handler, nil)

	ch1 := &mockChannel{name: "telegram"}
	r.Register(ch1)

	msg := &OutgoingMessage{Text: "hello"}
	r.SendTo(context.Background(), "telegram", msg)
	r.SendTo(context.Background(), "nonexistent", msg) // should not error

	if len(ch1.sent) != 1 {
		t.Fatalf("expected 1 message sent to telegram, got %d", len(ch1.sent))
	}
}

func TestTelegramParseMessage(t *testing.T) {
	// Test command parsing directly.
	in := &IncomingMessage{
		Text: "/status hello world",
	}

	// Simulate command parsing logic.
	if len(in.Text) > 0 && in.Text[0] == '/' {
		in.IsCommand = true
		parts := splitCommand(in.Text)
		in.Command = parts[0]
		in.Args = parts[1]
	}

	if !in.IsCommand {
		t.Fatal("expected command")
	}
	if in.Command != "status" {
		t.Fatalf("expected command 'status', got %q", in.Command)
	}
	if in.Args != "hello world" {
		t.Fatalf("expected args 'hello world', got %q", in.Args)
	}
}

func splitCommand(text string) [2]string {
	if len(text) == 0 || text[0] != '/' {
		return [2]string{text, ""}
	}
	cmd := text[1:]
	args := ""
	for i, c := range cmd {
		if c == ' ' {
			args = cmd[i+1:]
			cmd = cmd[:i]
			break
		}
	}
	return [2]string{cmd, args}
}
