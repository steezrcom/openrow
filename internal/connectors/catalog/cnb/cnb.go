// Package cnb integrates with the Czech National Bank's daily FX rate
// feed. Public, no authentication.
package cnb

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/openrow/openrow/internal/connectors"
)

const feedURL = "https://www.cnb.cz/en/financial-markets/foreign-exchange-market/central-bank-exchange-rate-fixing/central-bank-exchange-rate-fixing/daily.txt"

func init() {
	connectors.Register(&connectors.Connector{
		ID:          "cnb",
		Name:        "ČNB FX rates",
		Description: "Daily foreign-exchange rates published by the Czech National Bank.",
		Category:    "reference",
		Homepage:    "https://www.cnb.cz",
		Status:      connectors.StatusAvailable,
		Actions:     actions(),
	})
}

func actions() []connectors.Action {
	return []connectors.Action{
		{
			ID:   "get_rate",
			Name: "Get rate",
			Description: "Return the CZK exchange rate for a currency code on a given date (default: today). " +
				"ČNB quotes N foreign units → rate CZK, so the result carries both the unit count (amount) and the rate.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"code": map[string]any{"type": "string", "description": "ISO 4217 code, e.g. \"USD\"."},
					"date": map[string]any{"type": "string", "description": "YYYY-MM-DD; optional, defaults to today."},
				},
				"required": []string{"code"},
			},
			Handler: getRate,
		},
		{
			ID:          "list_rates",
			Name:        "List rates",
			Description: "Return all CZK exchange rates for a given date (default: today).",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"date": map[string]any{"type": "string", "description": "YYYY-MM-DD; optional, defaults to today."},
				},
			},
			Handler: listRates,
		},
	}
}

type rateRow struct {
	Country  string  `json:"country"`
	Currency string  `json:"currency"`
	Amount   int     `json:"amount"`
	Code     string  `json:"code"`
	Rate     float64 `json:"rate"`
}

type getRateIn struct {
	Code string `json:"code"`
	Date string `json:"date"`
}

func getRate(ctx context.Context, _ map[string]string, raw json.RawMessage) (any, error) {
	var in getRateIn
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	if code == "" {
		return nil, errors.New("code is required")
	}
	rows, _, err := fetchFeed(ctx, in.Date)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.Code == code {
			return r, nil
		}
	}
	return nil, fmt.Errorf("cnb: rate for %q not found", code)
}

type listRatesIn struct {
	Date string `json:"date"`
}

func listRates(ctx context.Context, _ map[string]string, raw json.RawMessage) (any, error) {
	var in listRatesIn
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, fmt.Errorf("parse input: %w", err)
		}
	}
	rows, date, err := fetchFeed(ctx, in.Date)
	if err != nil {
		return nil, err
	}
	return map[string]any{"date": date, "rates": rows}, nil
}

// fetchFeed downloads and parses the daily feed. The ČNB txt is pipe-delimited
// with a first line "DD Mon YYYY #NN" (English) and a header row we skip.
func fetchFeed(ctx context.Context, isoDate string) ([]rateRow, string, error) {
	u := feedURL
	if d := strings.TrimSpace(isoDate); d != "" {
		parsed, err := time.Parse("2006-01-02", d)
		if err != nil {
			return nil, "", fmt.Errorf("date must be YYYY-MM-DD: %w", err)
		}
		u += "?date=" + parsed.Format("02.01.2006")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("cnb: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 128*1024))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, "", fmt.Errorf("cnb: status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	return parseFeed(body)
}

func parseFeed(body []byte) ([]rateRow, string, error) {
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 64*1024), 256*1024)
	var (
		rows []rateRow
		date string
		line int
	)
	for sc.Scan() {
		text := strings.TrimSpace(sc.Text())
		line++
		if text == "" {
			continue
		}
		if line == 1 {
			if i := strings.Index(text, "#"); i > 0 {
				date = strings.TrimSpace(text[:i])
			} else {
				date = text
			}
			continue
		}
		parts := strings.Split(text, "|")
		if len(parts) != 5 {
			continue
		}
		if strings.EqualFold(parts[3], "Code") {
			continue
		}
		amount, err := strconv.Atoi(strings.TrimSpace(parts[2]))
		if err != nil {
			continue
		}
		rate, err := strconv.ParseFloat(strings.Replace(strings.TrimSpace(parts[4]), ",", ".", 1), 64)
		if err != nil {
			continue
		}
		rows = append(rows, rateRow{
			Country:  parts[0],
			Currency: parts[1],
			Amount:   amount,
			Code:     strings.ToUpper(strings.TrimSpace(parts[3])),
			Rate:     rate,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, "", err
	}
	if len(rows) == 0 {
		return nil, "", errors.New("cnb: no rates parsed from feed")
	}
	return rows, date, nil
}
