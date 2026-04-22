// Package resend integrates with Resend for transactional email.
// Single bearer API key.
package resend

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

const baseURL = "https://api.resend.com"

func init() {
	connectors.Register(&connectors.Connector{
		ID:          "resend",
		Name:        "Resend",
		Description: "Transactional email via Resend. Verify a domain first, then send from any address on it.",
		Category:    "email",
		Homepage:    "https://resend.com",
		Status:      connectors.StatusAvailable,
		Credentials: []connectors.CredentialField{
			{Name: "api_key", Label: "API key", Kind: connectors.FieldSecret, Required: true,
				Placeholder: "re_…"},
			{Name: "default_from", Label: "Default From", Kind: connectors.FieldText, Required: false,
				Placeholder: "you@yourdomain.com",
				Help:        "Used when a send_email call doesn't set 'from'."},
		},
		Test:    test,
		Actions: actions(),
	})
}

func actions() []connectors.Action {
	return []connectors.Action{
		{
			ID:          "send_email",
			Name:        "Send email",
			Description: "Send a transactional email. At least one of html or text must be set.",
			Mutates:     true,
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"from":    map[string]any{"type": "string", "description": "Sender. Omit to use default_from from config."},
					"to":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Recipient list."},
					"cc":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"bcc":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"subject": map[string]any{"type": "string"},
					"html":    map[string]any{"type": "string"},
					"text":    map[string]any{"type": "string"},
					"reply_to": map[string]any{"type": "string"},
				},
				"required": []string{"to", "subject"},
			},
			Handler: sendEmail,
		},
	}
}

type sendIn struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	CC      []string `json:"cc,omitempty"`
	BCC     []string `json:"bcc,omitempty"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html,omitempty"`
	Text    string   `json:"text,omitempty"`
	ReplyTo string   `json:"reply_to,omitempty"`
}

func sendEmail(ctx context.Context, creds map[string]string, raw json.RawMessage) (any, error) {
	var in sendIn
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	if in.From == "" {
		in.From = creds["default_from"]
	}
	if in.From == "" {
		return nil, errors.New("from is required (no default_from configured)")
	}
	if len(in.To) == 0 {
		return nil, errors.New("to must have at least one recipient")
	}
	if in.HTML == "" && in.Text == "" {
		return nil, errors.New("at least one of html or text must be set")
	}

	payload := map[string]any{"from": in.From, "to": in.To, "subject": in.Subject}
	if in.HTML != "" {
		payload["html"] = in.HTML
	}
	if in.Text != "" {
		payload["text"] = in.Text
	}
	if len(in.CC) > 0 {
		payload["cc"] = in.CC
	}
	if len(in.BCC) > 0 {
		payload["bcc"] = in.BCC
	}
	if in.ReplyTo != "" {
		payload["reply_to"] = in.ReplyTo
	}
	body, _ := json.Marshal(payload)

	resp, err := call(ctx, creds, http.MethodPost, "/emails", body)
	if err != nil {
		return nil, err
	}
	var r struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp, &r); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return map[string]any{"id": r.ID}, nil
}

func test(ctx context.Context, creds map[string]string) error {
	_, err := call(ctx, creds, http.MethodGet, "/domains", nil)
	return err
}

func call(ctx context.Context, creds map[string]string, method, path string, body []byte) ([]byte, error) {
	key := strings.TrimSpace(creds["api_key"])
	if key == "" {
		return nil, errors.New("resend: api_key missing")
	}
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("resend: %w", err)
	}
	defer res.Body.Close()
	resp, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("resend: %s %s: %d %s", method, path, res.StatusCode, strings.TrimSpace(string(resp)))
	}
	return resp, nil
}
