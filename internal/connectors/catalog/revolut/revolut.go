// Package revolut will integrate with Revolut Business banking.
// Currently a coming-soon stub — descriptor only.
package revolut

import "github.com/openrow/openrow/internal/connectors"

func init() {
	connectors.Register(&connectors.Connector{
		ID:          "revolut",
		Name:        "Revolut Business",
		Description: "Business banking. Import account balances and transactions.",
		Category:    "banking",
		Homepage:    "https://business.revolut.com",
		Status:      connectors.StatusComingSoon,
		Credentials: []connectors.CredentialField{
			{Name: "client_id", Label: "Client ID", Kind: connectors.FieldText, Required: true},
			{Name: "private_key", Label: "Private key (PEM)", Kind: connectors.FieldSecret, Required: true,
				Help: "Paste the full PEM including BEGIN/END lines."},
			{Name: "issuer", Label: "Issuer", Kind: connectors.FieldText, Required: true,
				Placeholder: "your.domain.com"},
		},
	})
}
