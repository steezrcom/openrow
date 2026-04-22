// Package ares integrates with the Czech company registry (ARES).
// Public REST API v3, no authentication required.
package ares

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

const baseURL = "https://ares.gov.cz/ekonomicke-subjekty-v-be/rest/ekonomicke-subjekty"

func init() {
	connectors.Register(&connectors.Connector{
		ID:          "ares",
		Name:        "ARES",
		Description: "Czech company registry. Look up companies by IČO or name.",
		Category:    "registry",
		Homepage:    "https://ares.gov.cz",
		Status:      connectors.StatusAvailable,
		Actions:     actions(),
	})
}

func actions() []connectors.Action {
	return []connectors.Action{
		{
			ID:          "lookup_by_ico",
			Name:        "Lookup by IČO",
			Description: "Return the company registered under this IČO (Czech company ID, 8 digits), or null if none.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ico": map[string]any{"type": "string", "description": "8-digit IČO."},
				},
				"required": []string{"ico"},
			},
			Handler: lookupByICO,
		},
		{
			ID:          "search_by_name",
			Name:        "Search by name",
			Description: "Search the registry by company name (full or partial). Returns up to 'limit' compact matches.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":  map[string]any{"type": "string", "description": "Substring to search in the trading name."},
					"limit": map[string]any{"type": "integer", "description": "Max matches; default 20, max 100."},
				},
				"required": []string{"name"},
			},
			Handler: searchByName,
		},
	}
}

type companySlim struct {
	ICO       string `json:"ico"`
	DIC       string `json:"dic,omitempty"`
	Name      string `json:"name"`
	LegalForm string `json:"legal_form,omitempty"`
	Address   string `json:"address,omitempty"`
	Founded   string `json:"founded,omitempty"`
	Active    bool   `json:"active"`
}

type lookupIn struct {
	ICO string `json:"ico"`
}

func lookupByICO(ctx context.Context, _ map[string]string, raw json.RawMessage) (any, error) {
	var in lookupIn
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	ico := strings.TrimSpace(in.ICO)
	if ico == "" {
		return nil, errors.New("ico is required")
	}
	body, status, err := httpCall(ctx, http.MethodGet, baseURL+"/"+url.PathEscape(ico), nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("ares: GET /%s: %d %s", ico, status, strings.TrimSpace(string(body)))
	}
	var raw2 map[string]any
	if err := json.Unmarshal(body, &raw2); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return toSlim(raw2), nil
}

type searchIn struct {
	Name  string `json:"name"`
	Limit int    `json:"limit"`
}

func searchByName(ctx context.Context, _ map[string]string, raw json.RawMessage) (any, error) {
	var in searchIn
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, errors.New("name is required")
	}
	if in.Limit <= 0 {
		in.Limit = 20
	}
	if in.Limit > 100 {
		in.Limit = 100
	}
	reqBody, _ := json.Marshal(map[string]any{"obchodniJmeno": name, "pocet": in.Limit})
	body, status, err := httpCall(ctx, http.MethodPost, baseURL+"/vyhledat", reqBody)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("ares: search: %d %s", status, strings.TrimSpace(string(body)))
	}
	var resp struct {
		EkonomickeSubjekty []map[string]any `json:"ekonomickeSubjekty"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	out := make([]companySlim, 0, len(resp.EkonomickeSubjekty))
	for _, r := range resp.EkonomickeSubjekty {
		out = append(out, toSlim(r))
	}
	return out, nil
}

func toSlim(r map[string]any) companySlim {
	address := ""
	if s, ok := r["sidlo"].(map[string]any); ok {
		address = stringOf(s["textovaAdresa"])
	}
	active := true
	if zanik := stringOf(r["datumZaniku"]); zanik != "" {
		active = false
	}
	return companySlim{
		ICO:       stringOf(r["ico"]),
		DIC:       stringOf(r["dic"]),
		Name:      stringOf(r["obchodniJmeno"]),
		LegalForm: stringOf(r["pravniForma"]),
		Address:   address,
		Founded:   stringOf(r["datumVzniku"]),
		Active:    active,
	}
}

func httpCall(ctx context.Context, method, u string, body []byte) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("ares: %w", err)
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	return respBody, res.StatusCode, nil
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
