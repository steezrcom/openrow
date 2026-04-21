// Package fakturoid integrates with Fakturoid, a Czech invoicing SaaS.
// API v3 uses OAuth 2.0 client credentials; tokens are issued at
// https://app.fakturoid.cz/api/v3/oauth/token and expire after ~2h. The
// API requires a User-Agent header with a contact e-mail so they can
// reach out about misuse or schema changes.
package fakturoid

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openrow/openrow/internal/connectors"
)

const (
	baseURL    = "https://app.fakturoid.cz/api/v3"
	userAgentF = "OpenRow (%s)"
)

func init() {
	connectors.Register(&connectors.Connector{
		ID:          "fakturoid",
		Name:        "Fakturoid",
		Description: "Czech invoicing SaaS. Sync issued invoices, clients and payments.",
		Category:    "billing",
		Homepage:    "https://www.fakturoid.cz",
		Status:      connectors.StatusAvailable,
		Credentials: []connectors.CredentialField{
			{Name: "slug", Label: "Account slug", Kind: connectors.FieldText, Required: true,
				Placeholder: "your-company",
				Help:        "The subdomain part of app.fakturoid.cz/<slug>."},
			{Name: "client_id", Label: "OAuth client ID", Kind: connectors.FieldText, Required: true,
				Help: "Create an API client under Nastavení → Uživatelský účet → API. Pick the 'Client credentials' flow."},
			{Name: "client_secret", Label: "OAuth client secret", Kind: connectors.FieldSecret, Required: true},
			{Name: "contact_email", Label: "Contact e-mail", Kind: connectors.FieldText, Required: true,
				Placeholder: "you@example.com",
				Help:        "Sent in the User-Agent header so Fakturoid can reach you if something goes wrong."},
		},
		Test: test,
	})
}

// test acquires a token and calls /account.json — the lightest authenticated
// endpoint — to confirm the credentials + slug are valid.
func test(ctx context.Context, creds map[string]string) error {
	slug := strings.TrimSpace(creds["slug"])
	if slug == "" {
		return errors.New("slug is required")
	}
	client := &http.Client{Timeout: 10 * time.Second}

	token, err := acquireToken(ctx, client, creds)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/accounts/%s/account.json", baseURL, url.PathEscape(slug)),
		nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", fmt.Sprintf(userAgentF, creds["contact_email"]))
	req.Header.Set("Accept", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fakturoid: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotFound {
		return fmt.Errorf("fakturoid: account slug %q not found", slug)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("fakturoid: unexpected status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func acquireToken(ctx context.Context, client *http.Client, creds map[string]string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/oauth/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(creds["client_id"], creds["client_secret"])
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", fmt.Sprintf(userAgentF, creds["contact_email"]))
	req.Header.Set("Accept", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fakturoid oauth: %w", err)
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode == http.StatusUnauthorized {
		return "", errors.New("fakturoid: invalid client_id or client_secret")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("fakturoid oauth: status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	var out struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("fakturoid oauth: decode: %w", err)
	}
	if out.AccessToken == "" {
		return "", errors.New("fakturoid oauth: empty access token")
	}
	return out.AccessToken, nil
}
