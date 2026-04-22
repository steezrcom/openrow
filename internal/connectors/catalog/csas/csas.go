// Package csas integrates with the Česká spořitelna George Developer
// API (Erste Group). Auth is OAuth 2.0 — the tenant registers an
// application at developers.erstegroup.com, obtains a client_id /
// client_secret / WEB-API key, runs through the one-time consent flow
// (scope "siblings.accounts") to get a long-lived refresh_token, and
// pastes the values here. Access tokens (~1 h) are minted on demand by
// exchanging the refresh token at the /token endpoint.
//
// This is the bank-account owner's own-data access path, not PSD2
// third-party AISP — no banking licence required.
//
// Docs: https://developers.erstegroup.com/docs/cs/cz/accounts
package csas

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
	prodTokenURL    = "https://bezpecnost.csas.cz/api/psd2/fl/oidc/v1/token"
	prodAccountsURL = "https://www.csas.cz/webapi/api/v3/accounts"

	sandboxTokenURL    = "https://webapi.developers.erstegroup.com/api/csas/sandbox/v1/sandbox-idp/token"
	sandboxAccountsURL = "https://webapi.developers.erstegroup.com/api/csas/public/sandbox/v3/accounts"
)

func init() {
	connectors.Register(&connectors.Connector{
		ID:          "csas",
		Name:        "Česká spořitelna",
		Description: "Read own-account transactions via the George Developer API.",
		Category:    "banking",
		Homepage:    "https://developers.erstegroup.com",
		Status:      connectors.StatusAvailable,
		Credentials: []connectors.CredentialField{
			{Name: "client_id", Label: "Client ID", Kind: connectors.FieldText, Required: true,
				Help: "From developers.erstegroup.com → My Applications."},
			{Name: "client_secret", Label: "Client Secret", Kind: connectors.FieldSecret, Required: true},
			{Name: "api_key", Label: "WEB-API key", Kind: connectors.FieldSecret, Required: true,
				Help: "Sent as the WEB-API-key header on every request. Often equal to the Client ID in sandbox; differs in production."},
			{Name: "refresh_token", Label: "Refresh token", Kind: connectors.FieldSecret, Required: true,
				Help: "Obtained once through the OAuth consent flow with scope \"siblings.accounts\"."},
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
			Description: "List all authorised business accounts (IBAN, currency, product, balance if exposed).",
			Schema:      map[string]any{"type": "object", "properties": map[string]any{}},
			Handler:     listAccounts,
		},
		{
			ID:          "list_transactions",
			Name:        "List transactions",
			Description: "List transactions for one account between fromDate and toDate (inclusive, YYYY-MM-DD). Paginated server-side. Newest first. Returns a compact projection with amount, counterparty, VS/KS/SS symbols and description.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"account_id": map[string]any{"type": "string", "description": "Account UUID as returned by list_accounts."},
					"from":       map[string]any{"type": "string", "description": "YYYY-MM-DD (inclusive)."},
					"to":         map[string]any{"type": "string", "description": "YYYY-MM-DD (inclusive). ČS rejects future dates; use yesterday at the latest."},
					"page":       map[string]any{"type": "integer", "description": "0-indexed; default 0."},
					"size":       map[string]any{"type": "integer", "description": "Max 100; default 100."},
				},
				"required": []string{"account_id", "from", "to"},
			},
			Handler: listTransactions,
		},
		{
			ID:          "get_account",
			Name:        "Get account",
			Description: "Fetch a single account by id.",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"id": map[string]any{"type": "string"}},
				"required":   []string{"id"},
			},
			Handler: getAccount,
		},
	}
}

// --- actions -------------------------------------------------------------

type accountSlim struct {
	ID       string  `json:"id"`
	IBAN     string  `json:"iban,omitempty"`
	Number   string  `json:"number,omitempty"`
	BankCode string  `json:"bank_code,omitempty"`
	Name     string  `json:"name"`
	Product  string  `json:"product,omitempty"`
	Currency string  `json:"currency"`
	Balance  float64 `json:"balance,omitempty"`
}

func listAccounts(ctx context.Context, creds map[string]string, _ json.RawMessage) (any, error) {
	body, err := call(ctx, creds, http.MethodGet, "", nil)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Accounts []map[string]any `json:"accounts"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode accounts: %w", err)
	}
	out := make([]accountSlim, 0, len(raw.Accounts))
	for _, a := range raw.Accounts {
		out = append(out, toAccount(a))
	}
	return out, nil
}

type listTxIn struct {
	AccountID string `json:"account_id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Page      int    `json:"page"`
	Size      int    `json:"size"`
}

type txSlim struct {
	ID           string  `json:"id"`
	Ref          string  `json:"ref,omitempty"`
	BookingDate  string  `json:"booking_date"`
	ValueDate    string  `json:"value_date,omitempty"`
	Direction    string  `json:"direction"` // "in" or "out"
	Amount       float64 `json:"amount"`    // positive for "in", negative for "out"
	Currency     string  `json:"currency"`
	Counterparty string  `json:"counterparty,omitempty"`
	CounterIBAN  string  `json:"counter_iban,omitempty"`
	CounterAcc   string  `json:"counter_account,omitempty"`
	CounterBank  string  `json:"counter_bank_code,omitempty"`
	VS           string  `json:"vs,omitempty"`
	KS           string  `json:"ks,omitempty"`
	SS           string  `json:"ss,omitempty"`
	Description  string  `json:"description,omitempty"`
}

func listTransactions(ctx context.Context, creds map[string]string, rawIn json.RawMessage) (any, error) {
	var in listTxIn
	if err := json.Unmarshal(rawIn, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	if strings.TrimSpace(in.AccountID) == "" {
		return nil, errors.New("account_id is required")
	}
	if _, err := time.Parse("2006-01-02", in.From); err != nil {
		return nil, fmt.Errorf("from must be YYYY-MM-DD: %w", err)
	}
	if _, err := time.Parse("2006-01-02", in.To); err != nil {
		return nil, fmt.Errorf("to must be YYYY-MM-DD: %w", err)
	}
	if in.Size <= 0 || in.Size > 100 {
		in.Size = 100
	}
	if in.Page < 0 {
		in.Page = 0
	}

	q := url.Values{}
	q.Set("fromDate", in.From)
	q.Set("toDate", in.To)
	q.Set("page", fmt.Sprintf("%d", in.Page))
	q.Set("size", fmt.Sprintf("%d", in.Size))
	path := "/" + url.PathEscape(in.AccountID) + "/transactions?" + q.Encode()

	body, err := call(ctx, creds, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Transactions []map[string]any `json:"transactions"`
		PageInfo     map[string]any   `json:"pageInfo"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode transactions: %w", err)
	}
	out := make([]txSlim, 0, len(raw.Transactions))
	for _, tx := range raw.Transactions {
		out = append(out, toTx(tx))
	}
	return map[string]any{
		"transactions": out,
		"page_info":    raw.PageInfo,
	}, nil
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
	body, err := call(ctx, creds, http.MethodGet, "/"+url.PathEscape(in.ID), nil)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode account: %w", err)
	}
	return toAccount(raw), nil
}

// --- mapping -------------------------------------------------------------

func toAccount(a map[string]any) accountSlim {
	out := accountSlim{
		ID:       stringOf(a["id"]),
		Name:     firstNonEmpty(stringOf(a["nameI18N"]), stringOf(a["name"])),
		Product:  firstNonEmpty(stringOf(a["productI18N"]), stringOf(a["product"])),
		Currency: stringOf(a["currency"]),
	}
	if out.ID == "" {
		out.ID = stringOf(a["resourceId"])
	}
	if ident, ok := a["identification"].(map[string]any); ok {
		out.IBAN = stringOf(ident["iban"])
		if other := stringOf(ident["other"]); other != "" {
			num, bank, _ := strings.Cut(other, "/")
			out.Number = strings.TrimSpace(num)
			out.BankCode = strings.TrimSpace(bank)
		}
	}
	if servicer, ok := a["servicer"].(map[string]any); ok {
		if out.BankCode == "" {
			out.BankCode = stringOf(servicer["bankCode"])
		}
	}
	if bal, ok := a["balance"].(map[string]any); ok {
		out.Balance = floatOf(bal["value"])
	}
	return out
}

func toTx(tx map[string]any) txSlim {
	out := txSlim{
		ID:          firstNonEmpty(stringOf(tx["transactionId"]), stringOf(tx["entryReference"])),
		Ref:         stringOf(tx["entryReference"]),
		BookingDate: stringOf(tx["bookingDate"]),
		ValueDate:   stringOf(tx["valueDate"]),
	}
	cd := strings.ToUpper(stringOf(tx["creditDebitIndicator"]))
	sign := 1.0
	out.Direction = "in"
	if cd == "DBIT" {
		sign = -1.0
		out.Direction = "out"
	}

	if amt, ok := tx["amount"].(map[string]any); ok {
		out.Amount = sign * floatOf(amt["value"])
		out.Currency = stringOf(amt["currency"])
	}

	if ed, ok := tx["entryDetails"].(map[string]any); ok {
		if td, ok := ed["transactionDetails"].(map[string]any); ok {
			if rp, ok := td["relatedParties"].(map[string]any); ok {
				// Counterparty = opposite side
				partyKey, acctKey := "creditor", "creditorAccount"
				if out.Direction == "in" {
					partyKey, acctKey = "debtor", "debtorAccount"
				}
				if p, ok := rp[partyKey].(map[string]any); ok {
					out.Counterparty = stringOf(p["name"])
				}
				if a, ok := rp[acctKey].(map[string]any); ok {
					if ident, ok := a["identification"].(map[string]any); ok {
						out.CounterIBAN = stringOf(ident["iban"])
						if other := stringOf(ident["other"]); other != "" {
							num, bank, _ := strings.Cut(other, "/")
							out.CounterAcc = strings.TrimSpace(num)
							out.CounterBank = strings.TrimSpace(bank)
						}
					}
				}
			}
			if ri, ok := td["remittanceInformation"].(map[string]any); ok {
				if s, ok := ri["structured"].(map[string]any); ok {
					if cr, ok := s["creditorReferenceInformation"].(map[string]any); ok {
						if refs, ok := cr["reference"].([]any); ok {
							for _, r := range refs {
								switch v := stringOf(r); {
								case strings.HasPrefix(v, "VS:"):
									out.VS = strings.TrimPrefix(v, "VS:")
								case strings.HasPrefix(v, "KS:"):
									out.KS = strings.TrimPrefix(v, "KS:")
								case strings.HasPrefix(v, "SS:"):
									out.SS = strings.TrimPrefix(v, "SS:")
								}
							}
						}
					}
				}
				out.Description = firstNonEmpty(
					stringOf(ri["unstructured"]),
					stringOf(td["additionalTransactionInformation"]),
				)
			}
		}
	}
	return out
}

// --- auth + http ---------------------------------------------------------

func test(ctx context.Context, creds map[string]string) error {
	_, err := call(ctx, creds, http.MethodGet, "", nil)
	return err
}

func baseURLs(creds map[string]string) (tokenURL, accountsURL string) {
	if strings.EqualFold(strings.TrimSpace(creds["environment"]), "sandbox") {
		return sandboxTokenURL, sandboxAccountsURL
	}
	return prodTokenURL, prodAccountsURL
}

func call(ctx context.Context, creds map[string]string, method, path string, body []byte) ([]byte, error) {
	apiKey := strings.TrimSpace(creds["api_key"])
	if apiKey == "" {
		apiKey = strings.TrimSpace(creds["client_id"])
	}
	if apiKey == "" {
		return nil, errors.New("csas: api_key or client_id required")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	token, err := acquireAccessToken(ctx, client, creds)
	if err != nil {
		return nil, err
	}

	_, accountsURL := baseURLs(creds)
	req, err := http.NewRequestWithContext(ctx, method, accountsURL+path, bytesReaderOrNil(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("WEB-API-key", apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("csas: %w", err)
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("csas: %s %s: %d %s", method, path, res.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return respBody, nil
}

func acquireAccessToken(ctx context.Context, client *http.Client, creds map[string]string) (string, error) {
	clientID := strings.TrimSpace(creds["client_id"])
	clientSecret := strings.TrimSpace(creds["client_secret"])
	refresh := strings.TrimSpace(creds["refresh_token"])
	if clientID == "" || clientSecret == "" || refresh == "" {
		return "", errors.New("csas: missing client_id / client_secret / refresh_token")
	}
	tokenURL, _ := baseURLs(creds)

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refresh)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("csas oauth: %w", err)
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("csas oauth: status %d: %s", res.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("csas oauth: decode: %w", err)
	}
	if out.AccessToken == "" {
		return "", errors.New("csas oauth: empty access token")
	}
	return out.AccessToken, nil
}

// --- helpers -------------------------------------------------------------

func bytesReaderOrNil(b []byte) io.Reader {
	if b == nil {
		return nil
	}
	return bytes.NewReader(b)
}

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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
