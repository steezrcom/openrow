package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sashabaranov/go-openai"

	"github.com/openrow/openrow/internal/entities"
	"github.com/openrow/openrow/internal/llm"
)

const proposeToolName = "propose_entity"

const proposerSystemPrompt = `You design database entities for a business ERP.

Given a natural-language description, return exactly one entity using the propose_entity tool.

Rules:
- name: lowercase snake_case, plural when it represents a collection (customers, invoices). Machine identifier only; must match ^[a-z][a-z0-9_]{0,62}$.
- display_name: human label, title case ("Customers", "Purchase Orders").
- Fields you should almost never add: id, created_at, updated_at — the system adds them.
- Reasonable fields only. Don't invent fields the user didn't hint at. If the user says "customer with name and email", return two fields. Don't speculate "phone, address, notes".
- Data types: text, integer, bigint, numeric, boolean, date, timestamptz, uuid, jsonb, reference.
- Use "reference" with reference_entity set to another existing entity's name for foreign keys. Only reference entities that already exist in the provided list.
- Prefer text for free strings, numeric for money/quantities, date for dates without time, timestamptz when time of day matters.
- Set is_required only when the business clearly needs it. Don't mark everything required.`

// Proposer turns a natural-language description into an entity spec, via a forced tool call.
type Proposer struct {
	llm *llm.Service
}

func NewProposer(llmSvc *llm.Service) *Proposer {
	return &Proposer{llm: llmSvc}
}

func (p *Proposer) Propose(ctx context.Context, tenantID, description string, existing []entities.Entity) (*entities.EntitySpec, error) {
	if p == nil {
		return nil, errors.New("proposer not available")
	}
	cfg, err := p.llm.Resolve(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	client := llm.NewClient(cfg)

	existingNames := make([]string, len(existing))
	for i, e := range existing {
		existingNames[i] = e.Name
	}
	userMsg := description
	if len(existingNames) > 0 {
		userMsg += "\n\nExisting entities you can reference: " + strings.Join(existingNames, ", ")
	}

	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:     cfg.Model,
		MaxTokens: 2048,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: proposerSystemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: userMsg},
		},
		Tools: []openai.Tool{{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        proposeToolName,
				Description: "Propose one entity (table) definition for the ERP.",
				Parameters:  objectSchema(entitySchemaProperties(), "name", "display_name", "fields"),
			},
		}},
		// Force the model to call propose_entity exactly (structured output).
		ToolChoice: map[string]any{
			"type":     "function",
			"function": map[string]string{"name": proposeToolName},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("llm: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, errors.New("llm returned no choices")
	}
	calls := resp.Choices[0].Message.ToolCalls
	if len(calls) == 0 {
		return nil, fmt.Errorf("llm did not call %s", proposeToolName)
	}
	call := calls[0]
	if call.Function.Name != proposeToolName {
		return nil, fmt.Errorf("llm called unexpected tool %q", call.Function.Name)
	}
	var spec entities.EntitySpec
	if err := json.Unmarshal([]byte(call.Function.Arguments), &spec); err != nil {
		return nil, fmt.Errorf("unmarshal proposed entity: %w", err)
	}
	if err := spec.Validate(); err != nil {
		return nil, fmt.Errorf("llm returned invalid spec: %w", err)
	}
	return &spec, nil
}
