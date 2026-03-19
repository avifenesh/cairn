package channel

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// NotifyConfig controls how notifications are routed across channels.
type NotifyConfig struct {
	PreferredChannel string // "telegram", "discord", "slack"
	QuietHoursStart  int    // Hour (0-23), -1 = disabled
	QuietHoursEnd    int    // Hour (0-23), -1 = disabled
	QuietHoursTZ     string // IANA timezone (e.g. "Asia/Jerusalem"), empty = UTC
}

// isQuietHours returns true if the current time falls within quiet hours.
// Handles midnight wrap-around (e.g. 22:00 → 08:00).
// Returns false if quiet hours are disabled (start or end < 0).
func (c *NotifyConfig) isQuietHours() bool {
	if c == nil || c.QuietHoursStart < 0 || c.QuietHoursEnd < 0 {
		return false
	}

	loc := time.UTC
	if c.QuietHoursTZ != "" {
		if l, err := time.LoadLocation(c.QuietHoursTZ); err == nil {
			loc = l
		}
	}

	hour := time.Now().In(loc).Hour()

	if c.QuietHoursStart <= c.QuietHoursEnd {
		// Same-day range: e.g. 1:00 → 6:00
		return hour >= c.QuietHoursStart && hour < c.QuietHoursEnd
	}
	// Midnight wrap: e.g. 22:00 → 08:00
	return hour >= c.QuietHoursStart || hour < c.QuietHoursEnd
}

// SetNotifyConfig sets the notification routing configuration.
func (r *Router) SetNotifyConfig(cfg *NotifyConfig) {
	r.notifyCfg = cfg
}

// Notify routes a message based on its Priority and the NotifyConfig.
//
// Routing rules:
//
//	Critical → Broadcast to all channels (always)
//	High     → Preferred channel (bypasses quiet hours)
//	Medium   → Preferred channel (queued during quiet hours)
//	Low      → Always queued for digest
//
// If no preferred channel is configured, High/Medium broadcast to all.
// If no channels are registered, messages are silently dropped (logged).
func (r *Router) Notify(ctx context.Context, msg *OutgoingMessage) {
	if len(r.channels) == 0 {
		r.logger.Warn("notify: no channels registered, dropping message")
		return
	}

	cfg := r.notifyCfg
	quiet := cfg != nil && cfg.isQuietHours()

	switch msg.Priority {
	case PriorityCritical:
		r.Broadcast(ctx, msg)
		r.logger.Info("notify: critical broadcast", "channels", len(r.channels))

	case PriorityHigh:
		// Bypass quiet hours, send to preferred or broadcast.
		r.sendPreferredOrBroadcast(ctx, msg)
		r.logger.Info("notify: high priority sent", "quiet", quiet)

	case PriorityMedium:
		if quiet {
			r.enqueueDigest(msg)
			r.logger.Info("notify: medium queued (quiet hours)")
		} else {
			r.sendPreferredOrBroadcast(ctx, msg)
			r.logger.Info("notify: medium sent")
		}

	case PriorityLow:
		r.enqueueDigest(msg)
		r.logger.Info("notify: low queued for digest")

	default:
		// Unknown priority — treat as medium.
		r.sendPreferredOrBroadcast(ctx, msg)
	}
}

// sendPreferredOrBroadcast sends to the preferred channel if configured and
// registered, otherwise broadcasts to all channels.
func (r *Router) sendPreferredOrBroadcast(ctx context.Context, msg *OutgoingMessage) {
	if r.notifyCfg != nil && r.notifyCfg.PreferredChannel != "" {
		if _, ok := r.channels[r.notifyCfg.PreferredChannel]; ok {
			if err := r.SendTo(ctx, r.notifyCfg.PreferredChannel, msg); err != nil {
				r.logger.Error("notify: preferred channel send failed, broadcasting",
					"channel", r.notifyCfg.PreferredChannel, "error", err)
				r.Broadcast(ctx, msg)
			}
			return
		}
		r.logger.Warn("notify: preferred channel not registered, broadcasting",
			"preferred", r.notifyCfg.PreferredChannel)
	}
	r.Broadcast(ctx, msg)
}

// enqueueDigest adds a message to the digest queue.
func (r *Router) enqueueDigest(msg *OutgoingMessage) {
	r.digest.enqueue(msg.Text, msg.Priority)
}

// FlushDigest sends all queued digest items as a single message to the
// preferred (or digest) channel. Returns the number of items flushed.
func (r *Router) FlushDigest(ctx context.Context) int {
	items := r.digest.flush()
	if len(items) == 0 {
		return 0
	}

	// Build digest message.
	var b strings.Builder
	fmt.Fprintf(&b, "## Digest (%d items)\n\n", len(items))
	for _, item := range items {
		prefix := ""
		switch item.priority {
		case PriorityMedium:
			prefix = "[medium] "
		case PriorityLow:
			prefix = "[low] "
		}
		fmt.Fprintf(&b, "- %s%s\n", prefix, item.text)
	}

	msg := &OutgoingMessage{
		Text:     b.String(),
		Priority: PriorityMedium, // digest itself is medium
	}

	// Send to digest channel, preferred, or broadcast.
	target := ""
	if r.notifyCfg != nil {
		target = r.notifyCfg.PreferredChannel
	}
	if target != "" {
		if err := r.SendTo(ctx, target, msg); err != nil {
			r.Broadcast(ctx, msg)
		}
	} else {
		r.Broadcast(ctx, msg)
	}

	r.logger.Info("notify: digest flushed", "items", len(items))
	return len(items)
}

// DigestLen returns the number of messages waiting in the digest queue.
func (r *Router) DigestLen() int {
	return r.digest.len()
}

// digestQueue holds low-priority messages for batch delivery.
type digestQueue struct {
	mu    sync.Mutex
	items []digestItem
}

type digestItem struct {
	text     string
	priority Priority
	queuedAt time.Time
}

func (q *digestQueue) enqueue(text string, priority Priority) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, digestItem{
		text:     text,
		priority: priority,
		queuedAt: time.Now(),
	})
}

func (q *digestQueue) flush() []digestItem {
	q.mu.Lock()
	defer q.mu.Unlock()
	items := q.items
	q.items = nil
	return items
}

func (q *digestQueue) len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}
