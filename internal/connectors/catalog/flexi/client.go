package flexi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	openrownet "github.com/openrow/openrow/internal/net"
)

type Client struct {
	baseURL  string
	company  string
	username string
	password string
	http     *http.Client
}

// NewClient builds a client from the credential map. allowInternal skips SSRF
// host-range checks; pass true only in tests or operator-opted-in deployments.
func NewClient(creds map[string]string, allowInternal bool) (*Client, error) {
	base := strings.TrimRight(creds["base_url"], "/")
	company := creds["company"]
	if base == "" || company == "" || creds["username"] == "" {
		return nil, errors.New("flexi: base_url, company, username are required")
	}
	if err := openrownet.ValidateOutboundURL(base, allowInternal); err != nil {
		return nil, fmt.Errorf("flexi base_url: %w", err)
	}
	return &Client{
		baseURL:  base,
		company:  company,
		username: creds["username"],
		password: creds["password"],
		http: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	u := fmt.Sprintf("%s/c/%s/%s", c.baseURL, url.PathEscape(c.company), path)
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("flexi %s: %s — %s", u, resp.Status, truncate(body, 200))
	}
	return body, nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

func (c *Client) ListEvidences(ctx context.Context) ([]string, error) {
	body, err := c.get(ctx, "evidence-list.json", nil)
	if err != nil {
		return nil, err
	}
	var env struct {
		Winstrom struct {
			EvidenceList []struct {
				EvidenceType string `json:"evidenceType"`
			} `json:"evidenceList"`
		} `json:"winstrom"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode evidence-list: %w", err)
	}
	out := make([]string, 0, len(env.Winstrom.EvidenceList))
	for _, e := range env.Winstrom.EvidenceList {
		out = append(out, e.EvidenceType)
	}
	return out, nil
}

type Property struct {
	Name       string `json:"propertyName"`
	Type       string `json:"type"`
	IsWritable bool   `json:"isWritable"`
}

func (c *Client) GetProperties(ctx context.Context, evidence string) ([]Property, error) {
	body, err := c.get(ctx, fmt.Sprintf("%s/properties.json", url.PathEscape(evidence)), nil)
	if err != nil {
		return nil, err
	}
	var env struct {
		Properties struct {
			Property []Property `json:"property"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode properties %q: %w", evidence, err)
	}
	return env.Properties.Property, nil
}

func (c *Client) GetSamples(ctx context.Context, evidence string, limit int) ([]map[string]any, error) {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("detail", "full")
	return c.GetSamplesQuery(ctx, evidence, q)
}

// GetSamplesQuery is GetSamples with caller-controlled query params (used by
// the read-through action so callers can pass `where` filters and explicit
// limits).
func (c *Client) GetSamplesQuery(ctx context.Context, evidence string, q url.Values) ([]map[string]any, error) {
	body, err := c.get(ctx, fmt.Sprintf("%s.json", url.PathEscape(evidence)), q)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode samples %q: %w", evidence, err)
	}
	w, ok := raw["winstrom"].(map[string]any)
	if !ok {
		return nil, nil
	}
	rows, ok := w[evidence].([]any)
	if !ok {
		return nil, nil
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		if m, ok := r.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, nil
}

func (c *Client) Count(ctx context.Context, evidence string) (int64, error) {
	body, err := c.get(ctx, fmt.Sprintf("%s/$count", url.PathEscape(evidence)), nil)
	if err != nil {
		return 0, err
	}
	var env struct {
		Winstrom map[string]any `json:"winstrom"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return 0, fmt.Errorf("decode count %q: %w", evidence, err)
	}
	if s, ok := env.Winstrom["@rowCount"].(string); ok {
		n, _ := strconv.ParseInt(s, 10, 64)
		return n, nil
	}
	return 0, nil
}
