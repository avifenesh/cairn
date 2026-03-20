package auth

import (
	"testing"
	"time"

	"github.com/avifenesh/cairn/internal/db"
	"github.com/go-webauthn/webauthn/webauthn"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:): %v", err)
	}
	if err := d.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestStore_SaveAndListCredentials(t *testing.T) {
	d := openTestDB(t)
	s := NewStore(d.DB)

	cred := &webauthn.Credential{
		ID:        []byte("test-cred-id-123"),
		PublicKey: []byte("fake-public-key"),
		Authenticator: webauthn.Authenticator{
			SignCount: 0,
		},
	}

	if err := s.SaveCredential(cred, "My Laptop"); err != nil {
		t.Fatalf("SaveCredential: %v", err)
	}

	creds, err := s.ListCredentials()
	if err != nil {
		t.Fatalf("ListCredentials: %v", err)
	}
	if len(creds) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(creds))
	}
	if creds[0].Name != "My Laptop" {
		t.Errorf("name = %q, want %q", creds[0].Name, "My Laptop")
	}
	if creds[0].SignCount != 0 {
		t.Errorf("signCount = %d, want 0", creds[0].SignCount)
	}
}

func TestStore_GetWebAuthnCredentials(t *testing.T) {
	d := openTestDB(t)
	s := NewStore(d.DB)

	cred := &webauthn.Credential{
		ID:        []byte("wc-id-456"),
		PublicKey: []byte("fake-pk-2"),
		Authenticator: webauthn.Authenticator{
			SignCount: 5,
		},
	}
	if err := s.SaveCredential(cred, "Phone"); err != nil {
		t.Fatalf("SaveCredential: %v", err)
	}

	waCreds, err := s.GetWebAuthnCredentials()
	if err != nil {
		t.Fatalf("GetWebAuthnCredentials: %v", err)
	}
	if len(waCreds) != 1 {
		t.Fatalf("expected 1, got %d", len(waCreds))
	}
	if string(waCreds[0].ID) != string(cred.ID) {
		t.Errorf("credential ID mismatch")
	}
	if waCreds[0].Authenticator.SignCount != 5 {
		t.Errorf("signCount = %d, want 5", waCreds[0].Authenticator.SignCount)
	}
}

func TestStore_UpdateSignCount(t *testing.T) {
	d := openTestDB(t)
	s := NewStore(d.DB)

	cred := &webauthn.Credential{
		ID:        []byte("sign-count-test"),
		PublicKey: []byte("pk"),
		Authenticator: webauthn.Authenticator{
			SignCount: 0,
		},
	}
	if err := s.SaveCredential(cred, "Test"); err != nil {
		t.Fatalf("SaveCredential: %v", err)
	}

	if err := s.UpdateSignCount(cred.ID, 42); err != nil {
		t.Fatalf("UpdateSignCount: %v", err)
	}

	creds, err := s.ListCredentials()
	if err != nil {
		t.Fatalf("ListCredentials: %v", err)
	}
	if creds[0].SignCount != 42 {
		t.Errorf("signCount = %d, want 42", creds[0].SignCount)
	}
	if creds[0].LastUsedAt == nil {
		t.Error("expected LastUsedAt to be set after UpdateSignCount")
	}
}

func TestStore_DeleteCredential(t *testing.T) {
	d := openTestDB(t)
	s := NewStore(d.DB)

	cred := &webauthn.Credential{
		ID:        []byte("delete-me"),
		PublicKey: []byte("pk"),
	}
	if err := s.SaveCredential(cred, "Temp"); err != nil {
		t.Fatalf("SaveCredential: %v", err)
	}

	id := credIDToString(cred.ID)
	if err := s.DeleteCredential(id); err != nil {
		t.Fatalf("DeleteCredential: %v", err)
	}

	creds, err := s.ListCredentials()
	if err != nil {
		t.Fatalf("ListCredentials: %v", err)
	}
	if len(creds) != 0 {
		t.Errorf("expected 0 credentials after delete, got %d", len(creds))
	}
}

func TestStore_DeleteCredential_NotFound(t *testing.T) {
	d := openTestDB(t)
	s := NewStore(d.DB)

	err := s.DeleteCredential("nonexistent")
	if err == nil {
		t.Error("expected error deleting nonexistent credential")
	}
}

func TestStore_HasCredentials(t *testing.T) {
	d := openTestDB(t)
	s := NewStore(d.DB)

	has, err := s.HasCredentials()
	if err != nil {
		t.Fatalf("HasCredentials: %v", err)
	}
	if has {
		t.Error("expected no credentials initially")
	}

	cred := &webauthn.Credential{
		ID:        []byte("has-check"),
		PublicKey: []byte("pk"),
	}
	if err := s.SaveCredential(cred, "Test"); err != nil {
		t.Fatalf("SaveCredential: %v", err)
	}

	has, err = s.HasCredentials()
	if err != nil {
		t.Fatalf("HasCredentials: %v", err)
	}
	if !has {
		t.Error("expected has=true after save")
	}
}

func TestStore_CreateAndValidateSession(t *testing.T) {
	d := openTestDB(t)
	s := NewStore(d.DB)

	// Must have a credential first (FK constraint).
	cred := &webauthn.Credential{
		ID:        []byte("sess-cred"),
		PublicKey: []byte("pk"),
	}
	if err := s.SaveCredential(cred, "Test"); err != nil {
		t.Fatalf("SaveCredential: %v", err)
	}

	credID := credIDToString(cred.ID)
	token, err := s.CreateSession(credID, "127.0.0.1", "TestAgent/1.0")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if len(token) != 64 { // 32 bytes hex-encoded
		t.Errorf("token length = %d, want 64", len(token))
	}

	sess, err := s.ValidateSession(token)
	if err != nil {
		t.Fatalf("ValidateSession: %v", err)
	}
	if sess.CredentialID != credID {
		t.Errorf("credentialID = %q, want %q", sess.CredentialID, credID)
	}
	if sess.IP != "127.0.0.1" {
		t.Errorf("IP = %q, want 127.0.0.1", sess.IP)
	}
	if sess.UserAgent != "TestAgent/1.0" {
		t.Errorf("UserAgent = %q, want TestAgent/1.0", sess.UserAgent)
	}
	if time.Until(sess.ExpiresAt) < 29*24*time.Hour {
		t.Errorf("session expires too soon: %v", sess.ExpiresAt)
	}
}

func TestStore_ValidateSession_Invalid(t *testing.T) {
	d := openTestDB(t)
	s := NewStore(d.DB)

	_, err := s.ValidateSession("bogus-token")
	if err == nil {
		t.Error("expected error for invalid session token")
	}
}

func TestStore_DeleteSession(t *testing.T) {
	d := openTestDB(t)
	s := NewStore(d.DB)

	cred := &webauthn.Credential{
		ID:        []byte("del-sess-cred"),
		PublicKey: []byte("pk"),
	}
	if err := s.SaveCredential(cred, "Test"); err != nil {
		t.Fatalf("SaveCredential: %v", err)
	}

	token, err := s.CreateSession(credIDToString(cred.ID), "10.0.0.1", "Bot")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if err := s.DeleteSession(token); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	_, err = s.ValidateSession(token)
	if err == nil {
		t.Error("expected error after session deletion")
	}
}

func TestStore_CleanExpiredSessions(t *testing.T) {
	d := openTestDB(t)
	s := NewStore(d.DB)

	cred := &webauthn.Credential{
		ID:        []byte("clean-cred"),
		PublicKey: []byte("pk"),
	}
	if err := s.SaveCredential(cred, "Test"); err != nil {
		t.Fatalf("SaveCredential: %v", err)
	}

	credID := credIDToString(cred.ID)

	// Insert a session that's already expired.
	_, err := d.DB.Exec(`INSERT INTO webauthn_sessions (id, credential_id, expires_at, ip, user_agent) VALUES (?, ?, ?, ?, ?)`,
		"expired-token", credID, "2020-01-01 00:00:00", "", "")
	if err != nil {
		t.Fatalf("insert expired session: %v", err)
	}

	// Insert a valid session.
	validToken, err := s.CreateSession(credID, "", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	cleaned, err := s.CleanExpiredSessions()
	if err != nil {
		t.Fatalf("CleanExpiredSessions: %v", err)
	}
	if cleaned != 1 {
		t.Errorf("cleaned = %d, want 1", cleaned)
	}

	// Valid session should still work.
	if _, err := s.ValidateSession(validToken); err != nil {
		t.Errorf("valid session should survive cleanup: %v", err)
	}
}

func TestCredIDRoundTrip(t *testing.T) {
	original := []byte("test-credential-id-bytes")
	encoded := credIDToString(original)
	decoded := credIDFromString(encoded)
	if string(decoded) != string(original) {
		t.Errorf("round-trip failed: got %q, want %q", decoded, original)
	}
}

func TestWebAuthn_NewWebAuthn(t *testing.T) {
	d := openTestDB(t)
	s := NewStore(d.DB)

	wa, err := NewWebAuthn("Cairn", "localhost", "http://localhost:8788", s)
	if err != nil {
		t.Fatalf("NewWebAuthn: %v", err)
	}
	if wa == nil {
		t.Fatal("expected non-nil WebAuthn")
	}
}

func TestWebAuthn_BeginLogin_NoCredentials(t *testing.T) {
	d := openTestDB(t)
	s := NewStore(d.DB)

	wa, err := NewWebAuthn("Cairn", "localhost", "http://localhost:8788", s)
	if err != nil {
		t.Fatalf("NewWebAuthn: %v", err)
	}

	_, err = wa.BeginLogin()
	if err == nil {
		t.Error("expected error when no credentials registered")
	}
}

func TestWebAuthn_FinishRegistration_NoPendingChallenge(t *testing.T) {
	d := openTestDB(t)
	s := NewStore(d.DB)

	wa, err := NewWebAuthn("Cairn", "localhost", "http://localhost:8788", s)
	if err != nil {
		t.Fatalf("NewWebAuthn: %v", err)
	}

	_, err = wa.FinishRegistration([]byte(`{}`))
	if err == nil {
		t.Error("expected error with no pending challenge")
	}
}

func TestWebAuthn_FinishLogin_NoPendingChallenge(t *testing.T) {
	d := openTestDB(t)
	s := NewStore(d.DB)

	wa, err := NewWebAuthn("Cairn", "localhost", "http://localhost:8788", s)
	if err != nil {
		t.Fatalf("NewWebAuthn: %v", err)
	}

	_, err = wa.FinishLogin([]byte(`{}`))
	if err == nil {
		t.Error("expected error with no pending challenge")
	}
}

func TestWebAuthn_ChallengeExpiry(t *testing.T) {
	d := openTestDB(t)
	s := NewStore(d.DB)

	wa, err := NewWebAuthn("Cairn", "localhost", "http://localhost:8788", s)
	if err != nil {
		t.Fatalf("NewWebAuthn: %v", err)
	}

	// Manually inject an expired challenge.
	wa.mu.Lock()
	wa.challenges["register"] = &challengeEntry{
		session:   &webauthn.SessionData{},
		expiresAt: time.Now().Add(-1 * time.Minute),
	}
	wa.mu.Unlock()

	_, err = wa.FinishRegistration([]byte(`{}`))
	if err == nil {
		t.Error("expected error for expired challenge")
	}
}
