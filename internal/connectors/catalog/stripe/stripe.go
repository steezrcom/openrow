// Package stripe integrates with Stripe for card payments and subscriptions.
// Actions are still stubs, but the webhook signature verifier is live —
// a flow can receive Stripe events as soon as the user pastes in the
// signing secret.
package stripe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/openrow/openrow/internal/connectors"
)

func init() {
	connectors.Register(&connectors.Connector{
		ID:          "stripe",
		Name:        "Stripe",
		Description: "Card payments and subscriptions. Sync charges, payouts and fees.",
		Category:    "payments",
		Homepage:    "https://stripe.com",
		Status:      connectors.StatusComingSoon,
		Credentials: []connectors.CredentialField{
			{Name: "secret_key", Label: "Secret key", Kind: connectors.FieldSecret, Required: true,
				Placeholder: "sk_live_…",
				Help:        "Restricted keys with read access on charges, payouts and customers are recommended."},
			{Name: "webhook_secret", Label: "Webhook signing secret", Kind: connectors.FieldSecret, Required: false,
				Placeholder: "whsec_…",
				Help:        "Optional. Enables near-real-time sync via webhook."},
		},
		VerifyWebhook: verifyWebhook,
	})
}

// verifyWebhook implements Stripe's webhook signature scheme:
// https://docs.stripe.com/webhooks#verify-manually
//
// The Stripe-Signature header carries comma-separated key=value pairs,
// e.g. "t=1700000000,v1=abcdef...". We compute HMAC-SHA256 over the
// literal string "<t>.<body>" with the signing secret and compare in
// constant time to each v1 value. Tolerance guards against replay.
func verifyWebhook(_ context.Context, secret string, headers map[string][]string, body []byte) error {
	if secret == "" {
		return errors.New("stripe: no signing secret configured")
	}
	sigHeader := firstHeader(headers, "Stripe-Signature")
	if sigHeader == "" {
		return errors.New("stripe: missing Stripe-Signature header")
	}

	var (
		timestamp  int64
		signatures []string
	)
	for _, part := range strings.Split(sigHeader, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			n, err := strconv.ParseInt(kv[1], 10, 64)
			if err != nil {
				return fmt.Errorf("stripe: bad timestamp: %w", err)
			}
			timestamp = n
		case "v1":
			signatures = append(signatures, kv[1])
		}
	}
	if timestamp == 0 {
		return errors.New("stripe: signature header missing timestamp")
	}
	if len(signatures) == 0 {
		return errors.New("stripe: signature header missing v1 entry")
	}

	// 5-minute replay tolerance, matching Stripe's SDK default.
	if abs(time.Now().Unix()-timestamp) > 300 {
		return errors.New("stripe: signature timestamp outside tolerance (possible replay)")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.", timestamp)
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	for _, s := range signatures {
		if hmac.Equal([]byte(expected), []byte(s)) {
			return nil
		}
	}
	return errors.New("stripe: signature mismatch")
}

func firstHeader(headers map[string][]string, key string) string {
	for k, v := range headers {
		if strings.EqualFold(k, key) && len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
