package signal

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// WebhookHandler receives HTTP POST webhooks, verifies signatures, and ingests
// events into the event store.
type WebhookHandler struct {
	store   *EventStore
	secrets map[string]string // name -> secret for HMAC verification
}

// NewWebhookHandler creates a webhook handler. secrets maps webhook names to
// their HMAC-SHA256 secrets. Webhooks without a configured secret are rejected.
func NewWebhookHandler(store *EventStore, secrets map[string]string) *WebhookHandler {
	return &WebhookHandler{
		store:   store,
		secrets: secrets,
	}
}

// ServeHTTP handles POST /v1/webhooks/{name}. Verifies the signature, parses
// the payload, and ingests as a signal event.
func (wh *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "webhook name required", http.StatusBadRequest)
		return
	}

	secret, ok := wh.secrets[name]
	if !ok {
		http.Error(w, "unknown webhook", http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	// Verify signature.
	if secret != "" {
		if !wh.verifySignature(r, body, secret) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	// Parse webhook payload.
	ev, err := wh.parsePayload(name, r, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Ingest event.
	if _, err := wh.store.Ingest(context.Background(), []*RawEvent{ev}); err != nil {
		http.Error(w, "ingest error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"ok":true}`))
}

// verifySignature checks the webhook signature against the configured secret.
// Supports GitHub (X-Hub-Signature-256) and generic (X-Signature) headers.
func (wh *WebhookHandler) verifySignature(r *http.Request, body []byte, secret string) bool {
	// GitHub: X-Hub-Signature-256: sha256=<hex>
	if sig := r.Header.Get("X-Hub-Signature-256"); sig != "" {
		return verifyHMACSHA256(body, secret, strings.TrimPrefix(sig, "sha256="))
	}
	// Generic: X-Signature: <hex>
	if sig := r.Header.Get("X-Signature"); sig != "" {
		return verifyHMACSHA256(body, secret, sig)
	}
	// Token-based: ?token=<secret>
	if token := r.URL.Query().Get("token"); token != "" {
		return token == secret
	}
	return false
}

func verifyHMACSHA256(body []byte, secret, expectedHex string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected, err := hex.DecodeString(expectedHex)
	if err != nil {
		return false
	}
	return hmac.Equal(mac.Sum(nil), expected)
}

// parsePayload extracts a RawEvent from the webhook request.
func (wh *WebhookHandler) parsePayload(name string, r *http.Request, body []byte) (*RawEvent, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid JSON payload")
	}

	title := stringFromMap(payload, "title", "summary", "subject", "message")
	if title == "" {
		title = fmt.Sprintf("Webhook: %s", name)
	}

	url := stringFromMap(payload, "url", "html_url", "link")
	actor := stringFromMap(payload, "sender.login", "actor", "user", "from")

	return &RawEvent{
		Source:     SourceWebhook,
		SourceID:   fmt.Sprintf("wh:%s:%d", name, time.Now().UnixNano()),
		Kind:       KindWebhook,
		Title:      title,
		URL:        url,
		Actor:      actor,
		GroupKey:   fmt.Sprintf("webhook:%s", name),
		Metadata:   payload,
		OccurredAt: time.Now().UTC(),
	}, nil
}

// stringFromMap tries multiple keys and returns the first non-empty string value found.
func stringFromMap(m map[string]any, keys ...string) string {
	for _, key := range keys {
		// Handle dotted paths like "sender.login".
		parts := strings.SplitN(key, ".", 2)
		if len(parts) == 2 {
			if nested, ok := m[parts[0]].(map[string]any); ok {
				if v, ok := nested[parts[1]].(string); ok && v != "" {
					return v
				}
			}
			continue
		}
		if v, ok := m[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
