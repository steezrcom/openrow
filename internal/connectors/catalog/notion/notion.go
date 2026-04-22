// Package notion integrates with the Notion REST API for page creation
// and database queries. Auth is an integration token; the integration
// must be explicitly invited to each page/database it touches.
package notion

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

const (
	baseURL       = "https://api.notion.com/v1"
	notionVersion = "2022-06-28"
)

func init() {
	connectors.Register(&connectors.Connector{
		ID:          "notion",
		Name:        "Notion",
		Description: "Create pages and query Notion databases.",
		Category:    "docs",
		Homepage:    "https://notion.so",
		Status:      connectors.StatusAvailable,
		Credentials: []connectors.CredentialField{
			{Name: "api_token", Label: "Integration token", Kind: connectors.FieldSecret, Required: true,
				Placeholder: "secret_… or ntn_…",
				Help:        "https://www.notion.so/profile/integrations — create an internal integration, then share each target page with it."},
		},
		Test:    test,
		Actions: actions(),
	})
}

func actions() []connectors.Action {
	return []connectors.Action{
		{
			ID:   "create_page",
			Name: "Create page",
			Description: "Create a Notion page. 'parent' is either { \"database_id\": \"…\" } or { \"page_id\": \"…\" }. " +
				"'properties' must match the parent's schema (see Notion's property object reference). " +
				"'children' is an optional block list appended to the page body.",
			Mutates: true,
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"parent":     map[string]any{"type": "object", "description": "{ database_id } or { page_id }."},
					"properties": map[string]any{"type": "object"},
					"children":   map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
				},
				"required": []string{"parent", "properties"},
			},
			Handler: createPage,
		},
		{
			ID:   "query_database",
			Name: "Query database",
			Description: "Query a Notion database. 'filter' and 'sorts' are Notion's native structures; pass them through as-is. " +
				"Returns up to 'page_size' rows (default 100).",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"database_id": map[string]any{"type": "string"},
					"filter":      map[string]any{"type": "object"},
					"sorts":       map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
					"page_size":   map[string]any{"type": "integer", "description": "Max 100."},
				},
				"required": []string{"database_id"},
			},
			Handler: queryDatabase,
		},
	}
}

type createPageIn struct {
	Parent     json.RawMessage `json:"parent"`
	Properties json.RawMessage `json:"properties"`
	Children   json.RawMessage `json:"children,omitempty"`
}

func createPage(ctx context.Context, creds map[string]string, raw json.RawMessage) (any, error) {
	var in createPageIn
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	if len(in.Parent) == 0 || len(in.Properties) == 0 {
		return nil, errors.New("parent and properties are required")
	}
	payload := map[string]any{"parent": in.Parent, "properties": in.Properties}
	if len(in.Children) > 0 {
		payload["children"] = in.Children
	}
	body, _ := json.Marshal(payload)
	resp, err := call(ctx, creds, http.MethodPost, "/pages", body)
	if err != nil {
		return nil, err
	}
	var r struct {
		ID      string `json:"id"`
		URL     string `json:"url"`
		Created string `json:"created_time"`
	}
	_ = json.Unmarshal(resp, &r)
	return map[string]any{"id": r.ID, "url": r.URL, "created_time": r.Created}, nil
}

type queryIn struct {
	DatabaseID string          `json:"database_id"`
	Filter     json.RawMessage `json:"filter,omitempty"`
	Sorts      json.RawMessage `json:"sorts,omitempty"`
	PageSize   int             `json:"page_size"`
}

func queryDatabase(ctx context.Context, creds map[string]string, raw json.RawMessage) (any, error) {
	var in queryIn
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	if strings.TrimSpace(in.DatabaseID) == "" {
		return nil, errors.New("database_id is required")
	}
	payload := map[string]any{}
	if len(in.Filter) > 0 {
		payload["filter"] = in.Filter
	}
	if len(in.Sorts) > 0 {
		payload["sorts"] = in.Sorts
	}
	if in.PageSize > 0 {
		if in.PageSize > 100 {
			in.PageSize = 100
		}
		payload["page_size"] = in.PageSize
	}
	body, _ := json.Marshal(payload)
	resp, err := call(ctx, creds, http.MethodPost, "/databases/"+in.DatabaseID+"/query", body)
	if err != nil {
		return nil, err
	}
	var r struct {
		Results    []map[string]any `json:"results"`
		HasMore    bool             `json:"has_more"`
		NextCursor string           `json:"next_cursor"`
	}
	if err := json.Unmarshal(resp, &r); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return map[string]any{
		"results":     r.Results,
		"has_more":    r.HasMore,
		"next_cursor": r.NextCursor,
	}, nil
}

func test(ctx context.Context, creds map[string]string) error {
	_, err := call(ctx, creds, http.MethodGet, "/users/me", nil)
	return err
}

func call(ctx context.Context, creds map[string]string, method, path string, body []byte) ([]byte, error) {
	token := strings.TrimSpace(creds["api_token"])
	if token == "" {
		return nil, errors.New("notion: api_token missing")
	}
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Notion-Version", notionVersion)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 20 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("notion: %w", err)
	}
	defer res.Body.Close()
	resp, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("notion: %s %s: %d %s", method, path, res.StatusCode, strings.TrimSpace(string(resp)))
	}
	return resp, nil
}
