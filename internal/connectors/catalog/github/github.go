// Package github integrates with the GitHub REST API for issues and
// comments, plus verifies inbound webhook signatures. Auth is a
// personal-access token (fine-grained or classic); the webhook secret
// is optional and only needed if flows trigger on GitHub events.
package github

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/openrow/openrow/internal/connectors"
)

const baseURL = "https://api.github.com"

func init() {
	connectors.Register(&connectors.Connector{
		ID:          "github",
		Name:        "GitHub",
		Description: "Create and comment on GitHub issues, and receive webhook events.",
		Category:    "dev",
		Homepage:    "https://github.com",
		Status:      connectors.StatusAvailable,
		Credentials: []connectors.CredentialField{
			{Name: "token", Label: "Personal access token", Kind: connectors.FieldSecret, Required: true,
				Placeholder: "ghp_… or github_pat_…",
				Help:        "Fine-grained PAT scoped to the repos you want to touch, with Issues: Read & write."},
			{Name: "default_owner", Label: "Default owner", Kind: connectors.FieldText, Required: false,
				Placeholder: "my-org"},
			{Name: "default_repo", Label: "Default repo", Kind: connectors.FieldText, Required: false,
				Placeholder: "my-repo",
				Help:        "Used when an action omits owner/repo."},
			{Name: "webhook_secret", Label: "Webhook secret", Kind: connectors.FieldSecret, Required: false,
				Help: "Optional. Only required if flows are triggered by GitHub webhooks."},
		},
		Test:          test,
		Actions:       actions(),
		VerifyWebhook: verifyWebhook,
	})
}

func actions() []connectors.Action {
	return []connectors.Action{
		{
			ID:          "create_issue",
			Name:        "Create issue",
			Description: "Open a new issue on a repo.",
			Mutates:     true,
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"owner":     map[string]any{"type": "string", "description": "Repo owner; falls back to default_owner."},
					"repo":      map[string]any{"type": "string", "description": "Repo name; falls back to default_repo."},
					"title":     map[string]any{"type": "string"},
					"body":      map[string]any{"type": "string"},
					"labels":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"assignees": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"title"},
			},
			Handler: createIssue,
		},
		{
			ID:          "comment_issue",
			Name:        "Comment on issue",
			Description: "Add a comment to an existing issue or pull request.",
			Mutates:     true,
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"owner":        map[string]any{"type": "string"},
					"repo":         map[string]any{"type": "string"},
					"issue_number": map[string]any{"type": "integer"},
					"body":         map[string]any{"type": "string"},
				},
				"required": []string{"issue_number", "body"},
			},
			Handler: commentIssue,
		},
		{
			ID:          "add_labels",
			Name:        "Add labels",
			Description: "Add labels to an issue or pull request (additive; does not replace).",
			Mutates:     true,
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"owner":        map[string]any{"type": "string"},
					"repo":         map[string]any{"type": "string"},
					"issue_number": map[string]any{"type": "integer"},
					"labels":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"issue_number", "labels"},
			},
			Handler: addLabels,
		},
	}
}

type createIssueIn struct {
	Owner     string   `json:"owner"`
	Repo      string   `json:"repo"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Labels    []string `json:"labels"`
	Assignees []string `json:"assignees"`
}

func createIssue(ctx context.Context, creds map[string]string, raw json.RawMessage) (any, error) {
	var in createIssueIn
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	owner, repo, err := resolveRepo(creds, in.Owner, in.Repo)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Title) == "" {
		return nil, errors.New("title is required")
	}
	payload := map[string]any{"title": in.Title}
	if in.Body != "" {
		payload["body"] = in.Body
	}
	if len(in.Labels) > 0 {
		payload["labels"] = in.Labels
	}
	if len(in.Assignees) > 0 {
		payload["assignees"] = in.Assignees
	}
	body, _ := json.Marshal(payload)
	resp, err := call(ctx, creds, http.MethodPost, fmt.Sprintf("/repos/%s/%s/issues", owner, repo), body)
	if err != nil {
		return nil, err
	}
	var r struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
	}
	_ = json.Unmarshal(resp, &r)
	return map[string]any{"number": r.Number, "url": r.HTMLURL}, nil
}

type commentIn struct {
	Owner       string `json:"owner"`
	Repo        string `json:"repo"`
	IssueNumber int    `json:"issue_number"`
	Body        string `json:"body"`
}

func commentIssue(ctx context.Context, creds map[string]string, raw json.RawMessage) (any, error) {
	var in commentIn
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	owner, repo, err := resolveRepo(creds, in.Owner, in.Repo)
	if err != nil {
		return nil, err
	}
	if in.IssueNumber == 0 {
		return nil, errors.New("issue_number is required")
	}
	payload, _ := json.Marshal(map[string]string{"body": in.Body})
	resp, err := call(ctx, creds, http.MethodPost,
		fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, in.IssueNumber), payload)
	if err != nil {
		return nil, err
	}
	var r struct {
		ID      int64  `json:"id"`
		HTMLURL string `json:"html_url"`
	}
	_ = json.Unmarshal(resp, &r)
	return map[string]any{"id": r.ID, "url": r.HTMLURL}, nil
}

type addLabelsIn struct {
	Owner       string   `json:"owner"`
	Repo        string   `json:"repo"`
	IssueNumber int      `json:"issue_number"`
	Labels      []string `json:"labels"`
}

func addLabels(ctx context.Context, creds map[string]string, raw json.RawMessage) (any, error) {
	var in addLabelsIn
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	owner, repo, err := resolveRepo(creds, in.Owner, in.Repo)
	if err != nil {
		return nil, err
	}
	if in.IssueNumber == 0 || len(in.Labels) == 0 {
		return nil, errors.New("issue_number and at least one label are required")
	}
	payload, _ := json.Marshal(map[string]any{"labels": in.Labels})
	if _, err := call(ctx, creds, http.MethodPost,
		fmt.Sprintf("/repos/%s/%s/issues/%d/labels", owner, repo, in.IssueNumber), payload); err != nil {
		return nil, err
	}
	return map[string]any{"issue_number": in.IssueNumber, "labels": in.Labels}, nil
}

func test(ctx context.Context, creds map[string]string) error {
	_, err := call(ctx, creds, http.MethodGet, "/user", nil)
	return err
}

func resolveRepo(creds map[string]string, owner, repo string) (string, string, error) {
	if owner == "" {
		owner = creds["default_owner"]
	}
	if repo == "" {
		repo = creds["default_repo"]
	}
	if owner == "" || repo == "" {
		return "", "", errors.New("owner and repo are required (no defaults configured)")
	}
	return owner, repo, nil
}

func call(ctx context.Context, creds map[string]string, method, path string, body []byte) ([]byte, error) {
	token := strings.TrimSpace(creds["token"])
	if token == "" {
		return nil, errors.New("github: token missing")
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
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github: %w", err)
	}
	defer res.Body.Close()
	resp, _ := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("github: %s %s: %d %s", method, path, res.StatusCode, strings.TrimSpace(string(resp)))
	}
	return resp, nil
}

// verifyWebhook validates GitHub's X-Hub-Signature-256 header:
// https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries
func verifyWebhook(_ context.Context, secret string, headers map[string][]string, body []byte) error {
	if secret == "" {
		return errors.New("github: no webhook secret configured")
	}
	sig := firstHeader(headers, "X-Hub-Signature-256")
	if sig == "" {
		return errors.New("github: missing X-Hub-Signature-256")
	}
	const prefix = "sha256="
	if !strings.HasPrefix(sig, prefix) {
		return errors.New("github: signature has unexpected prefix")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(sig[len(prefix):])) {
		return errors.New("github: signature mismatch")
	}
	return nil
}

func firstHeader(headers map[string][]string, key string) string {
	for k, v := range headers {
		if strings.EqualFold(k, key) && len(v) > 0 {
			return v[0]
		}
	}
	return ""
}
