package discovery

import (
	"context"
	"errors"

	"github.com/sashabaranov/go-openai"

	"github.com/openrow/openrow/internal/llm"
)

// WorkspaceClassifier is the production Classifier. It resolves the
// workspace's LLM config (the global setting) on every call and uses it. No
// per-binding model overrides — see spec Stage 5.
type WorkspaceClassifier struct {
	LLM      *llm.Service
	TenantID string
}

func (w *WorkspaceClassifier) Classify(ctx context.Context, prompt string) (string, error) {
	cfg, err := w.LLM.Resolve(ctx, w.TenantID)
	if err != nil {
		return "", err
	}
	client := llm.NewClient(cfg)
	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: cfg.Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: "You return JSON only, with no prose, no markdown fences."},
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
		Temperature: 0,
		MaxTokens:   1024,
	})
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("classifier: empty response")
	}
	return resp.Choices[0].Message.Content, nil
}
