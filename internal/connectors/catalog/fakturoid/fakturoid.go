// Package fakturoid integrates with Fakturoid, a Czech invoicing SaaS.
// API v3 uses OAuth 2.0 client credentials; tokens are issued at
// https://app.fakturoid.cz/api/v3/oauth/token and expire after ~2h. The
// API requires a User-Agent header with a contact e-mail so they can
// reach out about misuse or schema changes.
package fakturoid

import (
	"bytes"
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
		Test:    test,
		Actions: actions(),
	})
}

func actions() []connectors.Action {
	return []connectors.Action{
		{
			ID:   "list_invoices",
			Name: "List invoices",
			Description: "List invoices on the account. Filter by status (e.g. \"open\" for unpaid, \"paid\", \"overdue\"), " +
				"by invoice number substring, or by issue date (since YYYY-MM-DD). Returns a compact projection " +
				"(id, number, status, total, remaining_amount, due_on) to save context.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status": map[string]any{"type": "string", "description": "One of: open | paid | overdue | cancelled. Omit for any status."},
					"number": map[string]any{"type": "string", "description": "Substring search on invoice number."},
					"since":  map[string]any{"type": "string", "description": "Filter to invoices issued on/after this date (YYYY-MM-DD)."},
					"limit":  map[string]any{"type": "integer", "description": "Max rows to return; default 50, max 200."},
				},
			},
			Handler: listInvoices,
		},
		{
			ID:          "find_invoice_by_number",
			Name:        "Find invoice by number",
			Description: "Return exactly the invoice matching this number, or null if none. Use this to look up a specific invoice before acting on it.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"number": map[string]any{"type": "string", "description": "Exact invoice number, e.g. \"2024-0042\"."},
				},
				"required": []string{"number"},
			},
			Handler: findInvoiceByNumber,
		},
		{
			ID:          "mark_invoice_paid",
			Name:        "Mark invoice paid",
			Description: "Mark an invoice as paid in Fakturoid. Requires the invoice id (not the number). Idempotent: calling this on an already-paid invoice is a no-op.",
			Mutates:     true,
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "integer", "description": "Numeric Fakturoid invoice id."},
					"paid_on": map[string]any{"type": "string", "description": "Optional payment date (YYYY-MM-DD). Defaults to today in Fakturoid."},
				},
				"required": []string{"id"},
			},
			Handler: markInvoicePaid,
		},
	}
}

// --- actions -------------------------------------------------------------

type listInvoicesIn struct {
	Status string `json:"status"`
	Number string `json:"number"`
	Since  string `json:"since"`
	Limit  int    `json:"limit"`
}

type invoiceSlim struct {
	ID              int    `json:"id"`
	Number          string `json:"number"`
	Status          string `json:"status"`
	Total           string `json:"total"`
	RemainingAmount string `json:"remaining_amount"`
	DueOn           string `json:"due_on"`
	IssuedOn        string `json:"issued_on"`
}

func listInvoices(ctx context.Context, creds map[string]string, raw json.RawMessage) (any, error) {
	var in listInvoicesIn
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, fmt.Errorf("parse input: %w", err)
		}
	}
	if in.Limit <= 0 {
		in.Limit = 50
	}
	if in.Limit > 200 {
		in.Limit = 200
	}

	query := url.Values{}
	if in.Status != "" {
		query.Set("status", in.Status)
	}
	if in.Number != "" {
		query.Set("number", in.Number)
	}
	if in.Since != "" {
		query.Set("since", in.Since)
	}

	body, err := call(ctx, creds, http.MethodGet, "/invoices.json", query, nil)
	if err != nil {
		return nil, err
	}
	var full []map[string]any
	if err := json.Unmarshal(body, &full); err != nil {
		return nil, fmt.Errorf("decode invoices: %w", err)
	}
	out := make([]invoiceSlim, 0, min(len(full), in.Limit))
	for i, row := range full {
		if i >= in.Limit {
			break
		}
		out = append(out, invoiceSlim{
			ID:              intOf(row["id"]),
			Number:          stringOf(row["number"]),
			Status:          stringOf(row["status"]),
			Total:           stringOf(row["total"]),
			RemainingAmount: stringOf(row["remaining_amount"]),
			DueOn:           stringOf(row["due_on"]),
			IssuedOn:        stringOf(row["issued_on"]),
		})
	}
	return out, nil
}

type findByNumberIn struct {
	Number string `json:"number"`
}

func findInvoiceByNumber(ctx context.Context, creds map[string]string, raw json.RawMessage) (any, error) {
	var in findByNumberIn
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	if strings.TrimSpace(in.Number) == "" {
		return nil, errors.New("number is required")
	}
	query := url.Values{}
	query.Set("number", in.Number)
	body, err := call(ctx, creds, http.MethodGet, "/invoices.json", query, nil)
	if err != nil {
		return nil, err
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	for _, row := range rows {
		if stringOf(row["number"]) == in.Number {
			return invoiceSlim{
				ID:              intOf(row["id"]),
				Number:          stringOf(row["number"]),
				Status:          stringOf(row["status"]),
				Total:           stringOf(row["total"]),
				RemainingAmount: stringOf(row["remaining_amount"]),
				DueOn:           stringOf(row["due_on"]),
				IssuedOn:        stringOf(row["issued_on"]),
			}, nil
		}
	}
	return nil, nil
}

type markPaidIn struct {
	ID     int    `json:"id"`
	PaidOn string `json:"paid_on"`
}

func markInvoicePaid(ctx context.Context, creds map[string]string, raw json.RawMessage) (any, error) {
	var in markPaidIn
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	if in.ID == 0 {
		return nil, errors.New("id is required")
	}
	query := url.Values{}
	query.Set("event", "pay")
	if in.PaidOn != "" {
		query.Set("paid_on", in.PaidOn)
	}
	path := fmt.Sprintf("/invoices/%d/fire.json", in.ID)
	if _, err := call(ctx, creds, http.MethodPost, path, query, nil); err != nil {
		return nil, err
	}
	return map[string]any{"id": in.ID, "paid_on": in.PaidOn}, nil
}

// --- shared http plumbing ------------------------------------------------

// test acquires a token and calls /account.json — the lightest authenticated
// endpoint — to confirm the credentials + slug are valid.
func test(ctx context.Context, creds map[string]string) error {
	if _, err := call(ctx, creds, http.MethodGet, "/account.json", nil, nil); err != nil {
		return err
	}
	return nil
}

// call makes an authenticated request against the account-scoped endpoint.
// Path should start with "/" and is relative to /accounts/{slug}/. Returns
// the response body on 2xx, or a wrapped error otherwise.
func call(ctx context.Context, creds map[string]string, method, path string, query url.Values, body []byte) ([]byte, error) {
	slug := strings.TrimSpace(creds["slug"])
	if slug == "" {
		return nil, errors.New("slug is required")
	}
	client := &http.Client{Timeout: 15 * time.Second}

	token, err := acquireToken(ctx, client, creds)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("%s/accounts/%s%s", baseURL, url.PathEscape(slug), path)
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", fmt.Sprintf(userAgentF, creds["contact_email"]))
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fakturoid: %w", err)
	}
	defer res.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 512*1024))
	if res.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("fakturoid: %s %s: not found", method, path)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("fakturoid: %s %s: %d %s", method, path, res.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return respBody, nil
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

func intOf(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	}
	return 0
}
