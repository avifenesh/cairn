package server

import (
	"encoding/json"
	"io"
	"net/http"
)

// --- Auth: WebAuthn Registration ---

// handleAuthRegisterStart begins a WebAuthn registration ceremony.
// Requires write token (must be already authenticated to register a credential).
func (s *Server) handleAuthRegisterStart(w http.ResponseWriter, r *http.Request) {
	options, err := s.webauthn.BeginRegistration()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, options)
}

// handleAuthRegisterComplete finishes a WebAuthn registration ceremony.
func (s *Server) handleAuthRegisterComplete(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	// Extract optional name from wrapper: {"name": "...", "credential": {...}}
	// Or the body may be the raw credential response directly.
	var wrapper struct {
		Name       string          `json:"name"`
		Credential json.RawMessage `json:"credential"`
	}
	credBody := body
	if err := json.Unmarshal(body, &wrapper); err == nil && len(wrapper.Credential) > 0 {
		credBody = wrapper.Credential
	}

	cred, err := s.webauthn.FinishRegistration(credBody)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	name := wrapper.Name
	if name == "" {
		name = "Credential"
	}

	if err := s.authStore.SaveCredential(cred, name); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save credential: "+err.Error())
		return
	}

	// Create a session so the user is immediately logged in via biometric.
	token, err := s.authStore.CreateSession(string(cred.ID), clientIP(r), r.UserAgent())
	if err != nil {
		// Non-fatal — credential is saved, just no session cookie.
		s.logger.Warn("failed to create session after registration", "error", err)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	setCairnSessionCookie(w, token, r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- Auth: WebAuthn Login ---

// handleAuthLoginStart begins a WebAuthn login ceremony.
// Open to all — this is how users authenticate.
func (s *Server) handleAuthLoginStart(w http.ResponseWriter, r *http.Request) {
	options, err := s.webauthn.BeginLogin()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, options)
}

// handleAuthLoginComplete finishes a WebAuthn login ceremony.
func (s *Server) handleAuthLoginComplete(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}

	credID, err := s.webauthn.FinishLogin(body)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	token, err := s.authStore.CreateSession(string(credID), clientIP(r), r.UserAgent())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	setCairnSessionCookie(w, token, r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- Auth: Session ---

// handleAuthSession checks if the current request has a valid session.
func (s *Server) handleAuthSession(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("cairn_session")
	if err != nil || c.Value == "" {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": false, "method": "none"})
		return
	}

	sess, err := s.authStore.ValidateSession(c.Value)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": false, "method": "none"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"method":        "webauthn",
		"sessionId":     sess.ID[:8] + "...",
		"expiresAt":     sess.ExpiresAt,
	})
}

// handleAuthLogout invalidates the current session.
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("cairn_session")
	if err == nil && c.Value != "" {
		s.authStore.DeleteSession(c.Value)
	}

	// Clear the cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     "cairn_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteStrictMode,
	})

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- Auth: Credential Management ---

// handleListAuthCredentials lists all registered WebAuthn credentials.
func (s *Server) handleListAuthCredentials(w http.ResponseWriter, r *http.Request) {
	creds, err := s.authStore.ListCredentials()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"credentials": creds})
}

// handleDeleteAuthCredential removes a WebAuthn credential by ID.
func (s *Server) handleDeleteAuthCredential(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing credential id")
		return
	}
	if err := s.authStore.DeleteCredential(id); err != nil {
		writeError(w, http.StatusNotFound, "credential not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- Helpers ---

// setCairnSessionCookie sets the cairn_session HttpOnly cookie.
func setCairnSessionCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     "cairn_session",
		Value:    token,
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60, // 30 days
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}
