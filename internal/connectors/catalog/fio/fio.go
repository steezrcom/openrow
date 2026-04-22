// Package fio integrates with Fio banka's REST API for statement reads.
// A single account-scoped token grants read access to one account's
// movements. Only the date-range endpoint is used — it's stateless and
// doesn't advance Fio's "last-downloaded" cursor.
//
// Fio's API returns each transaction field wrapped in
// `{ "value": ..., "name": "...", "id": N }`; this connector flattens
// that into a compact projection.
package fio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/openrow/openrow/internal/connectors"
)

const baseURL = "https://fioapi.fio.cz/v1/rest"

func init() {
	connectors.Register(&connectors.Connector{
		ID:          "fio",
		Name:        "Fio banka",
		Description: "Czech bank. Read transactions and account balance via a per-account token.",
		Category:    "banking",
		Homepage:    "https://www.fio.cz",
		Status:      connectors.StatusAvailable,
		Credentials: []connectors.CredentialField{
			{Name: "api_token", Label: "API token", Kind: connectors.FieldSecret, Required: true,
				Help: "Internetbanking → Nastavení → API → Vytvořit token. Pick read-only scope."},
		},
		Test:    test,
		Actions: actions(),
	})
}

func actions() []connectors.Action {
	return []connectors.Action{
		{
			ID:          "list_transactions",
			Name:        "List transactions",
			Description: "Return transactions in a date range (YYYY-MM-DD, inclusive). Newest first.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"from": map[string]any{"type": "string", "description": "Start date (YYYY-MM-DD)."},
					"to":   map[string]any{"type": "string", "description": "End date (YYYY-MM-DD)."},
				},
				"required": []string{"from", "to"},
			},
			Handler: listTransactions,
		},
		{
			ID:          "account_info",
			Name:        "Account info",
			Description: "Return IBAN, currency and closing balance for the configured account (using today's date range).",
			Schema:      map[string]any{"type": "object", "properties": map[string]any{}},
			Handler:     accountInfo,
		},
	}
}

type listIn struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type txSlim struct {
	ID           string  `json:"id"`
	Date         string  `json:"date"`
	Amount       float64 `json:"amount"`
	Currency     string  `json:"currency"`
	Counter      string  `json:"counter,omitempty"`
	CounterName  string  `json:"counter_name,omitempty"`
	VS           string  `json:"vs,omitempty"`
	KS           string  `json:"ks,omitempty"`
	SS           string  `json:"ss,omitempty"`
	Type         string  `json:"type,omitempty"`
	Description  string  `json:"description,omitempty"`
	Note         string  `json:"note,omitempty"`
	MessageToRec string  `json:"message_to_recipient,omitempty"`
}

type accountInfo_ struct {
	AccountID      string  `json:"account_id"`
	BankID         string  `json:"bank_id"`
	Currency       string  `json:"currency"`
	IBAN           string  `json:"iban"`
	OpeningBalance float64 `json:"opening_balance"`
	ClosingBalance float64 `json:"closing_balance"`
	DateStart      string  `json:"date_start"`
	DateEnd        string  `json:"date_end"`
}

type statement struct {
	AccountStatement struct {
		Info            map[string]any `json:"info"`
		TransactionList struct {
			Transaction []map[string]json.RawMessage `json:"transaction"`
		} `json:"transactionList"`
	} `json:"accountStatement"`
}

func listTransactions(ctx context.Context, creds map[string]string, raw json.RawMessage) (any, error) {
	var in listIn
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	st, err := fetchPeriod(ctx, creds, in.From, in.To)
	if err != nil {
		return nil, err
	}
	out := make([]txSlim, 0, len(st.AccountStatement.TransactionList.Transaction))
	for _, tx := range st.AccountStatement.TransactionList.Transaction {
		out = append(out, toSlim(tx))
	}
	return out, nil
}

func accountInfo(ctx context.Context, creds map[string]string, _ json.RawMessage) (any, error) {
	today := time.Now().Format("2006-01-02")
	st, err := fetchPeriod(ctx, creds, today, today)
	if err != nil {
		return nil, err
	}
	info := st.AccountStatement.Info
	return accountInfo_{
		AccountID:      stringOf(info["accountId"]),
		BankID:         stringOf(info["bankId"]),
		Currency:       stringOf(info["currency"]),
		IBAN:           stringOf(info["iban"]),
		OpeningBalance: floatOf(info["openingBalance"]),
		ClosingBalance: floatOf(info["closingBalance"]),
		DateStart:      stringOf(info["dateStart"]),
		DateEnd:        stringOf(info["dateEnd"]),
	}, nil
}

func test(ctx context.Context, creds map[string]string) error {
	today := time.Now().Format("2006-01-02")
	_, err := fetchPeriod(ctx, creds, today, today)
	return err
}

func fetchPeriod(ctx context.Context, creds map[string]string, from, to string) (*statement, error) {
	if _, err := time.Parse("2006-01-02", from); err != nil {
		return nil, fmt.Errorf("from must be YYYY-MM-DD: %w", err)
	}
	if _, err := time.Parse("2006-01-02", to); err != nil {
		return nil, fmt.Errorf("to must be YYYY-MM-DD: %w", err)
	}
	token := strings.TrimSpace(creds["api_token"])
	if token == "" {
		return nil, errors.New("fio: api_token missing")
	}
	u := fmt.Sprintf("%s/periods/%s/%s/%s/transactions.json", baseURL, token, from, to)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fio: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if res.StatusCode == http.StatusConflict {
		return nil, errors.New("fio: 409 — token throttled (one request per 30s)")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("fio: status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var st statement
	if err := json.Unmarshal(body, &st); err != nil {
		return nil, fmt.Errorf("fio: decode: %w", err)
	}
	return &st, nil
}

func toSlim(tx map[string]json.RawMessage) txSlim {
	return txSlim{
		ID:           colString(tx, "column22"),
		Date:         colString(tx, "column0"),
		Amount:       colFloat(tx, "column1"),
		Currency:     colString(tx, "column14"),
		Counter:      colString(tx, "column2"),
		CounterName:  colString(tx, "column10"),
		VS:           colString(tx, "column5"),
		KS:           colString(tx, "column4"),
		SS:           colString(tx, "column6"),
		Type:         colString(tx, "column8"),
		Description: firstNonEmpty(
			colString(tx, "column16"),
			colString(tx, "column17"),
		),
		Note:         colString(tx, "column25"),
		MessageToRec: colString(tx, "column7"),
	}
}

func colString(tx map[string]json.RawMessage, key string) string {
	raw, ok := tx[key]
	if !ok {
		return ""
	}
	var c struct {
		Value any `json:"value"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return ""
	}
	return stringOf(c.Value)
}

func colFloat(tx map[string]json.RawMessage, key string) float64 {
	raw, ok := tx[key]
	if !ok {
		return 0
	}
	var c struct {
		Value any `json:"value"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return 0
	}
	return floatOf(c.Value)
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
