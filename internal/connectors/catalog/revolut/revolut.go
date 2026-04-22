// Package revolut integrates with the Revolut Business API. Auth is
// OAuth 2.0 with a JWT client assertion: the tenant uploads a public
// key to the Revolut dashboard, goes through the one-time consent flow
// to obtain a long-lived refresh_token, then stores client_id, the
// private key, their issuer (the domain registered with Revolut), and
// the refresh_token here. Access tokens (40 min) are minted on demand
// by signing an RS256 JWT assertion and exchanging it + the refresh
// token at POST /api/1.0/auth/token.
//
// Docs: https://developer.revolut.com/docs/business/business-api
package revolut

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
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
	prodBaseURL    = "https://b2b.revolut.com/api/1.0"
	sandboxBaseURL = "https://sandbox-b2b.revolut.com/api/1.0"
	jwtAudience    = "https://revolut.com"
)

func init() {
	connectors.Register(&connectors.Connector{
		ID:          "revolut",
		Name:        "Revolut Business",
		Description: "Business banking. Import account balances and transactions.",
		Category:    "banking",
		Homepage:    "https://business.revolut.com",
		Status:      connectors.StatusAvailable,
		Credentials: []connectors.CredentialField{
			{Name: "client_id", Label: "Client ID", Kind: connectors.FieldText, Required: true,
				Help: "From Revolut Business → Settings → APIs after uploading your public key."},
			{Name: "issuer", Label: "Issuer", Kind: connectors.FieldText, Required: true,
				Placeholder: "your.domain.com",
				Help:        "JWT issuer registered with your API client — typically your domain."},
			{Name: "private_key", Label: "Private key (PEM)", Kind: connectors.FieldSecret, Required: true,
				Help: "Paste the full PEM including BEGIN/END lines. Must match the public key uploaded to Revolut."},
			{Name: "refresh_token", Label: "Refresh token", Kind: connectors.FieldSecret, Required: true,
				Help: "Obtained once via the consent flow. Valid for 90 days."},
			{Name: "environment", Label: "Environment", Kind: connectors.FieldText, Required: false,
				Placeholder: "production",
				Help:        "Use \"sandbox\" for the test environment; leave blank for production."},
		},
		Test:    test,
		Actions: actions(),
	})
}

func actions() []connectors.Action {
	return []connectors.Action{
		{
			ID:          "list_accounts",
			Name:        "List accounts",
			Description: "List all business accounts with balance, currency and state. Use this to see how much money is available and in which currency.",
			Schema:      map[string]any{"type": "object", "properties": map[string]any{}},
			Handler:     listAccounts,
		},
		{
			ID:   "list_transactions",
			Name: "List transactions",
			Description: "List transactions, newest first. Filter by account_id, date range (from/to YYYY-MM-DD), " +
				"or transaction type (transfer, card_payment, exchange, topup, fee, refund, tax, ...). " +
				"Returns a compact projection per transaction; legs are flattened to the first leg " +
				"(amount, currency, counterparty) to save context.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"account_id": map[string]any{"type": "string", "description": "Only transactions for this account."},
					"from":       map[string]any{"type": "string", "description": "Include transactions completed on/after this date (YYYY-MM-DD)."},
					"to":         map[string]any{"type": "string", "description": "Include transactions completed on/before this date (YYYY-MM-DD)."},
					"type":       map[string]any{"type": "string", "description": "Filter by type (transfer, card_payment, exchange, topup, fee, refund, tax, ...)."},
					"count":      map[string]any{"type": "integer", "description": "Max rows to return; default 100, max 1000."},
				},
			},
			Handler: listTransactions,
		},
		{
			ID:          "get_account",
			Name:        "Get account",
			Description: "Fetch a single business account by id. Returns balance, currency, state and IBAN/BIC if published.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string", "description": "Revolut account UUID."},
				},
				"required": []string{"id"},
			},
			Handler: getAccount,
		},
	}
}

// --- actions -------------------------------------------------------------

type accountSlim struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Balance  float64 `json:"balance"`
	Currency string  `json:"currency"`
	State    string  `json:"state"`
	Public   bool    `json:"public"`
}

func listAccounts(ctx context.Context, creds map[string]string, _ json.RawMessage) (any, error) {
	body, err := call(ctx, creds, http.MethodGet, "/accounts", nil)
	if err != nil {
		return nil, err
	}
	var raw []map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode accounts: %w", err)
	}
	out := make([]accountSlim, 0, len(raw))
	for _, r := range raw {
		out = append(out, accountSlim{
			ID:       stringOf(r["id"]),
			Name:     stringOf(r["name"]),
			Balance:  floatOf(r["balance"]),
			Currency: stringOf(r["currency"]),
			State:    stringOf(r["state"]),
			Public:   boolOf(r["public"]),
		})
	}
	return out, nil
}

type listTxIn struct {
	AccountID string `json:"account_id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Type      string `json:"type"`
	Count     int    `json:"count"`
}

type txSlim struct {
	ID             string  `json:"id"`
	Type           string  `json:"type"`
	State          string  `json:"state"`
	Reference      string  `json:"reference,omitempty"`
	CreatedAt      string  `json:"created_at"`
	CompletedAt    string  `json:"completed_at,omitempty"`
	AccountID      string  `json:"account_id"`
	Amount         float64 `json:"amount"`
	Currency       string  `json:"currency"`
	Description    string  `json:"description,omitempty"`
	Counterparty   string  `json:"counterparty,omitempty"`
	CounterpartyID string  `json:"counterparty_id,omitempty"`
}

func listTransactions(ctx context.Context, creds map[string]string, rawIn json.RawMessage) (any, error) {
	var in listTxIn
	if len(rawIn) > 0 {
		if err := json.Unmarshal(rawIn, &in); err != nil {
			return nil, fmt.Errorf("parse input: %w", err)
		}
	}
	if in.Count <= 0 {
		in.Count = 100
	}
	if in.Count > 1000 {
		in.Count = 1000
	}
	q := url.Values{}
	q.Set("count", fmt.Sprintf("%d", in.Count))
	if in.AccountID != "" {
		q.Set("account", in.AccountID)
	}
	if in.From != "" {
		q.Set("from", in.From)
	}
	if in.To != "" {
		q.Set("to", in.To)
	}
	if in.Type != "" {
		q.Set("type", in.Type)
	}

	body, err := call(ctx, creds, http.MethodGet, "/transactions?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var raw []map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode transactions: %w", err)
	}
	out := make([]txSlim, 0, len(raw))
	for _, r := range raw {
		s := txSlim{
			ID:          stringOf(r["id"]),
			Type:        stringOf(r["type"]),
			State:       stringOf(r["state"]),
			Reference:   stringOf(r["reference"]),
			CreatedAt:   stringOf(r["created_at"]),
			CompletedAt: stringOf(r["completed_at"]),
		}
		if legs, ok := r["legs"].([]any); ok && len(legs) > 0 {
			if leg, ok := legs[0].(map[string]any); ok {
				s.AccountID = stringOf(leg["account_id"])
				s.Amount = floatOf(leg["amount"])
				s.Currency = stringOf(leg["currency"])
				s.Description = stringOf(leg["description"])
				if cp, ok := leg["counterparty"].(map[string]any); ok {
					s.CounterpartyID = stringOf(cp["id"])
					if n := stringOf(cp["name"]); n != "" {
						s.Counterparty = n
					} else if n := stringOf(cp["account_name"]); n != "" {
						s.Counterparty = n
					}
				}
			}
		}
		out = append(out, s)
	}
	return out, nil
}

type getAccountIn struct {
	ID string `json:"id"`
}

func getAccount(ctx context.Context, creds map[string]string, rawIn json.RawMessage) (any, error) {
	var in getAccountIn
	if err := json.Unmarshal(rawIn, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	if strings.TrimSpace(in.ID) == "" {
		return nil, errors.New("id is required")
	}
	body, err := call(ctx, creds, http.MethodGet, "/accounts/"+url.PathEscape(in.ID), nil)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode account: %w", err)
	}
	return accountSlim{
		ID:       stringOf(raw["id"]),
		Name:     stringOf(raw["name"]),
		Balance:  floatOf(raw["balance"]),
		Currency: stringOf(raw["currency"]),
		State:    stringOf(raw["state"]),
		Public:   boolOf(raw["public"]),
	}, nil
}

// --- auth + http ---------------------------------------------------------

func test(ctx context.Context, creds map[string]string) error {
	_, err := call(ctx, creds, http.MethodGet, "/accounts", nil)
	return err
}

func baseURL(creds map[string]string) string {
	if strings.EqualFold(strings.TrimSpace(creds["environment"]), "sandbox") {
		return sandboxBaseURL
	}
	return prodBaseURL
}

func call(ctx context.Context, creds map[string]string, method, path string, body []byte) ([]byte, error) {
	client := &http.Client{Timeout: 15 * time.Second}

	token, err := acquireAccessToken(ctx, client, creds)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL(creds)+path, bytesReaderOrNil(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("revolut: %w", err)
	}
	defer res.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("revolut: %s %s: %d %s", method, path, res.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return respBody, nil
}

func acquireAccessToken(ctx context.Context, client *http.Client, creds map[string]string) (string, error) {
	clientID := strings.TrimSpace(creds["client_id"])
	issuer := strings.TrimSpace(creds["issuer"])
	refresh := strings.TrimSpace(creds["refresh_token"])
	if clientID == "" || issuer == "" || refresh == "" || creds["private_key"] == "" {
		return "", errors.New("revolut: missing credentials")
	}

	assertion, err := signAssertion(creds["private_key"], clientID, issuer)
	if err != nil {
		return "", err
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refresh)
	form.Set("client_id", clientID)
	form.Set("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer")
	form.Set("client_assertion", assertion)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL(creds)+"/auth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("revolut oauth: %w", err)
	}
	defer res.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusBadRequest {
		return "", fmt.Errorf("revolut oauth: %d %s", res.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("revolut oauth: status %d: %s", res.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var out struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("revolut oauth: decode: %w", err)
	}
	if out.AccessToken == "" {
		return "", errors.New("revolut oauth: empty access token")
	}
	return out.AccessToken, nil
}

// signAssertion builds and signs the RS256 JWT client assertion Revolut
// requires at the token endpoint. exp is 2 minutes out — short by design.
func signAssertion(privateKeyPEM, clientID, issuer string) (string, error) {
	key, err := parseRSAPrivateKey(privateKeyPEM)
	if err != nil {
		return "", err
	}
	header := map[string]any{"alg": "RS256", "typ": "JWT"}
	claims := map[string]any{
		"iss": issuer,
		"sub": clientID,
		"aud": jwtAudience,
		"exp": time.Now().Add(2 * time.Minute).Unix(),
	}
	h, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	c, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := b64(h) + "." + b64(c)

	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("revolut: sign assertion: %w", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func parseRSAPrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("revolut: private key is not valid PEM")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	anyKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("revolut: parse private key: %w", err)
	}
	rsaKey, ok := anyKey.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("revolut: private key is not RSA")
	}
	return rsaKey, nil
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func bytesReaderOrNil(b []byte) io.Reader {
	if b == nil {
		return nil
	}
	return bytes.NewReader(b)
}

// --- tiny helpers --------------------------------------------------------

func stringOf(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func floatOf(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	}
	return 0
}

func boolOf(v any) bool {
	b, _ := v.(bool)
	return b
}
