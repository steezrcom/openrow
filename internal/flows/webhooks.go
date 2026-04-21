package flows

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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

// WebhookTarget bundles everything the public webhook endpoint needs to
// authenticate + dispatch a request: the flow, and an optional signing
// secret (plaintext) that the connector's VerifyWebhook will check
// against the request's signature header.
type WebhookTarget struct {
	Flow                 *Flow
	WebhookConnectorID   string // from trigger_config; empty if no signature verification
	WebhookSigningSecret string // decrypted plaintext; empty if not configured
}

// ResolveWebhookTarget validates the (tenant_slug, flow_id, presented_token)
// triple and returns the target. Public endpoint callers hit this before
// dispatching — it's the security boundary for token auth. Signature
// verification happens in the HTTP layer using the returned secret.
func (s *Service) ResolveWebhookTarget(ctx context.Context, tenantSlug, flowID, presentedToken string) (*WebhookTarget, error) {
	if presentedToken == "" {
		return nil, errors.New("missing token")
	}
	var (
		tenantID      string
		hash          []byte
		signingSecret []byte
	)
	err := s.pool.QueryRow(ctx, `
		SELECT t.id, f.webhook_token_hash, COALESCE(f.webhook_secret, ''::bytea)
		FROM openrow.flows f
		JOIN openrow.tenants t ON t.id = f.tenant_id
		WHERE t.slug = $1 AND f.id = $2
		  AND f.trigger_kind = 'webhook'
		  AND f.enabled = true`,
		tenantSlug, flowID,
	).Scan(&tenantID, &hash, &signingSecret)
	if err != nil {
		return nil, errors.New("no such webhook flow")
	}
	if !VerifyWebhookToken(presentedToken, hash) {
		return nil, errors.New("invalid token")
	}
	flow, err := s.Get(ctx, tenantID, flowID)
	if err != nil {
		return nil, err
	}
	target := &WebhookTarget{Flow: flow}
	// Connector binding lives in trigger_config.webhook_connector_id.
	var cfg struct {
		ConnectorID string `json:"webhook_connector_id"`
	}
	_ = json.Unmarshal(flow.TriggerConfig, &cfg)
	target.WebhookConnectorID = cfg.ConnectorID
	if target.WebhookConnectorID != "" && len(signingSecret) > 0 {
		if s.enc == nil {
			return nil, errors.New("flows service missing encrypter; cannot decrypt signing secret")
		}
		plaintext, decErr := s.enc.Decrypt(signingSecret)
		if decErr != nil {
			return nil, fmt.Errorf("decrypt signing secret: %w", decErr)
		}
		target.WebhookSigningSecret = string(plaintext)
	}
	return target, nil
}
