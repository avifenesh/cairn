// Package channel provides a multi-channel messaging system for Cairn.
// Each channel adapter (Telegram, Discord, Slack, Matrix) implements the
// Channel interface and handles platform-specific message formatting.
package channel

import (
	"context"
	"log/slog"
)

// Channel is the interface every channel adapter implements.
type Channel interface {
	// Name returns the channel identifier (e.g. "telegram", "discord").
	Name() string
	// Start connects to the platform and begins listening for messages.
	// Blocks until ctx is cancelled or a fatal error occurs.
	Start(ctx context.Context) error
	// Send delivers a message to the channel's configured destination.
	Send(ctx context.Context, msg *OutgoingMessage) error
	// Close disconnects from the platform and cleans up resources.
	Close() error
}

// MessageHandler processes an incoming message and returns a response.
// Set by the agent wiring layer — bridges channel messages to the agent loop.
type MessageHandler func(ctx context.Context, msg *IncomingMessage) (*OutgoingMessage, error)

// Router dispatches incoming messages to the agent and routes responses
// back to the originating channel. Manages channel lifecycle.
type Router struct {
	channels map[string]Channel
	handler  MessageHandler
	logger   *slog.Logger
}

// NewRouter creates a channel router with the given message handler.
func NewRouter(handler MessageHandler, logger *slog.Logger) *Router {
	if logger == nil {
		logger = slog.Default()
	}
	return &Router{
		channels: make(map[string]Channel),
		handler:  handler,
		logger:   logger,
	}
}

// Register adds a channel to the router.
func (r *Router) Register(ch Channel) {
	r.channels[ch.Name()] = ch
	r.logger.Info("channel registered", "channel", ch.Name())
}

// Start launches all registered channels in goroutines.
// Blocks until ctx is cancelled.
func (r *Router) Start(ctx context.Context) error {
	if len(r.channels) == 0 {
		r.logger.Info("no channels registered, router idle")
		<-ctx.Done()
		return ctx.Err()
	}

	errCh := make(chan error, len(r.channels))

	for _, ch := range r.channels {
		go func(c Channel) {
			r.logger.Info("channel starting", "channel", c.Name())
			if err := c.Start(ctx); err != nil && ctx.Err() == nil {
				r.logger.Error("channel error", "channel", c.Name(), "error", err)
				errCh <- err
			}
		}(ch)
	}

	select {
	case <-ctx.Done():
		r.Close()
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// Broadcast sends a message to all registered channels.
func (r *Router) Broadcast(ctx context.Context, msg *OutgoingMessage) {
	for _, ch := range r.channels {
		if err := ch.Send(ctx, msg); err != nil {
			r.logger.Error("broadcast send failed", "channel", ch.Name(), "error", err)
		}
	}
}

// SendTo sends a message to a specific channel by name.
func (r *Router) SendTo(ctx context.Context, channelName string, msg *OutgoingMessage) error {
	ch, ok := r.channels[channelName]
	if !ok {
		return nil // channel not registered, skip silently
	}
	return ch.Send(ctx, msg)
}

// Close stops all registered channels.
func (r *Router) Close() {
	for _, ch := range r.channels {
		if err := ch.Close(); err != nil {
			r.logger.Error("channel close error", "channel", ch.Name(), "error", err)
		}
	}
}

// Channels returns the names of all registered channels.
func (r *Router) Channels() []string {
	names := make([]string, 0, len(r.channels))
	for name := range r.channels {
		names = append(names, name)
	}
	return names
}
