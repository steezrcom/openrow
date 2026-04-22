// Package uol integrates with ÚOL (Účetnictví Online, ucetnictvi.uol.cz),
// a Czech double-entry accounting SaaS. Auth is HTTP Basic with the
// user's e-mail as username and an API token as password; the token is
// generated in UOL settings under /api_tokens and requires the "REST API"
// permission on the user.
//
// The base URL is per-customer: https://{customer_id}.ucetnictvi.uol.cz/api.
// A shared demo host (https://test.demo.uol.cz/api) is available with
// demo@ucetnictvi-on-line.cz + the documented sandbox token.
//
// Docs: https://api.uol.cz
package uol

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
	prodHostTemplate = "https://%s.ucetnictvi.uol.cz/api"
	demoHost         = "https://test.demo.uol.cz/api"
)

func init() {
	connectors.Register(&connectors.Connector{
		ID:          "uol",
		Name:        "ÚOL",
		Description: "Účetnictví Online (UOL). Read invoices, receivables, bank movements and contacts; create sales invoices.",
		Category:    "billing",
		Homepage:    "https://www.uol.cz",
		Status:      connectors.StatusAvailable,
		Credentials: []connectors.CredentialField{
			{Name: "customer_id", Label: "Customer ID", Kind: connectors.FieldText, Required: true,
				Placeholder: "acme",
				Help:        "Subdomain assigned by UOL: the 'acme' in https://acme.ucetnictvi.uol.cz. Use 'test' for the demo host."},
			{Name: "email", Label: "User e-mail", Kind: connectors.FieldText, Required: true,
				Help: "The UOL user whose API token this is. Used as HTTP Basic username."},
			{Name: "api_token", Label: "API token", Kind: connectors.FieldSecret, Required: true,
				Help: "From UOL settings → /api_tokens. Requires REST API permission on the user."},
			{Name: "environment", Label: "Environment", Kind: connectors.FieldText, Required: false,
				Placeholder: "production",
				Help:        "Use 'demo' to force the shared sandbox host; blank defaults to production."},
		},
		Test:    test,
		Actions: actions(),
	})
}

func actions() []connectors.Action {
	return []connectors.Action{
		{
			ID:          "list_sales_invoices",
			Name:        "List sales invoices",
			Description: "List vystavené faktury, newest first. Filter by variable_symbol, issue_date range (issue_date_from/till, YYYY-MM-DD), external_id, or type (standard | proforma | corrective | penalty). Returns a compact projection.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"variable_symbol": map[string]any{"type": "string"},
					"external_id":     map[string]any{"type": "string", "description": "Your unique invoice ID."},
					"type":            map[string]any{"type": "string", "description": "standard | proforma | corrective | penalty"},
					"issue_date_from": map[string]any{"type": "string", "description": "YYYY-MM-DD (inclusive)."},
					"issue_date_till": map[string]any{"type": "string", "description": "YYYY-MM-DD (inclusive)."},
					"page":            map[string]any{"type": "integer"},
					"per_page":        map[string]any{"type": "integer", "description": "Max 250; default 100."},
				},
			},
			Handler: listSalesInvoices,
		},
		{
			ID:          "get_sales_invoice",
			Name:        "Get sales invoice",
			Description: "Fetch one sales invoice by its number (public_id / invoice_id, e.g. '2020000003').",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"invoice_id": map[string]any{"type": "string"}},
				"required":   []string{"invoice_id"},
			},
			Handler: getSalesInvoice,
		},
		{
			ID:          "list_receivables",
			Name:        "List receivables",
			Description: "List pohledávky (open / paid / all). Filter by state (unpaid | paid | all), due_date_to (YYYY-MM-DD; useful for 'overdue as of'), or contact_id. Rate-limited to 10 req / 10 s by UOL.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"state":        map[string]any{"type": "string", "description": "unpaid | paid | all. Default unpaid."},
					"due_date_to":  map[string]any{"type": "string", "description": "YYYY-MM-DD. Returns invoices due on/before this date."},
					"contact_id":   map[string]any{"type": "string"},
					"invoice_id":   map[string]any{"type": "string"},
					"page":         map[string]any{"type": "integer"},
					"per_page":     map[string]any{"type": "integer"},
				},
			},
			Handler: listReceivables,
		},
		{
			ID:          "list_bank_movements",
			Name:        "List bank movements",
			Description: "List bank movements as tracked by UOL (paired with invoices where applicable).",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"page":     map[string]any{"type": "integer"},
					"per_page": map[string]any{"type": "integer"},
				},
			},
			Handler: listBankMovements,
		},
		{
			ID:          "list_contacts",
			Name:        "List contacts",
			Description: "List counterparties. Filter by name, company_number (IČO), vatin (DIČ), or free-text 'q'.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"q":              map[string]any{"type": "string"},
					"name":           map[string]any{"type": "string"},
					"company_number": map[string]any{"type": "string"},
					"vatin":          map[string]any{"type": "string"},
					"page":           map[string]any{"type": "integer"},
					"per_page":       map[string]any{"type": "integer"},
				},
			},
			Handler: listContacts,
		},
		{
			ID:          "create_sales_invoice",
			Name:        "Create sales invoice",
			Description: "Create a sales invoice. Minimum body is {buyer_id, items:[{product_id, unit_price, quantity}]}. Pass type='proforma' for zálohová faktura. Pass status='confirmed' to issue immediately; omit for draft.",
			Mutates:     true,
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"body": map[string]any{
						"type":        "object",
						"description": "Raw UOL sales-invoice payload. See https://api.uol.cz under POST /v1/sales_invoices for the full schema and examples.",
					},
				},
				"required": []string{"body"},
			},
			Handler: createSalesInvoice,
		},
	}
}

// --- actions -------------------------------------------------------------

type invoiceSlim struct {
	ID             string  `json:"id"`
	PublicID       string  `json:"public_id,omitempty"`
	ExternalID     string  `json:"external_id,omitempty"`
	Type           string  `json:"type,omitempty"`
	Status         string  `json:"status,omitempty"`
	BuyerID        string  `json:"buyer_id,omitempty"`
	IssueDate      string  `json:"issue_date,omitempty"`
	DueDate        string  `json:"due_date,omitempty"`
	VariableSymbol string  `json:"variable_symbol,omitempty"`
	Total          float64 `json:"total,omitempty"`
	TotalCurrency  string  `json:"currency,omitempty"`
	Remaining      float64 `json:"remaining,omitempty"`
}

func listSalesInvoices(ctx context.Context, creds map[string]string, rawIn json.RawMessage) (any, error) {
	var in struct {
		VariableSymbol string `json:"variable_symbol"`
		ExternalID     string `json:"external_id"`
		Type           string `json:"type"`
		IssueDateFrom  string `json:"issue_date_from"`
		IssueDateTill  string `json:"issue_date_till"`
		Page           int    `json:"page"`
		PerPage        int    `json:"per_page"`
	}
	if len(rawIn) > 0 {
		if err := json.Unmarshal(rawIn, &in); err != nil {
			return nil, fmt.Errorf("parse input: %w", err)
		}
	}
	q := url.Values{}
	if in.VariableSymbol != "" {
		q.Set("variable_symbol", in.VariableSymbol)
	}
	if in.ExternalID != "" {
		q.Set("external_id", in.ExternalID)
	}
	if in.Type != "" {
		q.Set("type", in.Type)
	}
	if in.IssueDateFrom != "" {
		q.Set("issue_date_from", in.IssueDateFrom)
	}
	if in.IssueDateTill != "" {
		q.Set("issue_date_till", in.IssueDateTill)
	}
	setPage(q, in.Page, in.PerPage)

	body, err := call(ctx, creds, http.MethodGet, "/v1/sales_invoices", q, nil)
	if err != nil {
		return nil, err
	}
	return projectInvoices(body)
}

func getSalesInvoice(ctx context.Context, creds map[string]string, rawIn json.RawMessage) (any, error) {
	var in struct {
		InvoiceID string `json:"invoice_id"`
	}
	if err := json.Unmarshal(rawIn, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	if strings.TrimSpace(in.InvoiceID) == "" {
		return nil, errors.New("invoice_id is required")
	}
	body, err := call(ctx, creds, http.MethodGet, "/v1/sales_invoices/"+url.PathEscape(in.InvoiceID), nil, nil)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode invoice: %w", err)
	}
	return toInvoiceSlim(raw), nil
}

func listReceivables(ctx context.Context, creds map[string]string, rawIn json.RawMessage) (any, error) {
	var in struct {
		State      string `json:"state"`
		DueDateTo  string `json:"due_date_to"`
		ContactID  string `json:"contact_id"`
		InvoiceID  string `json:"invoice_id"`
		Page       int    `json:"page"`
		PerPage    int    `json:"per_page"`
	}
	if len(rawIn) > 0 {
		if err := json.Unmarshal(rawIn, &in); err != nil {
			return nil, fmt.Errorf("parse input: %w", err)
		}
	}
	q := url.Values{}
	if in.State == "" {
		in.State = "unpaid"
	}
	q.Set("state", in.State)
	if in.DueDateTo != "" {
		q.Set("due_date_to", in.DueDateTo)
	}
	if in.ContactID != "" {
		q.Set("contact_id", in.ContactID)
	}
	if in.InvoiceID != "" {
		q.Set("invoice_id", in.InvoiceID)
	}
	setPage(q, in.Page, in.PerPage)

	body, err := call(ctx, creds, http.MethodGet, "/v1/receivables", q, nil)
	if err != nil {
		return nil, err
	}
	return projectInvoices(body)
}

type bankMovementSlim struct {
	GID             string  `json:"gid,omitempty"`
	Date            string  `json:"date,omitempty"`
	Amount          float64 `json:"amount,omitempty"`
	Currency        string  `json:"currency,omitempty"`
	BankAccountID   string  `json:"bank_account_id,omitempty"`
	Counterparty    string  `json:"counterparty,omitempty"`
	CounterpartyIBAN string `json:"counterparty_iban,omitempty"`
	VariableSymbol  string  `json:"variable_symbol,omitempty"`
	ConstantSymbol  string  `json:"constant_symbol,omitempty"`
	SpecificSymbol  string  `json:"specific_symbol,omitempty"`
	Note            string  `json:"note,omitempty"`
	ExternalID      string  `json:"external_id,omitempty"`
}

func listBankMovements(ctx context.Context, creds map[string]string, rawIn json.RawMessage) (any, error) {
	var in struct {
		Page    int `json:"page"`
		PerPage int `json:"per_page"`
	}
	if len(rawIn) > 0 {
		_ = json.Unmarshal(rawIn, &in)
	}
	q := url.Values{}
	setPage(q, in.Page, in.PerPage)

	body, err := call(ctx, creds, http.MethodGet, "/v1/bank_movements", q, nil)
	if err != nil {
		return nil, err
	}
	rows, err := decodeArray(body)
	if err != nil {
		return nil, err
	}
	out := make([]bankMovementSlim, 0, len(rows))
	for _, r := range rows {
		out = append(out, bankMovementSlim{
			GID:              stringOf(r["gid"]),
			Date:             stringOf(r["date"]),
			Amount:           floatOf(r["amount"]),
			Currency:         stringOf(r["currency_id"]),
			BankAccountID:    stringOf(r["bank_account_id"]),
			Counterparty:     stringOf(r["contact_name"]),
			CounterpartyIBAN: stringOf(r["iban"]),
			VariableSymbol:   stringOf(r["variable_symbol"]),
			ConstantSymbol:   stringOf(r["constant_symbol"]),
			SpecificSymbol:   stringOf(r["specific_symbol"]),
			Note:             stringOf(r["note"]),
			ExternalID:       stringOf(r["external_id"]),
		})
	}
	return out, nil
}

type contactSlim struct {
	ID            string `json:"id,omitempty"`
	ExternalID    string `json:"external_id,omitempty"`
	Name          string `json:"name,omitempty"`
	CompanyNumber string `json:"ico,omitempty"`
	Vatin         string `json:"dic,omitempty"`
	Email         string `json:"email,omitempty"`
}

func listContacts(ctx context.Context, creds map[string]string, rawIn json.RawMessage) (any, error) {
	var in struct {
		Q             string `json:"q"`
		Name          string `json:"name"`
		CompanyNumber string `json:"company_number"`
		Vatin         string `json:"vatin"`
		Page          int    `json:"page"`
		PerPage       int    `json:"per_page"`
	}
	if len(rawIn) > 0 {
		_ = json.Unmarshal(rawIn, &in)
	}
	q := url.Values{}
	if in.Q != "" {
		q.Set("q", in.Q)
	}
	if in.Name != "" {
		q.Set("name", in.Name)
	}
	if in.CompanyNumber != "" {
		q.Set("company_number", in.CompanyNumber)
	}
	if in.Vatin != "" {
		q.Set("vatin", in.Vatin)
	}
	setPage(q, in.Page, in.PerPage)

	body, err := call(ctx, creds, http.MethodGet, "/v1/contacts", q, nil)
	if err != nil {
		return nil, err
	}
	rows, err := decodeArray(body)
	if err != nil {
		return nil, err
	}
	out := make([]contactSlim, 0, len(rows))
	for _, r := range rows {
		out = append(out, contactSlim{
			ID:            stringOf(r["id"]),
			ExternalID:    stringOf(r["external_id"]),
			Name:          stringOf(r["name"]),
			CompanyNumber: stringOf(r["company_number"]),
			Vatin:         stringOf(r["vatin"]),
			Email:         stringOf(r["email"]),
		})
	}
	return out, nil
}

func createSalesInvoice(ctx context.Context, creds map[string]string, rawIn json.RawMessage) (any, error) {
	var in struct {
		Body json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(rawIn, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	if len(in.Body) == 0 {
		return nil, errors.New("body is required")
	}
	body, err := call(ctx, creds, http.MethodPost, "/v1/sales_invoices", nil, in.Body)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode invoice: %w", err)
	}
	return toInvoiceSlim(raw), nil
}

// --- projection helpers --------------------------------------------------

func projectInvoices(body []byte) (any, error) {
	rows, err := decodeArray(body)
	if err != nil {
		return nil, err
	}
	out := make([]invoiceSlim, 0, len(rows))
	for _, r := range rows {
		out = append(out, toInvoiceSlim(r))
	}
	return out, nil
}

func toInvoiceSlim(r map[string]any) invoiceSlim {
	return invoiceSlim{
		ID:             firstNonEmpty(stringOf(r["public_id"]), stringOf(r["id"])),
		PublicID:       stringOf(r["public_id"]),
		ExternalID:     stringOf(r["external_id"]),
		Type:           stringOf(r["type"]),
		Status:         stringOf(r["status"]),
		BuyerID:        stringOf(r["buyer_id"]),
		IssueDate:      stringOf(r["issue_date"]),
		DueDate:        stringOf(r["due_date"]),
		VariableSymbol: stringOf(r["variable_symbol"]),
		Total:          floatOf(firstPresent(r, "total", "amount", "total_vat_inclusive")),
		TotalCurrency:  stringOf(r["currency_id"]),
		Remaining:      floatOf(firstPresent(r, "remaining", "unpaid_amount")),
	}
}

// decodeArray handles both the wrapped response shape ({ "data": [...] })
// and the bare array form — UOL uses both depending on the endpoint.
func decodeArray(body []byte) ([]map[string]any, error) {
	trim := bytes.TrimSpace(body)
	if len(trim) > 0 && trim[0] == '[' {
		var arr []map[string]any
		if err := json.Unmarshal(trim, &arr); err != nil {
			return nil, fmt.Errorf("decode: %w", err)
		}
		return arr, nil
	}
	var wrapped struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(trim, &wrapped); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return wrapped.Data, nil
}

// --- auth + http ---------------------------------------------------------

func test(ctx context.Context, creds map[string]string) error {
	_, err := call(ctx, creds, http.MethodGet, "/v1/ping", nil, nil)
	return err
}

func baseURL(creds map[string]string) (string, error) {
	if strings.EqualFold(strings.TrimSpace(creds["environment"]), "demo") {
		return demoHost, nil
	}
	customer := strings.TrimSpace(creds["customer_id"])
	if customer == "" {
		return "", errors.New("uol: customer_id required")
	}
	// Reject anything that could escape the hostname.
	if strings.ContainsAny(customer, "/?#@ ") {
		return "", fmt.Errorf("uol: invalid customer_id %q", customer)
	}
	return fmt.Sprintf(prodHostTemplate, customer), nil
}

func call(ctx context.Context, creds map[string]string, method, path string, query url.Values, body []byte) ([]byte, error) {
	email := strings.TrimSpace(creds["email"])
	token := strings.TrimSpace(creds["api_token"])
	if email == "" || token == "" {
		return nil, errors.New("uol: email and api_token required")
	}
	base, err := baseURL(creds)
	if err != nil {
		return nil, err
	}
	u := base + path
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
	req.SetBasicAuth(email, token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("uol: %w", err)
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode == http.StatusTooManyRequests {
		return nil, errors.New("uol: 429 rate-limited (cool-down 30s)")
	}
	if res.StatusCode == http.StatusUnauthorized {
		return nil, errors.New("uol: 401 — check email, api_token, and REST API permission on the user")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("uol: %s %s: %d %s", method, path, res.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return respBody, nil
}

// --- tiny helpers --------------------------------------------------------

func setPage(q url.Values, page, perPage int) {
	if page > 0 {
		q.Set("page", fmt.Sprintf("%d", page))
	}
	if perPage > 0 {
		if perPage > 250 {
			perPage = 250
		}
		q.Set("per_page", fmt.Sprintf("%d", perPage))
	}
}

func firstPresent(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func stringOf(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		// UOL returns variable_symbol as integer sometimes — stringify it.
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%v", x)
	}
	return fmt.Sprint(v)
}

func floatOf(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		var f float64
		_, _ = fmt.Sscanf(x, "%f", &f)
		return f
	}
	return 0
}
