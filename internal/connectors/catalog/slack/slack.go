// Package slack integrates with Slack via bot token. Covers
// chat.postMessage; webhook delivery from Slack (e.g. slash commands,
// event subscriptions) is not wired here yet — add VerifyWebhook when
// needed.
package slack

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

const baseURL = "https://slack.com/api"

func init() {
	connectors.Register(&connectors.Connector{
		ID:          "slack",
		Name:        "Slack",
		Description: "Post messages to Slack channels from flows and the agent.",
		Category:    "chat",
		Homepage:    "https://slack.com",
		Status:      connectors.StatusAvailable,
		Credentials: []connectors.CredentialField{
			{Name: "bot_token", Label: "Bot token", Kind: connectors.FieldSecret, Required: true,
				Placeholder: "xoxb-…",
				Help:        "From a Slack app with at least chat:write. Invite the bot to any channel you want to post in."},
			{Name: "default_channel", Label: "Default channel", Kind: connectors.FieldText, Required: false,
				Placeholder: "#general",
				Help:        "Used when a flow/action doesn't specify one."},
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
			Description: "Send a message to a Slack channel. Supports threading via thread_ts.",
			Mutates:     true,
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"channel":   map[string]any{"type": "string", "description": "Channel name (#foo) or ID. Omit to use the default_channel from config."},
					"text":      map[string]any{"type": "string", "description": "Message text. Supports Slack mrkdwn."},
					"thread_ts": map[string]any{"type": "string", "description": "Parent message ts to reply in-thread."},
				},
				"required": []string{"text"},
			},
			Handler: postMessage,
		},
	}
}

type postIn struct {
	Channel  string `json:"channel"`
	Text     string `json:"text"`
	ThreadTS string `json:"thread_ts"`
}

func postMessage(ctx context.Context, creds map[string]string, raw json.RawMessage) (any, error) {
	var in postIn
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	if strings.TrimSpace(in.Text) == "" {
		return nil, errors.New("text is required")
	}
	channel := in.Channel
	if channel == "" {
		channel = creds["default_channel"]
	}
	if channel == "" {
		return nil, errors.New("channel is required (no default_channel configured)")
	}
	payload := map[string]any{"channel": channel, "text": in.Text}
	if in.ThreadTS != "" {
		payload["thread_ts"] = in.ThreadTS
	}
	body, _ := json.Marshal(payload)

	resp, err := call(ctx, creds, http.MethodPost, "/chat.postMessage", body)
	if err != nil {
		return nil, err
	}
	var r struct {
		OK      bool   `json:"ok"`
		Channel string `json:"channel"`
		TS      string `json:"ts"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(resp, &r); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if !r.OK {
		return nil, fmt.Errorf("slack: %s", r.Error)
	}
	return map[string]any{"channel": r.Channel, "ts": r.TS}, nil
}

func test(ctx context.Context, creds map[string]string) error {
	body, err := call(ctx, creds, http.MethodPost, "/auth.test", []byte(`{}`))
	if err != nil {
		return err
	}
	var r struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return err
	}
	if !r.OK {
		return fmt.Errorf("slack: %s", r.Error)
	}
	return nil
}

func call(ctx context.Context, creds map[string]string, method, path string, body []byte) ([]byte, error) {
	token := strings.TrimSpace(creds["bot_token"])
	if token == "" {
		return nil, errors.New("slack: bot_token missing")
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
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("slack: %w", err)
	}
	defer res.Body.Close()
	resp, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("slack: status %d: %s", res.StatusCode, strings.TrimSpace(string(resp)))
	}
	return resp, nil
}
