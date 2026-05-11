// Package flexi integrates with ABRA Flexi (formerly FlexiBee), a Czech SMB
// ERP. The REST API is self-describing via /c/<company>/evidence-list and
// per-evidence /properties endpoints — discovery uses those.
package flexi

import (
	"context"

	"github.com/openrow/openrow/internal/connectors"
)

const ConnectorID = "abra-flexi"

func init() {
	connectors.Register(&connectors.Connector{
		ID:          ConnectorID,
		Name:        "ABRA Flexi",
		Description: "Czech SMB ERP. AI-native binding scans the evidence list and proposes a mapping the agent can query.",
		Category:    "erp",
		Homepage:    "https://www.abra.eu/flexi/",
		Status:      connectors.StatusAvailable,
		Credentials: []connectors.CredentialField{
			{Name: "base_url", Label: "Server URL", Kind: connectors.FieldURL, Required: true,
				Placeholder: "https://demo.flexibee.eu",
				Help:        "Root URL of the Flexi server (no trailing /c/<company>)."},
			{Name: "company", Label: "Company token", Kind: connectors.FieldText, Required: true,
				Placeholder: "demo_s_r_o_",
				Help:        "Company segment from your Flexi URL — the part after /c/."},
			{Name: "username", Label: "Username", Kind: connectors.FieldText, Required: true},
			{Name: "password", Label: "Password", Kind: connectors.FieldSecret, Required: true},
		},
		Test:    testCreds,
		Actions: actions(),
	})
}

func testCreds(ctx context.Context, creds map[string]string) error {
	c, err := NewClient(creds, false)
	if err != nil {
		return err
	}
	_, err = c.ListEvidences(ctx)
	return err
}

func actions() []connectors.Action {
	return []connectors.Action{
		{
			ID:          "query",
			Name:        "Query Flexi",
			Description: "Read rows from a Flexi evidence by canonical role. Use roles like 'customer', 'product', 'invoice_outgoing'. Returns rows with canonical-role keys (mapped from Czech column names).",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"role": map[string]any{
						"type":        "string",
						"description": "Canonical semantic role. Must be one mapped on this binding (call list_external_bindings to see roles).",
					},
					"filter": map[string]any{
						"type":        "object",
						"description": "Equality filter; keys are canonical roles (e.g. {\"vat_id_cz\":\"CZ12345678\"}).",
					},
					"fields": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Subset of canonical roles to project. Empty = all auto-mapped fields.",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Max rows; default 100, max 200.",
					},
				},
				"required": []string{"role"},
			},
			Handler: queryHandler,
		},
	}
}
