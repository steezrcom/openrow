package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/steezrcom/steezr-erp/internal/entities"
)

const toolName = "propose_entity"

const systemPrompt = `You design database entities for a business ERP.

Given a natural-language description, return exactly one entity using the propose_entity tool.

Rules:
- name: lowercase snake_case, plural when it represents a collection (customers, invoices). Machine identifier only — must match ^[a-z][a-z0-9_]{0,62}$.
- display_name: human label, title case ("Customers", "Purchase Orders").
- Fields you should almost never add: id, created_at, updated_at — the system adds them.
- Reasonable fields only. Don't invent fields the user didn't hint at. If the user says "customer with name and email", return two fields. Don't speculate "phone, address, notes".
- Data types: text, integer, bigint, numeric, boolean, date, timestamptz, uuid, jsonb, reference.
- Use "reference" with reference_entity set to another existing entity's name for foreign keys. Only reference entities that already exist in the provided list.
- Prefer text for free strings, numeric for money/quantities, date for dates without time, timestamptz when time of day matters.
- Set is_required only when the business clearly needs it. Don't mark everything required.`

type Proposer struct {
	client anthropic.Client
	model  anthropic.Model
}

func NewProposer(apiKey string) *Proposer {
	if apiKey == "" {
		return nil
	}
	c := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &Proposer{client: c, model: anthropic.ModelClaudeSonnet4_6}
}

func (p *Proposer) Propose(ctx context.Context, description string, existing []entities.Entity) (*entities.EntitySpec, error) {
	if p == nil {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY not configured")
	}
	existingNames := make([]string, len(existing))
	for i, e := range existing {
		existingNames[i] = e.Name
	}
	userMsg := description
	if len(existingNames) > 0 {
		userMsg += "\n\nExisting entities you can reference: " + strings.Join(existingNames, ", ")
	}

	tool := anthropic.ToolParam{
		Name:        toolName,
		Description: anthropic.String("Propose an entity (table) definition for the ERP."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: entitySchemaProperties(),
			Required:   []string{"name", "display_name", "fields"},
		},
	}

	params := anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: 2048,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Tools:      []anthropic.ToolUnionParam{{OfTool: &tool}},
		ToolChoice: anthropic.ToolChoiceParamOfTool(toolName),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userMsg)),
		},
		CacheControl: anthropic.NewCacheControlEphemeralParam(),
	}

	resp, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("claude: %w", err)
	}

	for _, block := range resp.Content {
		if block.Type != "tool_use" || block.Name != toolName {
			continue
		}
		var spec entities.EntitySpec
		if err := json.Unmarshal(block.Input, &spec); err != nil {
			return nil, fmt.Errorf("unmarshal tool input: %w", err)
		}
		if err := spec.Validate(); err != nil {
			return nil, fmt.Errorf("claude returned invalid spec: %w", err)
		}
		return &spec, nil
	}
	return nil, fmt.Errorf("claude did not call %s", toolName)
}

