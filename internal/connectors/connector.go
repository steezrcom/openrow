// Package connectors defines the framework for third-party integrations
// (Fakturoid, Stripe, Revolut, etc.). Each integration is described by a
// Connector registered at init time; concrete wire-up (API clients, sync
// jobs) is added later when a connector graduates from "coming_soon" to
// "available".
package connectors

import (
	"context"
	"encoding/json"
)

// Status is the connector's readiness for end users.
type Status string

const (
	// StatusComingSoon: descriptor is registered and the UI renders a card,
	// but the configure flow is disabled — no working client exists yet.
	StatusComingSoon Status = "coming_soon"

	// StatusAvailable: fully implemented. The UI allows install/configure
	// and the stored credentials are used by the corresponding client.
	StatusAvailable Status = "available"
)

// FieldKind controls the HTML input type the settings UI renders for a
// credential field. "secret" means the value is masked and never sent back
// to the client after save.
type FieldKind string

const (
	FieldText   FieldKind = "text"
	FieldSecret FieldKind = "secret"
	FieldURL    FieldKind = "url"
)

// CredentialField describes one input in the connector's configure form.
type CredentialField struct {
	Name        string    `json:"name"`
	Label       string    `json:"label"`
	Kind        FieldKind `json:"kind"`
	Required    bool      `json:"required"`
	Placeholder string    `json:"placeholder,omitempty"`
	Help        string    `json:"help,omitempty"`
}

// Connector is the user-visible metadata for a third-party integration.
// Concrete implementations (sync, webhook handling, etc.) live next to
// the registration in their own file once built.
type Connector struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Category    string            `json:"category"`
	Homepage    string            `json:"homepage,omitempty"`
	Status      Status            `json:"status"`
	Credentials []CredentialField `json:"credentials"`

	// Test, if set, verifies the supplied credentials against the provider.
	// Return nil on success; the returned error message is shown to the user.
	// Leave nil for stubs and for connectors where verification isn't useful.
	Test func(ctx context.Context, creds map[string]string) error `json:"-"`

	// Actions are the verbs this connector exposes to the agent and to
	// flows. Each Action becomes a tool named
	// "connector.<connector_id>.<action_id>" when the tenant has this
	// connector installed and enabled.
	Actions []Action `json:"-"`
}

// Action is a callable verb on a connector. The handler receives the
// tenant's decrypted credentials and a JSON-encoded input matching Schema;
// it returns any JSON-marshallable value (compact shapes preferred — it's
// fed back to the LLM as a tool result).
type Action struct {
	ID          string
	Name        string
	Description string
	Mutates     bool
	Schema      map[string]any
	Handler     ActionHandler
}

// ActionHandler executes a single invocation of a connector action.
type ActionHandler func(ctx context.Context, creds map[string]string, input json.RawMessage) (any, error)
