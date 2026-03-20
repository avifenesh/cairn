package auth

import (
	"fmt"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// challengeTTL is how long a registration/login challenge is valid.
const challengeTTL = 5 * time.Minute

// WebAuthn wraps the go-webauthn library with challenge storage and the single-user model.
type WebAuthn struct {
	wa    *webauthn.WebAuthn
	store *Store

	// In-memory challenge storage (single server, no need for DB).
	mu         sync.Mutex
	challenges map[string]*challengeEntry // key = "register" or "login"
}

type challengeEntry struct {
	session   *webauthn.SessionData
	expiresAt time.Time
}

// NewWebAuthn creates a WebAuthn handler for the given origin.
func NewWebAuthn(rpDisplayName, rpID, origin string, store *Store) (*WebAuthn, error) {
	wa, err := webauthn.New(&webauthn.Config{
		RPDisplayName: rpDisplayName,
		RPID:          rpID,
		RPOrigins:     []string{origin},
	})
	if err != nil {
		return nil, fmt.Errorf("webauthn.New: %w", err)
	}
	return &WebAuthn{
		wa:         wa,
		store:      store,
		challenges: make(map[string]*challengeEntry),
	}, nil
}

// ownerUser implements webauthn.User for the single owner.
type ownerUser struct {
	credentials []webauthn.Credential
}

func (u *ownerUser) WebAuthnID() []byte                         { return []byte("owner") }
func (u *ownerUser) WebAuthnName() string                       { return "owner" }
func (u *ownerUser) WebAuthnDisplayName() string                { return "Owner" }
func (u *ownerUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

// BeginRegistration starts a WebAuthn registration ceremony.
func (w *WebAuthn) BeginRegistration() (*protocol.CredentialCreation, error) {
	creds, err := w.store.GetWebAuthnCredentials()
	if err != nil {
		return nil, fmt.Errorf("load credentials: %w", err)
	}
	user := &ownerUser{credentials: creds}

	options, session, err := w.wa.BeginRegistration(user)
	if err != nil {
		return nil, fmt.Errorf("begin registration: %w", err)
	}

	w.mu.Lock()
	w.challenges["register"] = &challengeEntry{
		session:   session,
		expiresAt: time.Now().Add(challengeTTL),
	}
	w.mu.Unlock()

	return options, nil
}

// FinishRegistration completes a WebAuthn registration ceremony.
// Returns the new credential on success.
func (w *WebAuthn) FinishRegistration(body []byte) (*webauthn.Credential, error) {
	w.mu.Lock()
	entry, ok := w.challenges["register"]
	if ok {
		delete(w.challenges, "register")
	}
	w.mu.Unlock()

	if !ok || time.Now().After(entry.expiresAt) {
		return nil, fmt.Errorf("no pending registration challenge or challenge expired")
	}

	creds, err := w.store.GetWebAuthnCredentials()
	if err != nil {
		return nil, fmt.Errorf("load credentials: %w", err)
	}
	user := &ownerUser{credentials: creds}

	// Parse the response from the body bytes.
	parsedResponse, err := parseCredentialCreationResponse(body)
	if err != nil {
		return nil, fmt.Errorf("parse registration response: %w", err)
	}

	cred, err := w.wa.CreateCredential(user, *entry.session, parsedResponse)
	if err != nil {
		return nil, fmt.Errorf("create credential: %w", err)
	}

	return cred, nil
}

// BeginLogin starts a WebAuthn login ceremony.
func (w *WebAuthn) BeginLogin() (*protocol.CredentialAssertion, error) {
	creds, err := w.store.GetWebAuthnCredentials()
	if err != nil {
		return nil, fmt.Errorf("load credentials: %w", err)
	}
	if len(creds) == 0 {
		return nil, fmt.Errorf("no credentials registered")
	}
	user := &ownerUser{credentials: creds}

	options, session, err := w.wa.BeginLogin(user)
	if err != nil {
		return nil, fmt.Errorf("begin login: %w", err)
	}

	w.mu.Lock()
	w.challenges["login"] = &challengeEntry{
		session:   session,
		expiresAt: time.Now().Add(challengeTTL),
	}
	w.mu.Unlock()

	return options, nil
}

// FinishLogin completes a WebAuthn login ceremony.
// Returns the credential ID on success.
func (w *WebAuthn) FinishLogin(body []byte) ([]byte, error) {
	w.mu.Lock()
	entry, ok := w.challenges["login"]
	if ok {
		delete(w.challenges, "login")
	}
	w.mu.Unlock()

	if !ok || time.Now().After(entry.expiresAt) {
		return nil, fmt.Errorf("no pending login challenge or challenge expired")
	}

	creds, err := w.store.GetWebAuthnCredentials()
	if err != nil {
		return nil, fmt.Errorf("load credentials: %w", err)
	}
	user := &ownerUser{credentials: creds}

	parsedResponse, err := parseCredentialRequestResponse(body)
	if err != nil {
		return nil, fmt.Errorf("parse login response: %w", err)
	}

	cred, err := w.wa.ValidateLogin(user, *entry.session, parsedResponse)
	if err != nil {
		return nil, fmt.Errorf("validate login: %w", err)
	}

	// Update sign count.
	if err := w.store.UpdateSignCount(cred.ID, cred.Authenticator.SignCount); err != nil {
		// Non-fatal — log but don't fail the login.
		_ = err
	}

	return cred.ID, nil
}
