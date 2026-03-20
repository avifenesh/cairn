-- WebAuthn credentials and sessions for biometric authentication.

CREATE TABLE IF NOT EXISTS webauthn_credentials (
    id          TEXT PRIMARY KEY,       -- credential ID (base64url-encoded)
    public_key  BLOB NOT NULL,          -- CBOR-encoded public key
    aaguid      TEXT NOT NULL DEFAULT '',
    sign_count  INTEGER NOT NULL DEFAULT 0,
    name        TEXT NOT NULL DEFAULT '',  -- user-friendly label
    flags       INTEGER NOT NULL DEFAULT 0, -- CredentialFlags.ProtocolValue()
    transports  TEXT NOT NULL DEFAULT '[]', -- JSON array of transport strings
    attestation_object      BLOB,
    attestation_client_data BLOB,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    last_used_at TEXT
);

CREATE TABLE IF NOT EXISTS webauthn_sessions (
    id              TEXT PRIMARY KEY,       -- session token (hex, 32 random bytes)
    credential_id   TEXT NOT NULL REFERENCES webauthn_credentials(id) ON DELETE CASCADE,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at      TEXT NOT NULL,
    ip              TEXT NOT NULL DEFAULT '',
    user_agent      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_webauthn_sessions_expires ON webauthn_sessions(expires_at);
