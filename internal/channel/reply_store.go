package channel

import (
	"sync"
	"time"
)

// ReplyStore maps outgoing platform message IDs to their content text,
// so when a user replies to a bot message the original context can be
// looked up and injected. Entries expire after a configurable TTL.
type ReplyStore struct {
	mu      sync.RWMutex
	store   map[string]replyEntry
	ttl     time.Duration
	cleanup time.Duration
	done    chan struct{}
}

type replyEntry struct {
	content   string
	createdAt time.Time
}

// NewReplyStore creates a reply context store with the given TTL.
// Entries older than TTL are automatically cleaned up. Pass 0 for
// default (24h TTL, 1h cleanup interval).
func NewReplyStore(ttl time.Duration) *ReplyStore {
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	rs := &ReplyStore{
		store:   make(map[string]replyEntry),
		ttl:     ttl,
		cleanup: 1 * time.Hour,
		done:    make(chan struct{}),
	}
	go rs.cleanLoop()
	return rs
}

// key builds a composite key: channel:chatID:messageID.
func key(channel, chatID, messageID string) string {
	return channel + ":" + chatID + ":" + messageID
}

// Save stores the content of an outgoing message for later reply lookup.
func (rs *ReplyStore) Save(channel, chatID, messageID, content string) {
	if messageID == "" {
		return
	}
	rs.mu.Lock()
	rs.store[key(channel, chatID, messageID)] = replyEntry{
		content:   content,
		createdAt: time.Now(),
	}
	rs.mu.Unlock()
}

// Lookup retrieves the original message content for a reply-to reference.
// Returns empty string if not found or expired.
func (rs *ReplyStore) Lookup(channel, chatID, messageID string) string {
	rs.mu.RLock()
	entry, ok := rs.store[key(channel, chatID, messageID)]
	rs.mu.RUnlock()
	if !ok || time.Since(entry.createdAt) > rs.ttl {
		return ""
	}
	return entry.content
}

// Close stops the background cleanup goroutine.
func (rs *ReplyStore) Close() {
	select {
	case <-rs.done:
	default:
		close(rs.done)
	}
}

func (rs *ReplyStore) cleanLoop() {
	ticker := time.NewTicker(rs.cleanup)
	defer ticker.Stop()
	for {
		select {
		case <-rs.done:
			return
		case <-ticker.C:
			rs.clean()
		}
	}
}

func (rs *ReplyStore) clean() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	now := time.Now()
	for k, e := range rs.store {
		if now.Sub(e.createdAt) > rs.ttl {
			delete(rs.store, k)
		}
	}
}
