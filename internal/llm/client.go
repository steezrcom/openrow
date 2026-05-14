package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"

	openrownet "github.com/openrow/openrow/internal/net"
)

// allowedLLMHosts is the explicit allowlist for hosted LLM providers.
// Self-hosted / local endpoints (localhost, RFC1918) require the operator to
// opt in via OPENROW_ALLOW_INTERNAL_LLM=true.
var allowedLLMHosts = map[string]bool{
	"api.anthropic.com":                true,
	"api.openai.com":                   true,
	"api.groq.com":                     true,
	"generativelanguage.googleapis.com": true,
	"api.together.xyz":                 true,
	"api.deepseek.com":                 true,
	"api.x.ai":                         true,
	"openrouter.ai":                    true,
}

// ValidateLLMBaseURL checks an LLM base_url before any HTTP traffic. Public
// endpoints must be on the allowlist; internal addresses require an explicit
// operator opt-in. Returns the trimmed URL on success.
func ValidateLLMBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("base_url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid base_url: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("base_url scheme %q not allowed", u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", errors.New("base_url has no host")
	}
	allowInternal := strings.EqualFold(os.Getenv("OPENROW_ALLOW_INTERNAL_LLM"), "true")
	if allowedLLMHosts[host] {
		return raw, nil
	}
	if !allowInternal {
		return "", fmt.Errorf("base_url host %q not in LLM allowlist; set OPENROW_ALLOW_INTERNAL_LLM=true for local endpoints", host)
	}
	if err := openrownet.ValidateOutboundURL(raw, true); err != nil {
		return "", fmt.Errorf("base_url: %w", err)
	}
	return raw, nil
}

// NewClient builds a go-openai client against the given config. Local endpoints
// (Ollama, LM Studio, etc.) may not require an API key; the SDK refuses empty
// strings, so we inject a harmless placeholder when none is configured.
func NewClient(cfg *Config) *openai.Client {
	c := openai.DefaultConfig(cfg.APIKey)
	if cfg.APIKey == "" {
		c = openai.DefaultConfig("not-required")
	}
	c.BaseURL = cfg.BaseURL
	// Longer timeout accommodates slow local inference; cloud calls finish in
	// seconds, local 70B on CPU can take a minute.
	c.HTTPClient = &http.Client{Timeout: 180 * time.Second}
	return openai.NewClientWithConfig(c)
}

// ModelInfo is the minimal shape we need from /v1/models.
type ModelInfo struct {
	ID string `json:"id"`
}

// ListModels hits GET /v1/models on the provider using the supplied base URL
// and API key directly (used by the settings UI before a config is saved).
func ListModels(ctx context.Context, baseURL, apiKey string) ([]ModelInfo, error) {
	baseURL = normalizeBaseURL(baseURL)
	if _, err := ValidateLLMBaseURL(baseURL); err != nil {
		return nil, err
	}
	tmp := &Config{BaseURL: baseURL, APIKey: apiKey}
	client := NewClient(tmp)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := client.ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	out := make([]ModelInfo, 0, len(resp.Models))
	for _, m := range resp.Models {
		out = append(out, ModelInfo{ID: m.ID})
	}
	return out, nil
}

// TestResult summarises a connection test against a provider.
type TestResult struct {
	OK        bool   `json:"ok"`
	ModelsOK  bool   `json:"models_ok"`
	ChatOK    bool   `json:"chat_ok"`
	ToolsOK   bool   `json:"tools_ok"`
	Message   string `json:"message,omitempty"`
	Model     string `json:"model,omitempty"`
}

// Test does a three-stage probe:
//  1. GET /v1/models (verifies auth + URL)
//  2. tiny chat completion (verifies chat endpoint)
//  3. chat completion with a no-op tool (verifies tool-call support)
//
// Any stage may fail; we report which ones passed so users can tell whether
// their local model "works for chat but doesn't tool-call."
func Test(ctx context.Context, baseURL, apiKey, model string) TestResult {
	baseURL = normalizeBaseURL(baseURL)
	result := TestResult{Model: model}
	if _, err := ValidateLLMBaseURL(baseURL); err != nil {
		result.Message = err.Error()
		return result
	}
	if model == "" {
		result.Message = "model is required"
		return result
	}

	client := NewClient(&Config{BaseURL: baseURL, APIKey: apiKey})

	// Stage 1: list models. Some endpoints fail here with a dummy key; don't
	// treat that as a showstopper. It's informational.
	listCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	if _, err := client.ListModels(listCtx); err == nil {
		result.ModelsOK = true
	}
	cancel()

	// Stage 2: one-shot chat.
	chatCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	resp, err := client.CreateChatCompletion(chatCtx, openai.ChatCompletionRequest{
		Model: model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: `Reply with the single word "ok".`},
		},
		MaxTokens: 16,
	})
	if err != nil {
		result.Message = fmt.Sprintf("chat: %v", err)
		return result
	}
	if len(resp.Choices) == 0 {
		result.Message = "chat: no choices in response"
		return result
	}
	result.ChatOK = true

	// Stage 3: chat with a trivial tool and force a tool call. Models that can't
	// handle tools will either ignore the tool or error here.
	toolResp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: "You have a tool called ping. Call it with {} to answer."},
			{Role: openai.ChatMessageRoleUser, Content: "ping please"},
		},
		Tools: []openai.Tool{{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "ping",
				Description: "Returns pong.",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		}},
		MaxTokens: 128,
	})
	if err == nil && len(toolResp.Choices) > 0 && len(toolResp.Choices[0].Message.ToolCalls) > 0 {
		result.ToolsOK = true
	}

	result.OK = result.ChatOK
	if !result.ToolsOK {
		result.Message = "Chat works but the model didn't emit a tool call. Agent features require a tool-capable model (GPT-4o, Claude 3.5+, Llama 3.1 8B+, Qwen 2.5 7B+, Mistral Nemo)."
	}
	return result
}
