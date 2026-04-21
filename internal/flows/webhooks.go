package flows

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
)

// NewWebhookToken generates a 32-byte random token plus its SHA-256 hash.
// Store the hash, hand the plaintext to the user once — it won't be
// retrievable later (standard pattern for API tokens).
func NewWebhookToken() (plaintext string, hash []byte, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	plaintext = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(plaintext))
	return plaintext, sum[:], nil
}

// VerifyWebhookToken compares a presented token against the stored hash
// in constant time.
func VerifyWebhookToken(presented string, hash []byte) bool {
	if len(hash) == 0 {
		return false
	}
	sum := sha256.Sum256([]byte(presented))
	return subtle.ConstantTimeCompare(sum[:], hash) == 1
}

// RotateWebhookToken regenerates the token for a flow. Returns the new
// plaintext (to show the user once).
func (s *Service) RotateWebhookToken(ctx context.Context, tenantID, flowID string) (string, error) {
	plaintext, hash, err := NewWebhookToken()
	if err != nil {
		return "", err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE openrow.flows
		SET webhook_token_hash = $1, updated_at = now()
		WHERE tenant_id = $2 AND id = $3`,
		hash, tenantID, flowID)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() == 0 {
		return "", ErrNotFound
	}
	return plaintext, nil
}

// ResolveWebhookTarget validates the (tenant_slug, flow_id, presented_token)
// triple and returns the flow. Public endpoint callers hit this before
// dispatching — it's the security boundary.
func (s *Service) ResolveWebhookTarget(ctx context.Context, tenantSlug, flowID, presentedToken string) (*Flow, error) {
	if presentedToken == "" {
		return nil, errors.New("missing token")
	}
	var (
		tenantID string
		hash     []byte
	)
	err := s.pool.QueryRow(ctx, `
		SELECT t.id, f.webhook_token_hash
		FROM openrow.flows f
		JOIN openrow.tenants t ON t.id = f.tenant_id
		WHERE t.slug = $1 AND f.id = $2
		  AND f.trigger_kind = 'webhook'
		  AND f.enabled = true`,
		tenantSlug, flowID,
	).Scan(&tenantID, &hash)
	if err != nil {
		return nil, errors.New("no such webhook flow")
	}
	if !VerifyWebhookToken(presentedToken, hash) {
		return nil, errors.New("invalid token")
	}
	return s.Get(ctx, tenantID, flowID)
}
