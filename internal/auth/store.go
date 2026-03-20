// Package auth provides WebAuthn credential storage and session management.
package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// SessionTTL is how long a biometric session lasts before expiry.
const SessionTTL = 30 * 24 * time.Hour // 30 days

// Store manages WebAuthn credentials and sessions in SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates a new auth Store backed by the given database.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Credential is a stored WebAuthn credential row.
type Credential struct {
	ID         string    `json:"id"`
	PublicKey  []byte    `json:"-"`
	AAGUID     string    `json:"aaguid"`
	SignCount  uint32    `json:"signCount"`
	Name       string    `json:"name"`
	Flags      uint8     `json:"flags"`
	Transports []string  `json:"transports"`
	AttObject  []byte    `json:"-"`
	AttClient  []byte    `json:"-"`
	CreatedAt  time.Time `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

// ToWebAuthn converts a stored credential to go-webauthn's Credential type.
func (c *Credential) ToWebAuthn() webauthn.Credential {
	cred := webauthn.Credential{
		ID:        []byte(c.ID),
		PublicKey: c.PublicKey,
		Flags:     webauthn.NewCredentialFlags(protocol.AuthenticatorFlags(c.Flags)),
		Authenticator: webauthn.Authenticator{
			SignCount: c.SignCount,
		},
	}
	if c.AttObject != nil {
		cred.Attestation.Object = c.AttObject
	}
	if c.AttClient != nil {
		cred.Attestation.ClientDataJSON = c.AttClient
	}
	return cred
}

// SaveCredential inserts a new credential from a WebAuthn registration result.
func (s *Store) SaveCredential(cred *webauthn.Credential, name string) error {
	transports, _ := json.Marshal(cred.Transport)
	_, err := s.db.Exec(`
		INSERT INTO webauthn_credentials (id, public_key, aaguid, sign_count, name, flags, transports, attestation_object, attestation_client_data)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(cred.ID),
		cred.PublicKey,
		"",
		cred.Authenticator.SignCount,
		name,
		cred.Flags.ProtocolValue(),
		string(transports),
		cred.Attestation.Object,
		cred.Attestation.ClientDataJSON,
	)
	return err
}

// ListCredentials returns all stored credentials.
func (s *Store) ListCredentials() ([]Credential, error) {
	rows, err := s.db.Query(`SELECT id, public_key, aaguid, sign_count, name, flags, transports, attestation_object, attestation_client_data, created_at, last_used_at FROM webauthn_credentials ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creds []Credential
	for rows.Next() {
		var c Credential
		var transportsJSON string
		var createdStr string
		var lastUsedStr sql.NullString
		var attObj, attClient []byte
		if err := rows.Scan(&c.ID, &c.PublicKey, &c.AAGUID, &c.SignCount, &c.Name, &c.Flags, &transportsJSON, &attObj, &attClient, &createdStr, &lastUsedStr); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(transportsJSON), &c.Transports)
		c.AttObject = attObj
		c.AttClient = attClient
		c.CreatedAt, _ = time.Parse(time.DateTime, createdStr)
		if lastUsedStr.Valid {
			t, _ := time.Parse(time.DateTime, lastUsedStr.String)
			c.LastUsedAt = &t
		}
		creds = append(creds, c)
	}
	return creds, nil
}

// GetWebAuthnCredentials returns all credentials in go-webauthn format.
func (s *Store) GetWebAuthnCredentials() ([]webauthn.Credential, error) {
	stored, err := s.ListCredentials()
	if err != nil {
		return nil, err
	}
	out := make([]webauthn.Credential, len(stored))
	for i := range stored {
		out[i] = stored[i].ToWebAuthn()
	}
	return out, nil
}

// UpdateSignCount updates the sign count after a successful login.
func (s *Store) UpdateSignCount(credID []byte, newCount uint32) error {
	_, err := s.db.Exec(`UPDATE webauthn_credentials SET sign_count = ?, last_used_at = datetime('now') WHERE id = ?`,
		newCount, string(credID))
	return err
}

// DeleteCredential removes a credential by ID.
func (s *Store) DeleteCredential(id string) error {
	res, err := s.db.Exec(`DELETE FROM webauthn_credentials WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// HasCredentials returns true if at least one credential is registered.
func (s *Store) HasCredentials() (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM webauthn_credentials`).Scan(&count)
	return count > 0, err
}

// --- Sessions ---

// Session represents an active biometric login session.
type Session struct {
	ID           string    `json:"id"`
	CredentialID string    `json:"credentialId"`
	CreatedAt    time.Time `json:"createdAt"`
	ExpiresAt    time.Time `json:"expiresAt"`
	IP           string    `json:"ip"`
	UserAgent    string    `json:"userAgent"`
}

// CreateSession creates a new session for a credential, returning the session token.
func (s *Store) CreateSession(credentialID string, ip, userAgent string) (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)
	expiresAt := time.Now().Add(SessionTTL)

	_, err := s.db.Exec(`INSERT INTO webauthn_sessions (id, credential_id, expires_at, ip, user_agent) VALUES (?, ?, ?, ?, ?)`,
		token, credentialID, expiresAt.UTC().Format(time.DateTime), ip, userAgent)
	if err != nil {
		return "", err
	}
	return token, nil
}

// ValidateSession checks if a session token is valid and not expired.
func (s *Store) ValidateSession(token string) (*Session, error) {
	var sess Session
	var createdStr, expiresStr string
	err := s.db.QueryRow(`SELECT id, credential_id, created_at, expires_at, ip, user_agent FROM webauthn_sessions WHERE id = ?`, token).
		Scan(&sess.ID, &sess.CredentialID, &createdStr, &expiresStr, &sess.IP, &sess.UserAgent)
	if err != nil {
		return nil, err
	}
	sess.CreatedAt, _ = time.Parse(time.DateTime, createdStr)
	sess.ExpiresAt, _ = time.Parse(time.DateTime, expiresStr)

	if time.Now().After(sess.ExpiresAt) {
		// Expired — clean it up.
		s.db.Exec(`DELETE FROM webauthn_sessions WHERE id = ?`, token)
		return nil, sql.ErrNoRows
	}
	return &sess, nil
}

// DeleteSession removes a session by token.
func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM webauthn_sessions WHERE id = ?`, token)
	return err
}

// CleanExpiredSessions removes all expired sessions.
func (s *Store) CleanExpiredSessions() (int64, error) {
	res, err := s.db.Exec(`DELETE FROM webauthn_sessions WHERE expires_at < datetime('now')`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
