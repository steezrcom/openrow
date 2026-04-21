package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/steezrcom/steezr-erp/internal/entities"
	"github.com/steezrcom/steezr-erp/internal/reports"
)

const agentSystemPrompt = `You are the assistant inside steezr, an AI-native ERP.

You design and operate the user's database through tools. Each user turn: use tools as needed, then briefly describe what you did (1-3 sentences max, no preamble). If no action is required, just answer.

Data modeling rules:
- Entity "name" must match ^[a-z][a-z0-9_]{0,62}$, usually plural (customers, invoices).
- Never create id / created_at / updated_at fields — the system adds them.
- Data types: text, integer, bigint, numeric, boolean, date, timestamptz, uuid, jsonb, reference.
- For reference fields, set reference_entity to the target's name. Only reference entities that currently exist.
- Don't invent fields the user didn't imply. If they say "a customer with name and email", use exactly those two fields.

Dashboards and reports:
- A dashboard is a named page; it contains one or more reports (widgets).
- A report has a widget_type (kpi | bar | line | pie | table) and a query_spec.
- query_spec shape: { entity, filters?, group_by?, aggregate?, sort?, limit? }.
- kpi: aggregate required, no group_by. bar/line/pie: aggregate + group_by both required. table: neither.
- For time-series reports, set group_by.bucket to "day" | "week" | "month" | "quarter" | "year" and the field must be a date/timestamp column.
- Aggregate fns: count | sum | avg | min | max. sum/avg need numeric fields.
- Before creating a dashboard that references an entity that doesn't exist yet, either create the entity first or tell the user you'll need them to confirm adding it.
- Prefer 2-4 reports per dashboard, widths chosen from 3/4/6/8/12 in a 12-column grid.
- For time-scoped reports (e.g. "revenue this month"), set query_spec.date_filter_field to the timestamp/date column users expect to scope by (usually created_at or a domain date field like invoice_issue_date). This makes them respond to the dashboard's date range picker.
- Never invent a filter value for a categorical text field. Before you filter by e.g. direction/status/type, call query_rows on the entity to see the actual values in use. Then match exactly — "income" vs "in" matters.
- Prefer non-nullable date columns for group_by. Nullable columns (like payment_date, when it stays empty for unpaid rows) silently drop records from the series. If unsure, call query_rows first and check for NULLs. A safe default for "revenue by month" is invoice_issue_date or created_at, not payment_date.

Mutations:
- Only mutate what the user asked for. If the user asks to add an entity, don't also add sample rows unless they said so.
- Before add_row/update_row, if you're unsure which entity, call list_entities.
- Treat destructive operations (delete_row, delete_dashboard, drop_field) carefully; confirm if the target is ambiguous.

Style: terse, concrete. Use the user's language.`

// ChatTurn is the client-visible message. Assistant turns may carry a list of actions (tool calls).
type ChatTurn struct {
	Role    string   `json:"role"` // "user" or "assistant"
	Text    string   `json:"text"`
	Actions []Action `json:"actions,omitempty"`
}

// Action records one tool execution inside an assistant turn, for UI display + query invalidation.
type Action struct {
	Tool       string          `json:"tool"`
	Input      json.RawMessage `json:"input"`
	Summary    string          `json:"summary"`
	EntityName string          `json:"entity_name,omitempty"`
	Error      string          `json:"error,omitempty"`
}

// Agent wraps the Claude tool loop with the fixed set of ERP tools.
type Agent struct {
	client     anthropic.Client
	model      anthropic.Model
	entities   *entities.Service
	dashboards *reports.Service
}

func NewAgent(apiKey string, ent *entities.Service, dash *reports.Service) *Agent {
	if apiKey == "" {
		return nil
	}
	return &Agent{
		client:     anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:      anthropic.ModelClaudeSonnet4_6,
		entities:   ent,
		dashboards: dash,
	}
}

// Run executes a single conversation turn. `history` is prior turns (user + assistant).
// userMessage is the new user message being sent.
func (a *Agent) Run(ctx context.Context, tenantID, pgSchema string, history []ChatTurn, userMessage string) (*ChatTurn, error) {
	if a == nil {
		return nil, errors.New("ANTHROPIC_API_KEY not configured")
	}

	existing, err := a.entities.List(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list entities: %w", err)
	}

	tools := a.buildTools(ctx, tenantID, pgSchema)

	msgs := buildMessageHistory(history, userMessage, existing)

	params := anthropic.MessageNewParams{
		Model:     a.model,
		MaxTokens: 2048,
		System: []anthropic.TextBlockParam{
			{Text: agentSystemPrompt},
		},
		Tools:        tools.toolParams(),
		Messages:     msgs,
		CacheControl: anthropic.NewCacheControlEphemeralParam(),
	}

	var actions []Action
	const maxIterations = 8

	for iter := 0; iter < maxIterations; iter++ {
		resp, err := a.client.Messages.New(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("claude: %w", err)
		}

		var (
			narrative  string
			toolUses   []toolUseRef
			nextBlocks []anthropic.ContentBlockParamUnion
		)

		for _, block := range resp.Content {
			switch block.Type {
			case "text":
				narrative += block.Text
				nextBlocks = append(nextBlocks, anthropic.NewTextBlock(block.Text))
			case "tool_use":
				toolUses = append(toolUses, toolUseRef{
					ID:    block.ID,
					Name:  block.Name,
					Input: block.Input,
				})
				nextBlocks = append(nextBlocks, anthropic.ContentBlockParamUnion{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    block.ID,
						Name:  block.Name,
						Input: block.Input,
					},
				})
			}
		}

		if len(toolUses) == 0 {
			return &ChatTurn{Role: "assistant", Text: narrative, Actions: actions}, nil
		}

		// Record the assistant's full message (text + tool_use blocks) for the next call.
		params.Messages = append(params.Messages, anthropic.MessageParam{
			Role:    anthropic.MessageParamRoleAssistant,
			Content: nextBlocks,
		})

		// Execute each tool and build a user message with the results.
		var resultBlocks []anthropic.ContentBlockParamUnion
		for _, tu := range toolUses {
			exec := tools.run(ctx, tu.Name, tu.Input)
			actions = append(actions, Action{
				Tool:       tu.Name,
				Input:      tu.Input,
				Summary:    exec.Summary,
				EntityName: exec.EntityName,
				Error:      exec.ErrMsg(),
			})
			resultBlocks = append(resultBlocks, anthropic.ContentBlockParamUnion{
				OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: tu.ID,
					Content: []anthropic.ToolResultBlockParamContentUnion{
						{OfText: &anthropic.TextBlockParam{Text: exec.ResultText()}},
					},
					IsError: anthropic.Bool(exec.Err != nil),
				},
			})
		}

		params.Messages = append(params.Messages, anthropic.MessageParam{
			Role:    anthropic.MessageParamRoleUser,
			Content: resultBlocks,
		})
	}

	return nil, fmt.Errorf("agent exceeded %d tool iterations", maxIterations)
}

type toolUseRef struct {
	ID    string
	Name  string
	Input json.RawMessage
}

func buildMessageHistory(history []ChatTurn, newUserMessage string, existing []entities.Entity) []anthropic.MessageParam {
	msgs := make([]anthropic.MessageParam, 0, len(history)+1)
	for _, t := range history {
		if t.Role == "user" {
			msgs = append(msgs, anthropic.NewUserMessage(anthropic.NewTextBlock(t.Text)))
		} else if t.Role == "assistant" && t.Text != "" {
			msgs = append(msgs, anthropic.NewAssistantMessage(anthropic.NewTextBlock(t.Text)))
		}
	}

	var ctx string
	if len(existing) > 0 {
		names := make([]string, 0, len(existing))
		for _, e := range existing {
			names = append(names, e.Name)
		}
		ctx = "\n\n[current entities: " + joinNames(names) + "]"
	}
	msgs = append(msgs, anthropic.NewUserMessage(anthropic.NewTextBlock(newUserMessage+ctx)))
	return msgs
}

func joinNames(ns []string) string {
	out := ""
	for i, n := range ns {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}
