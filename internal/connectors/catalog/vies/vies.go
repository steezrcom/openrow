// Package vies integrates with the EU VIES VAT number validation
// service. Public REST API, no authentication.
package vies

import (
	"bytes"
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

const endpoint = "https://ec.europa.eu/taxation_customs/vies/rest-api/check-vat-number"

func init() {
	connectors.Register(&connectors.Connector{
		ID:          "vies",
		Name:        "VIES (EU VAT)",
		Description: "Validate EU VAT numbers against the VIES system.",
		Category:    "registry",
		Homepage:    "https://ec.europa.eu/taxation_customs/vies/",
		Status:      connectors.StatusAvailable,
		Actions:     actions(),
	})
}

func actions() []connectors.Action {
	return []connectors.Action{
		{
			ID:   "validate_vat",
			Name: "Validate VAT number",
			Description: "Check whether an EU VAT number is registered. Accepts either a combined VAT (e.g. \"CZ12345678\") or country_code + vat_number. " +
				"Returns isValid plus the registered name/address when the member state chooses to disclose them.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"vat":          map[string]any{"type": "string", "description": "Combined VAT with country prefix, e.g. \"DE123456789\"."},
					"country_code": map[string]any{"type": "string", "description": "Two-letter country code (used with vat_number)."},
					"vat_number":   map[string]any{"type": "string", "description": "VAT number without country prefix (used with country_code)."},
				},
			},
			Handler: validateVAT,
		},
	}
}

type validateIn struct {
	VAT         string `json:"vat"`
	CountryCode string `json:"country_code"`
	VATNumber   string `json:"vat_number"`
}

type validateOut struct {
	CountryCode string `json:"country_code"`
	VATNumber   string `json:"vat_number"`
	IsValid     bool   `json:"is_valid"`
	RequestDate string `json:"request_date,omitempty"`
	Name        string `json:"name,omitempty"`
	Address     string `json:"address,omitempty"`
	UserError   string `json:"user_error,omitempty"`
}

func validateVAT(ctx context.Context, _ map[string]string, raw json.RawMessage) (any, error) {
	var in validateIn
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	cc, num := splitVAT(in)
	if cc == "" || num == "" {
		return nil, errors.New("provide either vat=\"XX12345678\" or both country_code and vat_number")
	}

	payload, _ := json.Marshal(map[string]string{"countryCode": cc, "vatNumber": num})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vies: %w", err)
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("vies: status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var r struct {
		CountryCode string `json:"countryCode"`
		VATNumber   string `json:"vatNumber"`
		IsValid     bool   `json:"isValid"`
		RequestDate string `json:"requestDate"`
		Name        string `json:"name"`
		Address     string `json:"address"`
		UserError   string `json:"userError"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return validateOut{
		CountryCode: r.CountryCode,
		VATNumber:   r.VATNumber,
		IsValid:     r.IsValid,
		RequestDate: r.RequestDate,
		Name:        r.Name,
		Address:     r.Address,
		UserError:   r.UserError,
	}, nil
}

func splitVAT(in validateIn) (cc, num string) {
	if in.CountryCode != "" && in.VATNumber != "" {
		return strings.ToUpper(strings.TrimSpace(in.CountryCode)), strings.TrimSpace(in.VATNumber)
	}
	v := strings.ToUpper(strings.TrimSpace(in.VAT))
	if len(v) < 3 {
		return "", ""
	}
	return v[:2], strings.TrimSpace(v[2:])
}
