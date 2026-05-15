// Package signedstate produces and verifies HMAC-SHA256 signed blobs.
// The intended use is short-lived round-trip tokens like an OAuth state
// parameter — the producer encodes whatever bytes it needs, the verifier
// recovers them after confirming integrity.
//
// Format: base64url(payload) "." base64url(mac), where mac is
// HMAC-SHA256 over the raw payload bytes (not the base64 form) under the
// configured key.
//
// The key passed to New is treated as raw key material. Callers should
// derive a domain-separated key (e.g. SHA-256(root_key || domain)) so
// different uses of the same secret don't share signers.
package signedstate

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// Signer signs and verifies state blobs with a shared key.
type Signer struct {
	key []byte
}

// New returns a Signer keyed by the given byte slice. The slice is
// copied so the caller may reuse or zero its original buffer.
func New(key []byte) (*Signer, error) {
	if len(key) == 0 {
		return nil, errors.New("signedstate: key must not be empty")
	}
	c := make([]byte, len(key))
	copy(c, key)
	return &Signer{key: c}, nil
}

// Derive returns a domain-separated 32-byte key for use with New.
// Convention: pass the deployment's root secret and a short, stable
// label like "openrow-oauth-state-v1".
func Derive(rootKey []byte, domain string) []byte {
	h := sha256.New()
	h.Write(rootKey)
	h.Write([]byte{0x00}) // separator so key and domain can't collide
	h.Write([]byte(domain))
	return h.Sum(nil)
}

// Build encodes payload into a self-contained signed blob.
func (s *Signer) Build(payload []byte) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write(payload)
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(sig)
}

// Open verifies a signed blob and returns the original payload bytes.
// The error is intentionally generic — never leak which check failed.
func (s *Signer) Open(token string) ([]byte, error) {
	dot := strings.IndexByte(token, '.')
	if dot <= 0 || dot == len(token)-1 {
		return nil, errInvalid
	}
	rawPayload, err := base64.RawURLEncoding.DecodeString(token[:dot])
	if err != nil {
		return nil, errInvalid
	}
	rawSig, err := base64.RawURLEncoding.DecodeString(token[dot+1:])
	if err != nil {
		return nil, errInvalid
	}
	mac := hmac.New(sha256.New, s.key)
	mac.Write(rawPayload)
	if !hmac.Equal(rawSig, mac.Sum(nil)) {
		return nil, errInvalid
	}
	return rawPayload, nil
}

var errInvalid = errors.New("signedstate: invalid token")

// Errorf wraps a low-level reason with the public errInvalid sentinel so
// callers can match on errors.Is(err, ErrInvalid) without seeing the
// internal detail.
func errorf(format string, a ...any) error {
	return fmt.Errorf("%w: %s", errInvalid, fmt.Sprintf(format, a...))
}

// ErrInvalid is the sole error class returned by Open. Callers treat
// every failure the same: reject and redirect with a generic error.
var ErrInvalid = errInvalid
