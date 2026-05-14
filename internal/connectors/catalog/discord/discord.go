// Package discord integrates with Discord via an incoming webhook URL.
// No OAuth — the user pastes a webhook URL from Server Settings →
// Integrations → Webhooks; the URL itself carries the auth token.
package discord

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
	openrownet "github.com/openrow/openrow/internal/net"
)

func init() {
	connectors.Register(&connectors.Connector{
		ID:          "discord",
		Name:        "Discord",
		Description: "Post messages to a Discord channel via an incoming webhook.",
		Category:    "chat",
		Homepage:    "https://discord.com",
		Status:      connectors.StatusAvailable,
		Credentials: []connectors.CredentialField{
			{Name: "webhook_url", Label: "Webhook URL", Kind: connectors.FieldSecret, Required: true,
				Placeholder: "https://discord.com/api/webhooks/…",
				Help:        "Server Settings → Integrations → Webhooks → New Webhook → Copy URL."},
		},
		Test:    test,
		Actions: actions(),
	})
}

func actions() []connectors.Action {
	return []connectors.Action{
		{
			ID:          "post_message",
			Name:        "Post message",
			Description: "Send a message to the configured Discord channel.",
			Mutates:     true,
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content":    map[string]any{"type": "string", "description": "Message text (up to 2000 chars)."},
					"username":   map[string]any{"type": "string", "description": "Override the display name for this message."},
					"avatar_url": map[string]any{"type": "string", "description": "Override the avatar for this message."},
				},
				"required": []string{"content"},
			},
			Handler: postMessage,
		},
	}
}

type postIn struct {
	Content   string `json:"content"`
	Username  string `json:"username,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

func postMessage(ctx context.Context, creds map[string]string, raw json.RawMessage) (any, error) {
	var in postIn
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	if strings.TrimSpace(in.Content) == "" {
		return nil, errors.New("content is required")
	}
	hook, err := validateWebhookURL(creds["webhook_url"])
	if err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(in)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hook+"?wait=true", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discord: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("discord: status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var r struct {
		ID        string `json:"id"`
		ChannelID string `json:"channel_id"`
	}
	_ = json.Unmarshal(body, &r)
	return map[string]any{"id": r.ID, "channel_id": r.ChannelID}, nil
}

// test GETs the webhook URL; Discord returns the webhook metadata JSON
// (id, name, channel_id, …) when the URL is valid.
func test(ctx context.Context, creds map[string]string) error {
	hook, err := validateWebhookURL(creds["webhook_url"])
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, hook, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("discord: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 8192))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("discord: status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// validateWebhookURL pins the credential to Discord's webhook host + path
// shape. Without these checks the tenant-supplied URL is a generic SSRF
// gadget that posts user-controlled JSON anywhere on the public internet.
func validateWebhookURL(raw string) (string, error) {
	hook := strings.TrimSpace(raw)
	if hook == "" {
		return "", errors.New("discord: webhook_url missing")
	}
	u, err := url.Parse(hook)
	if err != nil {
		return "", fmt.Errorf("discord: invalid webhook_url: %w", err)
	}
	if u.Scheme != "https" {
		return "", errors.New("discord: webhook_url must be https")
	}
	host := strings.ToLower(u.Hostname())
	if host != "discord.com" && host != "discordapp.com" &&
		!strings.HasSuffix(host, ".discord.com") && !strings.HasSuffix(host, ".discordapp.com") {
		return "", fmt.Errorf("discord: webhook_url host %q not in discord.com/discordapp.com", host)
	}
	if !strings.HasPrefix(u.Path, "/api/webhooks/") {
		return "", errors.New("discord: webhook_url path must start with /api/webhooks/")
	}
	if err := openrownet.ValidateOutboundURL(hook, false); err != nil {
		return "", fmt.Errorf("discord: webhook_url: %w", err)
	}
	return hook, nil
}
