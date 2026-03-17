package signal

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
// their HMAC-SHA256 secrets. Every webhook must have a non-empty secret.
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
	if !ok || secret == "" {
		http.Error(w, "unknown webhook", http.StatusNotFound)
		return
	}

	// Use MaxBytesReader for proper 413 on oversized bodies.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB
	body, err := readAll(r.Body)
	if err != nil {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}

	// Verify signature - always required (secret is never empty here).
	if !wh.verifySignature(r, body, secret) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	// Parse webhook payload.
	ev, err := wh.parsePayload(name, r, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Ingest event using request context for cancellation propagation.
	if _, err := wh.store.Ingest(r.Context(), []*RawEvent{ev}); err != nil {
		http.Error(w, "ingest error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"ok":true}`))
}

// readAll reads from r, returning error on any failure including MaxBytesReader limits.
func readAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var buf []byte
	tmp := make([]byte, 4096)
	for {
		n, err := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			if err.Error() == "http: request body too large" {
				return nil, err
			}
			break
		}
	}
	return buf, nil
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
// Uses delivery ID headers for stable dedup when available.
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

	// Build stable source ID from delivery headers for dedup on retries.
	sourceID := deliveryID(name, r)

	return &RawEvent{
		Source:     SourceWebhook,
		SourceID:   sourceID,
		Kind:       KindWebhook,
		Title:      title,
		URL:        url,
		Actor:      actor,
		GroupKey:   fmt.Sprintf("webhook:%s", name),
		Metadata:   payload,
		OccurredAt: time.Now().UTC(),
	}, nil
}

// deliveryID extracts a stable delivery identifier from webhook headers.
// Falls back to timestamp-based ID if no delivery header is present.
func deliveryID(name string, r *http.Request) string {
	// GitHub: X-GitHub-Delivery
	if id := r.Header.Get("X-GitHub-Delivery"); id != "" {
		return fmt.Sprintf("wh:%s:%s", name, id)
	}
	// Stripe: Stripe-Event-Id
	if id := r.Header.Get("Stripe-Event-Id"); id != "" {
		return fmt.Sprintf("wh:%s:%s", name, id)
	}
	// Generic: X-Request-Id / X-Delivery-Id
	if id := r.Header.Get("X-Request-Id"); id != "" {
		return fmt.Sprintf("wh:%s:%s", name, id)
	}
	if id := r.Header.Get("X-Delivery-Id"); id != "" {
		return fmt.Sprintf("wh:%s:%s", name, id)
	}
	// Fallback: timestamp-based (not ideal for dedup).
	return fmt.Sprintf("wh:%s:%d", name, time.Now().UnixNano())
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
