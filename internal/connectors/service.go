package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openrow/openrow/internal/secrets"
)

// Config is the stored per-tenant state for one connector. Credentials are
// returned decrypted; callers that serialize to HTTP must strip or redact
// any FieldSecret fields (see Safe).
type Config struct {
	ID          string            `json:"id"`
	TenantID    string            `json:"tenant_id"`
	ConnectorID string            `json:"connector_id"`
	Credentials map[string]string `json:"-"`
	Enabled     bool              `json:"enabled"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// SafeConfig is the client-facing view: no plaintext secrets, just which
// fields are populated so the UI can show filled/empty status.
type SafeConfig struct {
	ID            string          `json:"id"`
	ConnectorID   string          `json:"connector_id"`
	Enabled       bool            `json:"enabled"`
	Fields        map[string]any  `json:"fields"`
	FieldsPresent map[string]bool `json:"fields_present"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

func (c *Config) Safe(descriptor *Connector) SafeConfig {
	fields := make(map[string]any, len(c.Credentials))
	present := make(map[string]bool, len(c.Credentials))
	if descriptor != nil {
		for _, f := range descriptor.Credentials {
			v, ok := c.Credentials[f.Name]
			present[f.Name] = ok && v != ""
			if f.Kind == FieldSecret {
				continue
			}
			if ok {
				fields[f.Name] = v
			}
		}
	}
	return SafeConfig{
		ID:            c.ID,
		ConnectorID:   c.ConnectorID,
		Enabled:       c.Enabled,
		Fields:        fields,
		FieldsPresent: present,
		CreatedAt:     c.CreatedAt,
		UpdatedAt:     c.UpdatedAt,
	}
}

type Service struct {
	pool *pgxpool.Pool
	enc  *secrets.Encrypter
}

func NewService(pool *pgxpool.Pool, enc *secrets.Encrypter) *Service {
	return &Service{pool: pool, enc: enc}
}

var (
	ErrUnknownConnector = errors.New("unknown connector")
	ErrNotConfigured    = errors.New("connector not configured for this workspace")
)

// List returns all connector configs stored for a tenant.
func (s *Service) List(ctx context.Context, tenantID string) ([]*Config, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, connector_id, COALESCE(credentials, ''::bytea), enabled, created_at, updated_at
		FROM openrow.connector_configs
		WHERE tenant_id = $1
		ORDER BY connector_id`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*Config, 0)
	for rows.Next() {
		c, err := s.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Get returns one config by connector id, or ErrNotConfigured.
func (s *Service) Get(ctx context.Context, tenantID, connectorID string) (*Config, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, connector_id, COALESCE(credentials, ''::bytea), enabled, created_at, updated_at
		FROM openrow.connector_configs
		WHERE tenant_id = $1 AND connector_id = $2`, tenantID, connectorID)
	c, err := s.scan(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotConfigured
	}
	return c, err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (s *Service) scan(r rowScanner) (*Config, error) {
	var (
		c    Config
		blob []byte
	)
	if err := r.Scan(&c.ID, &c.TenantID, &c.ConnectorID, &blob, &c.Enabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	c.Credentials = map[string]string{}
	if len(blob) > 0 {
		pt, err := s.enc.Decrypt(blob)
		if err != nil {
			return nil, fmt.Errorf("decrypt credentials: %w", err)
		}
		if err := json.Unmarshal(pt, &c.Credentials); err != nil {
			return nil, fmt.Errorf("unmarshal credentials: %w", err)
		}
	}
	return &c, nil
}

// UpsertInput carries the patch from the HTTP layer. Secret fields absent
// from the map (or with value == nil) are left unchanged on update; setting
// a secret to "" explicitly clears it.
type UpsertInput struct {
	// Fields maps credential-field name → raw value. Use nil for "leave
	// unchanged" on secret fields. Non-secret fields are always set from the
	// input (missing keys clear them).
	Fields  map[string]*string
	Enabled *bool
}

// Upsert writes a config for (tenant, connector). Validates required fields
// and encrypts the merged credential blob in one step.
func (s *Service) Upsert(ctx context.Context, tenantID, connectorID string, in UpsertInput) (*Config, error) {
	descriptor := Get(connectorID)
	if descriptor == nil {
		return nil, ErrUnknownConnector
	}
	if descriptor.Status != StatusAvailable {
		return nil, fmt.Errorf("connector %q is not yet available", connectorID)
	}

	existing, err := s.Get(ctx, tenantID, connectorID)
	if err != nil && !errors.Is(err, ErrNotConfigured) {
		return nil, err
	}

	merged := map[string]string{}
	if existing != nil {
		for k, v := range existing.Credentials {
			merged[k] = v
		}
	}

	for _, f := range descriptor.Credentials {
		raw, present := in.Fields[f.Name]
		if f.Kind == FieldSecret {
			if !present || raw == nil {
				continue
			}
			if *raw == "" {
				delete(merged, f.Name)
				continue
			}
			merged[f.Name] = *raw
			continue
		}
		if !present || raw == nil {
			delete(merged, f.Name)
			continue
		}
		merged[f.Name] = *raw
	}

	for _, f := range descriptor.Credentials {
		if f.Required && merged[f.Name] == "" {
			return nil, fmt.Errorf("%s is required", f.Label)
		}
	}

	encoded, err := json.Marshal(merged)
	if err != nil {
		return nil, err
	}
	blob, err := s.enc.Encrypt(encoded)
	if err != nil {
		return nil, fmt.Errorf("encrypt credentials: %w", err)
	}

	enabled := true
	if existing != nil {
		enabled = existing.Enabled
	}
	if in.Enabled != nil {
		enabled = *in.Enabled
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO openrow.connector_configs (tenant_id, connector_id, credentials, enabled)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (tenant_id, connector_id) DO UPDATE
		SET credentials = EXCLUDED.credentials,
		    enabled     = EXCLUDED.enabled,
		    updated_at  = now()`,
		tenantID, connectorID, blob, enabled)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, connectorID)
}

// Test calls the connector's Test hook against the stored config.
// Returns ErrNotConfigured if no config exists, ErrTestNotSupported if the
// connector descriptor has no Test func, or whatever the hook returns.
func (s *Service) Test(ctx context.Context, tenantID, connectorID string) error {
	descriptor := Get(connectorID)
	if descriptor == nil {
		return ErrUnknownConnector
	}
	if descriptor.Test == nil {
		return ErrTestNotSupported
	}
	cfg, err := s.Get(ctx, tenantID, connectorID)
	if err != nil {
		return err
	}
	return descriptor.Test(ctx, cfg.Credentials)
}

var ErrTestNotSupported = errors.New("connector does not support credential tests")

// Delete removes a tenant's config for connectorID.
func (s *Service) Delete(ctx context.Context, tenantID, connectorID string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM openrow.connector_configs WHERE tenant_id = $1 AND connector_id = $2`,
		tenantID, connectorID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotConfigured
	}
	return nil
}
