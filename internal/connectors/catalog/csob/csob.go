// Package csob integrates with ČSOB's account-information API for
// own-account access. The tenant registers an application in ČSOB's API
// Portal, runs through the OAuth consent flow once to obtain a
// refresh_token, and pastes the values here. Access tokens are minted
// on demand.
//
// The endpoints follow the Berlin Group NextGenPSD2 v1 shape that ČSOB
// uses (/accounts, /accounts/{id}/transactions). Base URLs are
// configurable because ČSOB exposes both a partner sandbox and a
// production host, and the exact path prefix has changed historically.
//
// Docs: https://api.csob.cz (API Portal, login required)
package csob

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
	defaultProdBase    = "https://api.csob.cz"
	defaultSandboxBase = "https://api.csob.cz/sandbox"
	accountsPath       = "/aisp/v1/accounts"
	tokenPath          = "/oauth/token"
)

func init() {
	connectors.Register(&connectors.Connector{
		ID:          "csob",
		Name:        "ČSOB",
		Description: "Read own-account transactions via the ČSOB API Portal (OAuth 2.0).",
		Category:    "banking",
		Homepage:    "https://api.csob.cz",
		Status:      connectors.StatusAvailable,
		Credentials: []connectors.CredentialField{
			{Name: "client_id", Label: "Client ID", Kind: connectors.FieldText, Required: true,
				Help: "From ČSOB API Portal → My Applications."},
			{Name: "client_secret", Label: "Client Secret", Kind: connectors.FieldSecret, Required: true},
			{Name: "refresh_token", Label: "Refresh token", Kind: connectors.FieldSecret, Required: true,
				Help: "Obtained once via the OAuth consent flow for the accounts scope."},
			{Name: "base_url", Label: "API base URL", Kind: connectors.FieldURL, Required: false,
				Placeholder: defaultProdBase,
				Help:        "Leave blank for production (" + defaultProdBase + "). Set manually if ČSOB assigned you a different host."},
			{Name: "environment", Label: "Environment", Kind: connectors.FieldText, Required: false,
				Placeholder: "production",
				Help:        "Use \"sandbox\" to default to the sandbox host instead of production."},
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
			Description: "List consented ČSOB accounts (IBAN, currency, balance if exposed).",
			Schema:      map[string]any{"type": "object", "properties": map[string]any{}},
			Handler:     listAccounts,
		},
		{
			ID:          "list_transactions",
			Name:        "List transactions",
			Description: "List transactions for one account between dateFrom and dateTo (inclusive, YYYY-MM-DD). Returns a compact projection with amount, counterparty and Czech payment symbols (VS/KS/SS) extracted from remittance info.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"account_id":  map[string]any{"type": "string", "description": "Account ID as returned by list_accounts."},
					"from":        map[string]any{"type": "string", "description": "YYYY-MM-DD (inclusive)."},
					"to":          map[string]any{"type": "string", "description": "YYYY-MM-DD (inclusive)."},
					"booking_status": map[string]any{"type": "string", "description": "booked (default) | pending | both."},
				},
				"required": []string{"account_id", "from", "to"},
			},
			Handler: listTransactions,
		},
	}
}

// --- actions -------------------------------------------------------------

type accountSlim struct {
	ID       string  `json:"id"`
	IBAN     string  `json:"iban,omitempty"`
	BBAN     string  `json:"bban,omitempty"`
	Name     string  `json:"name,omitempty"`
	Product  string  `json:"product,omitempty"`
	Currency string  `json:"currency"`
	Balance  float64 `json:"balance,omitempty"`
}

func listAccounts(ctx context.Context, creds map[string]string, _ json.RawMessage) (any, error) {
	body, err := call(ctx, creds, http.MethodGet, accountsPath+"?withBalance=true", nil)
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
	AccountID     string `json:"account_id"`
	From          string `json:"from"`
	To            string `json:"to"`
	BookingStatus string `json:"booking_status"`
}

type txSlim struct {
	ID           string  `json:"id"`
	BookingDate  string  `json:"booking_date"`
	ValueDate    string  `json:"value_date,omitempty"`
	Direction    string  `json:"direction"`
	Amount       float64 `json:"amount"`
	Currency     string  `json:"currency"`
	Counterparty string  `json:"counterparty,omitempty"`
	CounterIBAN  string  `json:"counter_iban,omitempty"`
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
	bookingStatus := strings.ToLower(strings.TrimSpace(in.BookingStatus))
	if bookingStatus == "" {
		bookingStatus = "booked"
	}

	q := url.Values{}
	q.Set("dateFrom", in.From)
	q.Set("dateTo", in.To)
	q.Set("bookingStatus", bookingStatus)
	path := accountsPath + "/" + url.PathEscape(in.AccountID) + "/transactions?" + q.Encode()

	body, err := call(ctx, creds, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	// Berlin Group groups transactions under transactions.{booked, pending}.
	var raw struct {
		Transactions struct {
			Booked  []map[string]any `json:"booked"`
			Pending []map[string]any `json:"pending"`
		} `json:"transactions"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode transactions: %w", err)
	}
	out := make([]txSlim, 0, len(raw.Transactions.Booked)+len(raw.Transactions.Pending))
	for _, tx := range raw.Transactions.Booked {
		out = append(out, toTx(tx))
	}
	if bookingStatus == "both" || bookingStatus == "pending" {
		for _, tx := range raw.Transactions.Pending {
			out = append(out, toTx(tx))
		}
	}
	return out, nil
}

// --- mapping -------------------------------------------------------------

func toAccount(a map[string]any) accountSlim {
	out := accountSlim{
		ID:       firstNonEmpty(stringOf(a["resourceId"]), stringOf(a["id"])),
		IBAN:     stringOf(a["iban"]),
		BBAN:     stringOf(a["bban"]),
		Name:     firstNonEmpty(stringOf(a["name"]), stringOf(a["ownerName"])),
		Product:  stringOf(a["product"]),
		Currency: stringOf(a["currency"]),
	}
	// Berlin Group balances: array under "balances".
	if bals, ok := a["balances"].([]any); ok && len(bals) > 0 {
		if b, ok := bals[0].(map[string]any); ok {
			if amt, ok := b["balanceAmount"].(map[string]any); ok {
				out.Balance = floatOf(amt["amount"])
				if out.Currency == "" {
					out.Currency = stringOf(amt["currency"])
				}
			}
		}
	}
	return out
}

func toTx(tx map[string]any) txSlim {
	out := txSlim{
		ID:          firstNonEmpty(stringOf(tx["transactionId"]), stringOf(tx["entryReference"])),
		BookingDate: stringOf(tx["bookingDate"]),
		ValueDate:   stringOf(tx["valueDate"]),
	}
	amount := 0.0
	currency := ""
	if ta, ok := tx["transactionAmount"].(map[string]any); ok {
		amount = floatOf(ta["amount"])
		currency = stringOf(ta["currency"])
	}
	out.Amount = amount
	out.Currency = currency
	if amount < 0 {
		out.Direction = "out"
	} else {
		out.Direction = "in"
	}
	// creditDebitIndicator wins if present.
	switch strings.ToUpper(stringOf(tx["creditDebitIndicator"])) {
	case "DBIT":
		if amount > 0 {
			out.Amount = -amount
		}
		out.Direction = "out"
	case "CRDT":
		if amount < 0 {
			out.Amount = -amount
		}
		out.Direction = "in"
	}

	if out.Direction == "in" {
		out.Counterparty = firstNonEmpty(stringOf(tx["debtorName"]), stringOf(tx["ultimateDebtor"]))
		if da, ok := tx["debtorAccount"].(map[string]any); ok {
			out.CounterIBAN = stringOf(da["iban"])
		}
	} else {
		out.Counterparty = firstNonEmpty(stringOf(tx["creditorName"]), stringOf(tx["ultimateCreditor"]))
		if ca, ok := tx["creditorAccount"].(map[string]any); ok {
			out.CounterIBAN = stringOf(ca["iban"])
		}
	}

	out.Description = firstNonEmpty(
		stringOf(tx["remittanceInformationUnstructured"]),
		stringOf(tx["additionalInformation"]),
	)
	if s := stringOf(tx["remittanceInformationStructured"]); s != "" {
		parseSymbols(s, &out)
	}
	if arr, ok := tx["remittanceInformationStructuredArray"].([]any); ok {
		for _, r := range arr {
			parseSymbols(stringOf(r), &out)
		}
	}
	return out
}

func parseSymbols(s string, out *txSlim) {
	for _, part := range strings.Fields(strings.NewReplacer(",", " ", ";", " ").Replace(s)) {
		switch {
		case strings.HasPrefix(part, "VS:"):
			out.VS = strings.TrimPrefix(part, "VS:")
		case strings.HasPrefix(part, "KS:"):
			out.KS = strings.TrimPrefix(part, "KS:")
		case strings.HasPrefix(part, "SS:"):
			out.SS = strings.TrimPrefix(part, "SS:")
		}
	}
}

// --- auth + http ---------------------------------------------------------

func test(ctx context.Context, creds map[string]string) error {
	_, err := call(ctx, creds, http.MethodGet, accountsPath, nil)
	return err
}

func baseURL(creds map[string]string) string {
	if explicit := strings.TrimSpace(creds["base_url"]); explicit != "" {
		return strings.TrimRight(explicit, "/")
	}
	if strings.EqualFold(strings.TrimSpace(creds["environment"]), "sandbox") {
		return defaultSandboxBase
	}
	return defaultProdBase
}

func call(ctx context.Context, creds map[string]string, method, path string, body []byte) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
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
		return nil, fmt.Errorf("csob: %w", err)
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("csob: %s %s: %d %s", method, path, res.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return respBody, nil
}

func acquireAccessToken(ctx context.Context, client *http.Client, creds map[string]string) (string, error) {
	clientID := strings.TrimSpace(creds["client_id"])
	clientSecret := strings.TrimSpace(creds["client_secret"])
	refresh := strings.TrimSpace(creds["refresh_token"])
	if clientID == "" || clientSecret == "" || refresh == "" {
		return "", errors.New("csob: missing client_id / client_secret / refresh_token")
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refresh)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL(creds)+tokenPath, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("csob oauth: %w", err)
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("csob oauth: status %d: %s", res.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("csob oauth: decode: %w", err)
	}
	if out.AccessToken == "" {
		return "", errors.New("csob oauth: empty access token")
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
	case string:
		// Berlin Group sends amounts as strings.
		var f float64
		_, _ = fmt.Sscanf(x, "%f", &f)
		return f
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
