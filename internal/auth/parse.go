package auth

import (
	"bytes"

	"github.com/go-webauthn/webauthn/protocol"
)

// parseCredentialCreationResponse parses a WebAuthn registration response from raw JSON bytes.
func parseCredentialCreationResponse(body []byte) (*protocol.ParsedCredentialCreationData, error) {
	return protocol.ParseCredentialCreationResponseBody(bytes.NewReader(body))
}

// parseCredentialRequestResponse parses a WebAuthn login response from raw JSON bytes.
func parseCredentialRequestResponse(body []byte) (*protocol.ParsedCredentialAssertionData, error) {
	return protocol.ParseCredentialRequestResponseBody(bytes.NewReader(body))
}
