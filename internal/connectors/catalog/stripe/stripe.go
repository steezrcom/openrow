// Package stripe will integrate with Stripe for card payments and subscriptions.
// Currently a coming-soon stub — descriptor only.
package stripe

import "github.com/openrow/openrow/internal/connectors"

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
	})
}
