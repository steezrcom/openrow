package flows

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openrow/openrow/internal/ai"
	"github.com/openrow/openrow/internal/connectors"
)

// ChatTools returns an ai.ToolProvider that contributes the
// flow-authoring tools to the chat agent:
//   - list_installed_connectors: which connectors/actions are available
//   - preflight_flow: validate a draft flow's allowlist + trigger config
//   - install_flow: create the flow (always starts in dry_run mode)
//
// These live in the flows package rather than ai to avoid an import cycle
// (flows.Runner already imports ai.Toolset).
func ChatTools(svc *Service, conn *connectors.Service) ai.ToolProvider {
	return func(ctx context.Context, tenantID, pgSchema string) []ai.Tool {
		return []ai.Tool{
			listInstalledConnectorsTool(conn, tenantID),
			preflightFlowTool(tenantID),
			installFlowTool(svc, tenantID),
		}
	}
}

// --- list_installed_connectors -------------------------------------------

type connectorInfo struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Actions []actionInfo   `json:"actions"`
}

type actionInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Mutates     bool   `json:"mutates"`
	ToolName    string `json:"tool_name"`
}

func listInstalledConnectorsTool(conn *connectors.Service, tenantID string) ai.Tool {
	return ai.Tool{
		Name:        "list_installed_connectors",
		Description: "Return the connectors this workspace has configured and the actions each exposes (tool_name included). Call this before drafting a flow that depends on an external service. If the user asks for something that needs a missing connector, tell them which one to install.",
		Schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		Handler: func(ctx context.Context, _ json.RawMessage) ai.ExecResult {
			if conn == nil {
				return ai.ExecResult{Result: []connectorInfo{}}
			}
			configs, err := conn.List(ctx, tenantID)
			if err != nil {
				return ai.ExecResult{Err: err}
			}
			out := make([]connectorInfo, 0)
			for _, cfg := range configs {
				if !cfg.Enabled {
					continue
				}
				d := connectors.Get(cfg.ConnectorID)
				if d == nil || d.Status != connectors.StatusAvailable {
					continue
				}
				acts := make([]actionInfo, 0, len(d.Actions))
				for _, a := range d.Actions {
					acts = append(acts, actionInfo{
						ID:          a.ID,
						Name:        a.Name,
						Description: a.Description,
						Mutates:     a.Mutates,
						ToolName:    "connector_" + cfg.ConnectorID + "_" + a.ID,
					})
				}
				out = append(out, connectorInfo{
					ID:      cfg.ConnectorID,
					Name:    d.Name,
					Actions: acts,
				})
			}
			return ai.ExecResult{
				Summary: fmt.Sprintf("%d connector(s) installed", len(out)),
				Result:  out,
			}
		},
	}
}

// --- preflight_flow ------------------------------------------------------

type preflightInput struct {
	ToolAllowlist []string `json:"tool_allowlist"`
	TriggerKind   string   `json:"trigger_kind"`
	TriggerConfig any      `json:"trigger_config,omitempty"`
}

func preflightFlowTool(tenantID string) ai.Tool {
	_ = tenantID
	return ai.Tool{
		Name:        "preflight_flow",
		Description: "Validate a proposed flow spec before installing it. Checks the trigger_kind is supported and returns issues it finds. Always call this before install_flow.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tool_allowlist": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Tool names the flow will be allowed to call, e.g. [\"query_rows\", \"connector_fakturoid_mark_invoice_paid\"].",
				},
				"trigger_kind": map[string]any{
					"type":        "string",
					"description": "manual | entity_event | webhook",
				},
				"trigger_config": map[string]any{
					"type":        "object",
					"description": "Trigger-specific config. For entity_event: { entity, events: [insert|update|delete] }.",
				},
			},
			"required": []string{"tool_allowlist", "trigger_kind"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) ai.ExecResult {
			var in preflightInput
			if err := json.Unmarshal(raw, &in); err != nil {
				return ai.ExecResult{Err: fmt.Errorf("parse input: %w", err)}
			}
			issues := []string{}
			if err := validateTrigger(TriggerKind(in.TriggerKind)); err != nil {
				issues = append(issues, fmt.Sprintf("unsupported trigger_kind %q", in.TriggerKind))
			}
			if len(in.ToolAllowlist) == 0 {
				issues = append(issues, "tool_allowlist is empty")
			}
			return ai.ExecResult{
				Summary: fmt.Sprintf("%d issue(s)", len(issues)),
				Result:  map[string]any{"ok": len(issues) == 0, "issues": issues},
			}
		},
	}
}

// --- install_flow --------------------------------------------------------

type installInput struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Goal          string          `json:"goal"`
	TriggerKind   string          `json:"trigger_kind"`
	TriggerConfig json.RawMessage `json:"trigger_config,omitempty"`
	ToolAllowlist []string        `json:"tool_allowlist"`
}

func installFlowTool(svc *Service, tenantID string) ai.Tool {
	return ai.Tool{
		Name: "install_flow",
		// Mutates=true so flows that for some reason allowlist this tool
		// still get intercepted by dry_run / approve modes. Flows in the
		// normal course wouldn't include it; this is defense in depth.
		Mutates: true,
		Description: "Create a new flow in dry_run mode. Always runs preflight_flow first and confirms the proposal in prose with the user before calling this. Returns the flow id (and webhook URL for webhook triggers) so you can tell the user where to go.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":           map[string]any{"type": "string", "description": "Short human-readable name."},
				"description":    map[string]any{"type": "string"},
				"goal":           map[string]any{"type": "string", "description": "Full instruction the runtime agent follows. Write it as you would tell a careful colleague."},
				"trigger_kind":   map[string]any{"type": "string", "description": "manual | entity_event | webhook"},
				"trigger_config": map[string]any{"type": "object", "description": "For entity_event: { entity, events: [insert|update|delete] }. Omit for manual/webhook."},
				"tool_allowlist": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Exact tool names. Use list_installed_connectors + list_entities to ground these in reality."},
			},
			"required": []string{"name", "goal", "trigger_kind", "tool_allowlist"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) ai.ExecResult {
			var in installInput
			if err := json.Unmarshal(raw, &in); err != nil {
				return ai.ExecResult{Err: fmt.Errorf("parse input: %w", err)}
			}
			if strings.TrimSpace(in.TriggerKind) == "" {
				in.TriggerKind = "manual"
			}
			res, err := svc.Create(ctx, tenantID, CreateFlowInput{
				Name:          in.Name,
				Description:   in.Description,
				Goal:          in.Goal,
				TriggerKind:   TriggerKind(in.TriggerKind),
				TriggerConfig: in.TriggerConfig,
				ToolAllowlist: in.ToolAllowlist,
				Mode:          ModeDryRun,
			})
			if err != nil {
				return ai.ExecResult{Err: err}
			}
			result := map[string]any{
				"flow_id":      res.Flow.ID,
				"name":         res.Flow.Name,
				"mode":         string(res.Flow.Mode),
				"trigger_kind": string(res.Flow.TriggerKind),
				"url":          "/app/flows/" + res.Flow.ID,
			}
			if res.WebhookTokenOnce != "" {
				// Returned to the LLM so it can show the user. Only seen
				// once — the agent should tell the user to copy it.
				result["webhook_token_once"] = res.WebhookTokenOnce
				result["webhook_path"] = "/webhooks/<tenant_slug>/" + res.Flow.ID + "?token=<token>"
			}
			return ai.ExecResult{
				Summary: fmt.Sprintf("Installed flow %q (dry_run)", res.Flow.Name),
				Result:  result,
			}
		},
	}
}
