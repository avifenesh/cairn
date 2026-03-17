package server

import (
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// --- Auth Middleware ---

// extractToken reads the API token from the request, checking sources in
// precedence order: X-Api-Token header > Authorization: Bearer > ?token= > pub_session cookie.
func extractToken(r *http.Request) string {
	// 1. X-Api-Token header.
	if tok := r.Header.Get("X-Api-Token"); tok != "" {
		return tok
	}
	// 2. Authorization: Bearer header.
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	// 3. ?token= query param (needed for EventSource which can't set headers).
	if tok := r.URL.Query().Get("token"); tok != "" {
		return tok
	}
	// 4. pub_session cookie (WebAuthn sessions).
	if c, err := r.Cookie("pub_session"); err == nil && c.Value != "" {
		return c.Value
	}
	return ""
}

// authMiddleware enforces read/write token checks based on HTTP method and path.
// Health/ready endpoints are always open.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Health and ready are always open.
		if path == "/health" || path == "/ready" {
			next.ServeHTTP(w, r)
			return
		}

		// Preflight OPTIONS are always open (CORS handles them).
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		token := extractToken(r)

		// Write endpoints: POST, PUT, DELETE under /v1/*
		if isWriteRequest(r) {
			if s.config.WriteAPIToken == "" {
				writeError(w, http.StatusServiceUnavailable, "write token not configured")
				return
			}
			if token != s.config.WriteAPIToken {
				writeError(w, http.StatusUnauthorized, "invalid or missing write token")
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// Read endpoints: GET under /v1/* — optional token if configured.
		if s.config.ReadAPIToken != "" {
			validRead := token == s.config.ReadAPIToken
			validWrite := s.config.WriteAPIToken != "" && token == s.config.WriteAPIToken
			if !validRead && !validWrite {
				writeError(w, http.StatusUnauthorized, "invalid or missing read token")
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// isWriteRequest returns true if the request is a write (POST/PUT/DELETE) to /v1/*.
func isWriteRequest(r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/v1/") {
		return false
	}
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodDelete:
		return true
	}
	return false
}

// requireWrite is a per-handler wrapper that enforces write token. Used when a
// handler registered on a method-specific pattern still needs an explicit check.
func (s *Server) requireWrite(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.config.WriteAPIToken == "" {
			writeError(w, http.StatusServiceUnavailable, "write token not configured")
			return
		}
		token := extractToken(r)
		if token != s.config.WriteAPIToken {
			writeError(w, http.StatusUnauthorized, "invalid or missing write token")
			return
		}
		next(w, r)
	}
}

// optionalRead is a per-handler wrapper for read-protected endpoints.
func (s *Server) optionalRead(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.config.ReadAPIToken != "" {
			token := extractToken(r)
			if token != s.config.ReadAPIToken && token != s.config.WriteAPIToken {
				writeError(w, http.StatusUnauthorized, "invalid or missing read token")
				return
			}
		}
		next(w, r)
	}
}

// --- CORS Middleware ---

// corsMiddleware handles CORS headers and preflight OPTIONS requests.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := "*"
		if s.config.FrontendOrigin != "" {
			origin = s.config.FrontendOrigin
		}

		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Api-Token, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// --- Rate Limiting ---

// rateLimiter implements a simple sliding window rate limiter with per-IP tracking.
type rateLimiter struct {
	mu       sync.Mutex
	windows  map[string]*window
	done     chan struct{}
	stopped  atomic.Bool
}

// window tracks request timestamps within a sliding window.
type window struct {
	timestamps []time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		windows: make(map[string]*window),
		done:    make(chan struct{}),
	}
}

// allow checks if a request from the given key is within the rate limit.
// limit is the max number of requests, windowDuration is the sliding window size.
func (rl *rateLimiter) allow(key string, limit int, windowDuration time.Duration) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-windowDuration)

	w, ok := rl.windows[key]
	if !ok {
		w = &window{}
		rl.windows[key] = w
	}

	// Trim expired entries.
	valid := w.timestamps[:0]
	for _, ts := range w.timestamps {
		if ts.After(cutoff) {
			valid = append(valid, ts)
		}
	}
	w.timestamps = valid

	if len(w.timestamps) >= limit {
		return false
	}

	w.timestamps = append(w.timestamps, now)
	return true
}

// startCleanup launches a goroutine that periodically removes stale entries.
func (rl *rateLimiter) startCleanup() {
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-rl.done:
				return
			case <-ticker.C:
				rl.cleanup()
			}
		}
	}()
}

// cleanup removes windows that have no recent entries.
func (rl *rateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-2 * time.Minute)
	for key, w := range rl.windows {
		if len(w.timestamps) == 0 || w.timestamps[len(w.timestamps)-1].Before(cutoff) {
			delete(rl.windows, key)
		}
	}
}

// stop shuts down the cleanup goroutine.
func (rl *rateLimiter) stop() {
	if rl.stopped.CompareAndSwap(false, true) {
		close(rl.done)
	}
}

// rateLimitMiddleware wraps a handler with per-IP rate limiting.
func (s *Server) rateLimitMiddleware(limit int, window time.Duration, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !s.rateLimiter.allow(ip, limit, window) {
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next(w, r)
	}
}

// clientIP extracts the client IP from the request. Checks X-Forwarded-For
// first (for reverse proxy), then falls back to RemoteAddr.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain.
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	// Strip port from RemoteAddr.
	host, _, err := strings.Cut(r.RemoteAddr, ":")
	if err {
		return host
	}
	return r.RemoteAddr
}
