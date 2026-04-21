package llm

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openrow/openrow/internal/secrets"
)

// Config is the resolved LLM configuration for a tenant.
// APIKey is decrypted; empty string is valid (local endpoints with no auth).
type Config struct {
	TenantID  string    `json:"tenant_id"`
	Provider  string    `json:"provider"`
	BaseURL   string    `json:"base_url"`
	APIKey    string    `json:"-"`
	Model     string    `json:"model"`
	Source    string    `json:"source"` // "tenant" | "env-fallback"
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`

	// Last self-test outcome (nil when never tested).
	LastTestedAt    *time.Time
	LastTestOK      *bool
	LastTestToolsOK *bool
	LastTestMessage string
}

// Safe returns a copy with the api key redacted for serialization to clients.
func (c *Config) Safe() SafeConfig {
	if c == nil {
		return SafeConfig{}
	}
	return SafeConfig{
		Provider:        c.Provider,
		BaseURL:         c.BaseURL,
		Model:           c.Model,
		HasAPIKey:       c.APIKey != "",
		Source:          c.Source,
		UpdatedAt:       c.UpdatedAt,
		LastTestedAt:    c.LastTestedAt,
		LastTestOK:      c.LastTestOK,
		LastTestToolsOK: c.LastTestToolsOK,
		LastTestMessage: c.LastTestMessage,
	}
}

type SafeConfig struct {
	Provider        string     `json:"provider"`
	BaseURL         string     `json:"base_url"`
	Model           string     `json:"model"`
	HasAPIKey       bool       `json:"has_api_key"`
	Source          string     `json:"source"`
	UpdatedAt       time.Time  `json:"updated_at,omitempty"`
	LastTestedAt    *time.Time `json:"last_tested_at,omitempty"`
	LastTestOK      *bool      `json:"last_test_ok,omitempty"`
	LastTestToolsOK *bool      `json:"last_test_tools_ok,omitempty"`
	LastTestMessage string     `json:"last_test_message,omitempty"`
}

// SetInput is the patch from HTTP/agent.
type SetInput struct {
	Provider string  `json:"provider"`
	BaseURL  string  `json:"base_url"`
	APIKey   *string `json:"api_key,omitempty"` // nil = leave unchanged; "" = clear
	Model    string  `json:"model"`
}

// Service persists per-tenant LLM configs with encrypted API keys, and falls
// back to a single env-var default (ANTHROPIC_API_KEY) when a tenant has none.
// The fallback keeps existing dev setups working without per-tenant config.
type Service struct {
	pool         *pgxpool.Pool
	enc          *secrets.Encrypter
	fallbackAPI  string // e.g. ANTHROPIC_API_KEY, empty means no fallback
}

func NewService(pool *pgxpool.Pool, enc *secrets.Encrypter, fallbackAPIKey string) *Service {
	return &Service{pool: pool, enc: enc, fallbackAPI: fallbackAPIKey}
}

var urlRe = regexp.MustCompile(`^https?://`)

func normalizeBaseURL(u string) string {
	u = strings.TrimSpace(u)
	return strings.TrimRight(u, "/")
}

// Resolve returns the effective config for a tenant.
// Priority: stored tenant config → ANTHROPIC_API_KEY env fallback → error.
func (s *Service) Resolve(ctx context.Context, tenantID string) (*Config, error) {
	cfg, err := s.get(ctx, tenantID)
	if err == nil {
		return cfg, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if s.fallbackAPI != "" {
		return &Config{
			TenantID: tenantID,
			Provider: "anthropic",
			BaseURL:  "https://api.anthropic.com/v1",
			APIKey:   s.fallbackAPI,
			Model:    "claude-sonnet-4-5",
			Source:   "env-fallback",
		}, nil
	}
	return nil, ErrNotConfigured
}

var ErrNotConfigured = errors.New("llm not configured for this workspace")

// GetSafe returns the client-safe view (no api key) for the tenant, or the
// env-fallback placeholder. Callers should not treat a missing row as an error.
func (s *Service) GetSafe(ctx context.Context, tenantID string) (*SafeConfig, error) {
	cfg, err := s.Resolve(ctx, tenantID)
	if errors.Is(err, ErrNotConfigured) {
		return &SafeConfig{}, nil
	}
	if err != nil {
		return nil, err
	}
	safe := cfg.Safe()
	return &safe, nil
}

func (s *Service) get(ctx context.Context, tenantID string) (*Config, error) {
	var (
		c         Config
		key       []byte
		testedAt  *time.Time
		testOK    *bool
		testTools *bool
		testMsg   *string
	)
	err := s.pool.QueryRow(ctx, `
		SELECT tenant_id, provider, base_url, COALESCE(api_key, ''::bytea), model,
		       created_at, updated_at,
		       last_tested_at, last_test_ok, last_test_tools_ok, last_test_message
		FROM openrow.llm_configs
		WHERE tenant_id = $1`,
		tenantID,
	).Scan(&c.TenantID, &c.Provider, &c.BaseURL, &key, &c.Model,
		&c.CreatedAt, &c.UpdatedAt,
		&testedAt, &testOK, &testTools, &testMsg)
	if err != nil {
		return nil, err
	}
	c.Source = "tenant"
	c.LastTestedAt = testedAt
	c.LastTestOK = testOK
	c.LastTestToolsOK = testTools
	if testMsg != nil {
		c.LastTestMessage = *testMsg
	}
	if len(key) > 0 {
		pt, err := s.enc.Decrypt(key)
		if err != nil {
			return nil, fmt.Errorf("decrypt api key: %w", err)
		}
		c.APIKey = string(pt)
	}
	return &c, nil
}

// RecordTestResult saves the outcome of a self-test against the tenant's config.
// No-op if the tenant has no persisted row.
func (s *Service) RecordTestResult(ctx context.Context, tenantID string, r TestResult) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE openrow.llm_configs
		SET last_tested_at = now(),
		    last_test_ok = $2,
		    last_test_tools_ok = $3,
		    last_test_message = NULLIF($4, '')
		WHERE tenant_id = $1`,
		tenantID, r.OK, r.ToolsOK, r.Message)
	return err
}

// SelfTest runs Test() against the tenant's saved config and records the result.
// Returns ErrNotConfigured if the tenant has no saved config (env-fallback is
// deliberately excluded — there's nothing to save the result onto).
func (s *Service) SelfTest(ctx context.Context, tenantID string) (*TestResult, error) {
	cfg, err := s.get(ctx, tenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotConfigured
	}
	if err != nil {
		return nil, err
	}
	result := Test(ctx, cfg.BaseURL, cfg.APIKey, cfg.Model)
	if recErr := s.RecordTestResult(ctx, tenantID, result); recErr != nil {
		return &result, fmt.Errorf("test ran but recording failed: %w", recErr)
	}
	return &result, nil
}

// Set upserts the tenant's config. If in.APIKey is nil, the key is left
// unchanged (useful when the UI lets users edit URL/model without re-entering
// the secret). If in.APIKey is a non-nil empty string, the key is cleared.
func (s *Service) Set(ctx context.Context, tenantID string, in SetInput) (*SafeConfig, error) {
	if in.Provider == "" {
		return nil, errors.New("provider is required")
	}
	in.BaseURL = normalizeBaseURL(in.BaseURL)
	if in.BaseURL == "" || !urlRe.MatchString(in.BaseURL) {
		return nil, errors.New("base_url must start with http:// or https://")
	}
	if strings.TrimSpace(in.Model) == "" {
		return nil, errors.New("model is required")
	}

	var encrypted []byte
	keyUpdate := false
	if in.APIKey != nil {
		keyUpdate = true
		if *in.APIKey != "" {
			ct, err := s.enc.Encrypt([]byte(*in.APIKey))
			if err != nil {
				return nil, fmt.Errorf("encrypt api key: %w", err)
			}
			encrypted = ct
		}
	}

	var query string
	var args []interface{}
	if keyUpdate {
		query = `
			INSERT INTO openrow.llm_configs (tenant_id, provider, base_url, api_key, model)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (tenant_id) DO UPDATE
			SET provider = EXCLUDED.provider,
				base_url = EXCLUDED.base_url,
				api_key  = EXCLUDED.api_key,
				model    = EXCLUDED.model,
				updated_at = now()`
		args = []interface{}{tenantID, in.Provider, in.BaseURL, encrypted, in.Model}
	} else {
		query = `
			INSERT INTO openrow.llm_configs (tenant_id, provider, base_url, api_key, model)
			VALUES ($1, $2, $3, NULL, $4)
			ON CONFLICT (tenant_id) DO UPDATE
			SET provider = EXCLUDED.provider,
				base_url = EXCLUDED.base_url,
				model    = EXCLUDED.model,
				updated_at = now()`
		args = []interface{}{tenantID, in.Provider, in.BaseURL, in.Model}
	}

	if _, err := s.pool.Exec(ctx, query, args...); err != nil {
		return nil, err
	}
	safe, err := s.GetSafe(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return safe, nil
}

// Delete removes a tenant's config row entirely; the env fallback (if any)
// will then take over.
func (s *Service) Delete(ctx context.Context, tenantID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM openrow.llm_configs WHERE tenant_id = $1`, tenantID)
	return err
}
