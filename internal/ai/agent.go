package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sashabaranov/go-openai"

	"github.com/openrow/openrow/internal/entities"
	"github.com/openrow/openrow/internal/llm"
	"github.com/openrow/openrow/internal/reports"
)

const agentSystemPrompt = `You are the assistant inside OpenRow, an AI-native operations platform.

You design and operate the user's database through tools. Each user turn: use tools as needed, then briefly describe what you did (1-3 sentences max, no preamble). If no action is required, just answer.

Data modeling rules:
- Entity "name" must match ^[a-z][a-z0-9_]{0,62}$, usually plural (customers, invoices).
- Never create id / created_at / updated_at fields — the system adds them.
- Data types: text, integer, bigint, numeric, boolean, date, timestamptz, uuid, jsonb, reference.
- For reference fields, set reference_entity to the target's name. Only reference entities that currently exist.
- Don't invent fields the user didn't imply. If they say "a customer with name and email", use exactly those two fields.

Dashboards and reports:
- A dashboard is a named page; it contains one or more reports (widgets).
- A report has a widget_type (kpi | bar | line | area | pie | table), a query_spec, and options.
- query_spec shape: { entity, filters?, group_by?, series_by?, aggregate?, sort?, limit?, date_filter_field?, compare_period? }.
- kpi: aggregate required, no group_by, no series_by. bar/line/area: aggregate + group_by required; series_by optional. pie: aggregate + group_by, no series_by. table: no aggregate, no series_by.
- For time-series reports, set group_by.bucket to "day" | "week" | "month" | "quarter" | "year" and the field must be a date/timestamp column.
- Aggregate fns: count | sum | avg | min | max. sum/avg need numeric fields.
- Before creating a dashboard that references an entity that doesn't exist yet, either create the entity first or tell the user you'll need them to confirm adding it.
- Prefer 2-4 reports per dashboard, widths chosen from 3/4/6/8/12 in a 12-column grid.
- For time-scoped reports (e.g. "revenue this month"), set query_spec.date_filter_field to the timestamp/date column users expect to scope by (usually created_at or a domain date field like invoice_issue_date). This makes them respond to the dashboard's date range picker.
- Never invent a filter value for a categorical text field. Before you filter by e.g. direction/status/type, call query_rows on the entity to see the actual values in use. Then match exactly — "income" vs "in" matters.
- Prefer non-nullable date columns for group_by. Nullable columns (like payment_date, when it stays empty for unpaid rows) silently drop records from the series. If unsure, call query_rows first and check for NULLs. A safe default for "revenue by month" is invoice_issue_date or created_at, not payment_date.

series_by / multi-dimensional charts:
- Use series_by when the user wants to compare two dimensions on one chart (e.g. "income vs expenses by month" → group_by=invoice_issue_date bucket=month, series_by=direction).
- For "revenue per customer over time": group_by=date bucketed, series_by=customer (a reference field — refs resolve to their label).
- Bar widgets default to grouped bars. Pass options.stacked=true for stacked bars (ideal for composition metrics like "monthly spend broken down by category").
- area is just a filled line; use it for cumulative or "total over time" reads where a line's enclosed area conveys magnitude.

KPI comparisons:
- Set query_spec.compare_period to "previous_period" or "previous_year" when the user asks "vs last month/year/period" or when a comparison would obviously help (MoM, YoY).
- compare_period requires query_spec.date_filter_field to be set; otherwise the comparison is silently skipped.

Number formatting:
- Use options.number_format to format KPI values and axis/tooltip numbers: "currency" (with currency_code, e.g. CZK/USD/EUR), "percent", "integer", or "decimal" (default).
- Default to currency with CZK for Czech-context financial data; USD/EUR otherwise if the user has indicated preference.

Top-N charts:
- For "top N customers by revenue" or similar: group_by the entity field, sort={field:"value", dir:"desc"}, and set limit to N.

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

// Agent wraps the tool-calling loop against any OpenAI-compatible provider.
// Provider/model/api-key are resolved per-tenant at request time via llm.Service.
type Agent struct {
	llm        *llm.Service
	entities   *entities.Service
	dashboards *reports.Service
}

func NewAgent(llmSvc *llm.Service, ent *entities.Service, dash *reports.Service) *Agent {
	return &Agent{llm: llmSvc, entities: ent, dashboards: dash}
}

// Run executes a single conversation turn.
func (a *Agent) Run(ctx context.Context, tenantID, pgSchema string, history []ChatTurn, userMessage string) (*ChatTurn, error) {
	if a == nil {
		return nil, errors.New("agent not available")
	}
	cfg, err := a.llm.Resolve(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	client := llm.NewClient(cfg)

	existing, err := a.entities.List(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list entities: %w", err)
	}

	tools := a.BuildToolset(ctx, tenantID, pgSchema)
	msgs := buildMessageHistory(history, userMessage, existing)

	var actions []Action
	const maxIterations = 8

	for iter := 0; iter < maxIterations; iter++ {
		resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model:      cfg.Model,
			Messages:   msgs,
			Tools:      tools.ToolParams(),
			ToolChoice: "auto",
			MaxTokens:  2048,
		})
		if err != nil {
			return nil, fmt.Errorf("llm: %w", err)
		}
		if len(resp.Choices) == 0 {
			return nil, errors.New("llm returned no choices")
		}
		choice := resp.Choices[0]

		if len(choice.Message.ToolCalls) == 0 {
			return &ChatTurn{Role: "assistant", Text: choice.Message.Content, Actions: actions}, nil
		}

		// Append the assistant message (with its tool calls) so subsequent tool
		// messages refer to each call by id.
		msgs = append(msgs, choice.Message)

		for _, tc := range choice.Message.ToolCalls {
			exec := tools.Invoke(ctx, tc.Function.Name, json.RawMessage(tc.Function.Arguments))
			actions = append(actions, Action{
				Tool:       tc.Function.Name,
				Input:      json.RawMessage(tc.Function.Arguments),
				Summary:    exec.Summary,
				EntityName: exec.EntityName,
				Error:      exec.ErrMsg(),
			})
			msgs = append(msgs, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				ToolCallID: tc.ID,
				Content:    exec.ResultText(),
			})
		}
	}

	return nil, fmt.Errorf("agent exceeded %d tool iterations", maxIterations)
}

func buildMessageHistory(history []ChatTurn, newUserMessage string, existing []entities.Entity) []openai.ChatCompletionMessage {
	msgs := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: agentSystemPrompt},
	}
	for _, t := range history {
		switch t.Role {
		case "user":
			msgs = append(msgs, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: t.Text,
			})
		case "assistant":
			if t.Text != "" {
				msgs = append(msgs, openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleAssistant,
					Content: t.Text,
				})
			}
		}
	}
	var suffix string
	if len(existing) > 0 {
		names := make([]string, 0, len(existing))
		for _, e := range existing {
			names = append(names, e.Name)
		}
		suffix = "\n\n[current entities: " + joinNames(names) + "]"
	}
	msgs = append(msgs, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: newUserMessage + suffix,
	})
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
