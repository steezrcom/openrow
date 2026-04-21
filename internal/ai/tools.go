package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/sashabaranov/go-openai"

	"github.com/openrow/openrow/internal/connectors"
	"github.com/openrow/openrow/internal/entities"
	"github.com/openrow/openrow/internal/reports"
	"github.com/openrow/openrow/internal/templates"
)

// Tool describes one action the agent can invoke.
// Mutates distinguishes write-class tools from reads — flows use this to
// enforce dry-run and approval modes at the tool-call boundary.
type Tool struct {
	Name        string
	Description string
	Schema      map[string]any // full JSON Schema "object" — {type, properties, required}
	Mutates     bool
	Handler     func(ctx context.Context, input json.RawMessage) ExecResult
}

type ExecResult struct {
	Summary    string
	EntityName string
	Result     any
	Err        error
}

func (e ExecResult) ErrMsg() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e ExecResult) ResultText() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	if e.Result == nil {
		return e.Summary
	}
	b, err := json.Marshal(e.Result)
	if err != nil {
		return fmt.Sprintf("ok: %v", e.Result)
	}
	return string(b)
}

// Toolset bundles tools for one request (closed over tenantID/pgSchema).
type Toolset struct {
	tools []Tool
	index map[string]Tool
}

// NewToolset wraps an ordered slice of tools into a Toolset.
// Used by the flow runner to build a filtered subset from BuildToolset.
func NewToolset(tools []Tool) *Toolset {
	idx := make(map[string]Tool, len(tools))
	for _, t := range tools {
		idx[t.Name] = t
	}
	return &Toolset{tools: tools, index: idx}
}

// Tools returns the ordered list of tools, for inspection (e.g. by the flow
// runner, which needs to enumerate names + Mutates metadata).
func (ts *Toolset) Tools() []Tool { return ts.tools }

// Get returns the tool with the given name, or zero value + false.
func (ts *Toolset) Get(name string) (Tool, bool) {
	t, ok := ts.index[name]
	return t, ok
}

func (ts *Toolset) ToolParams() []openai.Tool {
	out := make([]openai.Tool, len(ts.tools))
	for i, t := range ts.tools {
		out[i] = openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Schema,
			},
		}
	}
	return out
}

// Invoke runs the named tool. Returns an error ExecResult for unknown names
// so the caller can still feed a response back to the LLM.
func (ts *Toolset) Invoke(ctx context.Context, name string, input json.RawMessage) ExecResult {
	t, ok := ts.index[name]
	if !ok {
		return ExecResult{Err: fmt.Errorf("unknown tool %q", name)}
	}
	return t.Handler(ctx, input)
}

// objectSchema wraps a properties map into a JSON Schema object with an
// optional required list. Keeps tool definitions readable.
func objectSchema(properties map[string]any, required ...string) map[string]any {
	out := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

// BuildToolset constructs a Toolset closed over the given tenant + schema.
// Call it once per run; the tools hold references to the tenant context.
// Includes the built-in entity/dashboard tools plus one tool per action on
// each installed + enabled connector the tenant has configured.
func (a *Agent) BuildToolset(ctx context.Context, tenantID, pgSchema string) *Toolset {
	svc := a.entities
	dash := a.dashboards

	ts := &Toolset{index: map[string]Tool{}}
	add := func(t Tool) {
		ts.tools = append(ts.tools, t)
		ts.index[t.Name] = t
	}

	add(Tool{
		Name:        "list_entities",
		Description: "Return all entities in the current organization with their fields. Call this whenever you're unsure which entities exist or need to verify a name.",
		Schema:      objectSchema(map[string]any{}),
		Handler: func(ctx context.Context, _ json.RawMessage) ExecResult {
			es, err := svc.List(ctx, tenantID)
			if err != nil {
				return ExecResult{Err: err}
			}
			out := make([]entities.Entity, len(es))
			for i, e := range es {
				detailed, err := svc.Get(ctx, tenantID, e.Name)
				if err != nil {
					return ExecResult{Err: err}
				}
				out[i] = *detailed
			}
			return ExecResult{Summary: fmt.Sprintf("Listed %d entities", len(out)), Result: out}
		},
	})

	add(Tool{
		Name:        "create_entity",
		Mutates:     true,
		Description: "Create a new entity (database table). Validates field names; id/created_at/updated_at are added automatically.",
		Schema:      objectSchema(entitySchemaProperties(), "name", "display_name", "fields"),
		Handler: func(ctx context.Context, input json.RawMessage) ExecResult {
			var spec entities.EntitySpec
			if err := json.Unmarshal(input, &spec); err != nil {
				return ExecResult{Err: fmt.Errorf("invalid input: %w", err)}
			}
			ent, err := svc.Create(ctx, tenantID, pgSchema, &spec)
			if err != nil {
				return ExecResult{Err: err}
			}
			return ExecResult{
				Summary:    fmt.Sprintf("Created entity %q with %d fields", ent.Name, len(ent.Fields)),
				EntityName: ent.Name,
				Result:     ent,
			}
		},
	})

	add(Tool{
		Name:        "add_row",
		Mutates:     true,
		Description: "Insert a new row into an entity's table. values is a string→string map; the server coerces by field type.",
		Schema: objectSchema(map[string]any{
			"entity": stringProp("Entity name (lowercase identifier)."),
			"values": map[string]any{
				"type":                 "object",
				"description":          "Field name to value, as strings. For boolean, use \"true\"/\"false\".",
				"additionalProperties": map[string]any{"type": "string"},
			},
		}, "entity", "values"),
		Handler: func(ctx context.Context, input json.RawMessage) ExecResult {
			var req struct {
				Entity string            `json:"entity"`
				Values map[string]string `json:"values"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return ExecResult{Err: err}
			}
			ent, err := svc.Get(ctx, tenantID, req.Entity)
			if err != nil {
				return ExecResult{Err: fmt.Errorf("entity %q not found", req.Entity)}
			}
			id, err := svc.InsertRow(ctx, pgSchema, ent, req.Values)
			if err != nil {
				return ExecResult{Err: err}
			}
			return ExecResult{
				Summary:    fmt.Sprintf("Added row to %q", ent.Name),
				EntityName: ent.Name,
				Result:     map[string]string{"id": id},
			}
		},
	})

	add(Tool{
		Name:        "update_row",
		Mutates:     true,
		Description: "Update an existing row. Only include the fields you want to change. To clear a non-required field, pass an empty string.",
		Schema: objectSchema(map[string]any{
			"entity": stringProp("Entity name."),
			"id":     stringProp("Row UUID."),
			"values": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
			},
		}, "entity", "id", "values"),
		Handler: func(ctx context.Context, input json.RawMessage) ExecResult {
			var req struct {
				Entity string            `json:"entity"`
				ID     string            `json:"id"`
				Values map[string]string `json:"values"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return ExecResult{Err: err}
			}
			ent, err := svc.Get(ctx, tenantID, req.Entity)
			if err != nil {
				return ExecResult{Err: fmt.Errorf("entity %q not found", req.Entity)}
			}
			if err := svc.UpdateRow(ctx, pgSchema, ent, req.ID, req.Values); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return ExecResult{Err: fmt.Errorf("row %s not found", req.ID)}
				}
				return ExecResult{Err: err}
			}
			return ExecResult{
				Summary:    fmt.Sprintf("Updated row in %q", ent.Name),
				EntityName: ent.Name,
				Result:     map[string]string{"id": req.ID},
			}
		},
	})

	add(Tool{
		Name:        "delete_row",
		Mutates:     true,
		Description: "Delete a row by id.",
		Schema: objectSchema(map[string]any{
			"entity": stringProp("Entity name."),
			"id":     stringProp("Row UUID."),
		}, "entity", "id"),
		Handler: func(ctx context.Context, input json.RawMessage) ExecResult {
			var req struct {
				Entity string `json:"entity"`
				ID     string `json:"id"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return ExecResult{Err: err}
			}
			ent, err := svc.Get(ctx, tenantID, req.Entity)
			if err != nil {
				return ExecResult{Err: fmt.Errorf("entity %q not found", req.Entity)}
			}
			if err := svc.DeleteRow(ctx, pgSchema, ent, req.ID); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return ExecResult{Err: fmt.Errorf("row %s not found", req.ID)}
				}
				return ExecResult{Err: err}
			}
			return ExecResult{
				Summary:    fmt.Sprintf("Deleted row from %q", ent.Name),
				EntityName: ent.Name,
			}
		},
	})

	add(Tool{
		Name:        "add_field",
		Mutates:     true,
		Description: "Add a new column to an existing entity. Use when the user asks to extend a table with a new attribute.",
		Schema: objectSchema(map[string]any{
			"entity": stringProp("Target entity name."),
			"field":  objectSchema(fieldSchemaProperties(), "name", "display_name", "data_type"),
		}, "entity", "field"),
		Handler: func(ctx context.Context, input json.RawMessage) ExecResult {
			var req struct {
				Entity string             `json:"entity"`
				Field  entities.FieldSpec `json:"field"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return ExecResult{Err: err}
			}
			ent, err := svc.AddField(ctx, tenantID, pgSchema, req.Entity, req.Field)
			if err != nil {
				return ExecResult{Err: err}
			}
			return ExecResult{
				Summary:    fmt.Sprintf("Added field %q to %q", req.Field.Name, req.Entity),
				EntityName: req.Entity,
				Result:     ent,
			}
		},
	})

	add(Tool{
		Name:        "drop_field",
		Mutates:     true,
		Description: "Remove a column from an entity. Data in that column is lost. Only call after confirming with the user or when explicitly requested.",
		Schema: objectSchema(map[string]any{
			"entity": stringProp("Entity name."),
			"field":  stringProp("Field name to drop."),
		}, "entity", "field"),
		Handler: func(ctx context.Context, input json.RawMessage) ExecResult {
			var req struct {
				Entity string `json:"entity"`
				Field  string `json:"field"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return ExecResult{Err: err}
			}
			if err := svc.DropField(ctx, tenantID, pgSchema, req.Entity, req.Field); err != nil {
				return ExecResult{Err: err}
			}
			return ExecResult{
				Summary:    fmt.Sprintf("Dropped field %q from %q", req.Field, req.Entity),
				EntityName: req.Entity,
			}
		},
	})

	add(Tool{
		Name:        "list_templates",
		Description: "List pre-built workspace templates (entities + starter dashboards).",
		Schema:      objectSchema(map[string]any{}),
		Handler: func(_ context.Context, _ json.RawMessage) ExecResult {
			ts := templates.All()
			out := make([]map[string]string, len(ts))
			for i, t := range ts {
				out[i] = map[string]string{"id": t.ID, "name": t.Name, "description": t.Description}
			}
			return ExecResult{Summary: fmt.Sprintf("Listed %d templates", len(out)), Result: out}
		},
	})

	add(Tool{
		Name:    "apply_template",
		Mutates: true,
		Description: "Install a pre-built template (entities + default dashboard) in the current workspace. " +
			"Fails if any template entity name conflicts with an existing entity, so prefer fresh workspaces.",
		Schema: objectSchema(map[string]any{
			"id": stringProp("Template id; currently 'agency' is the supported value."),
		}, "id"),
		Handler: func(ctx context.Context, input json.RawMessage) ExecResult {
			var req struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return ExecResult{Err: err}
			}
			t, ok := templates.Get(req.ID)
			if !ok {
				return ExecResult{Err: fmt.Errorf("template %q not found", req.ID)}
			}
			if err := t.Install(ctx, tenantID, pgSchema, svc, dash); err != nil {
				return ExecResult{Err: err}
			}
			return ExecResult{Summary: fmt.Sprintf("Installed template %q", req.ID)}
		},
	})

	add(Tool{
		Name:        "list_dashboards",
		Description: "List dashboards in the current organization (with report titles).",
		Schema:      objectSchema(map[string]any{}),
		Handler: func(ctx context.Context, _ json.RawMessage) ExecResult {
			ds, err := dash.List(ctx, tenantID)
			if err != nil {
				return ExecResult{Err: err}
			}
			return ExecResult{
				Summary: fmt.Sprintf("Listed %d dashboards", len(ds)),
				Result:  ds,
			}
		},
	})

	add(Tool{
		Name:        "get_dashboard",
		Description: "Get one dashboard with its reports.",
		Schema:      objectSchema(map[string]any{"slug": stringProp("Dashboard slug.")}, "slug"),
		Handler: func(ctx context.Context, input json.RawMessage) ExecResult {
			var req struct {
				Slug string `json:"slug"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return ExecResult{Err: err}
			}
			d, err := dash.Get(ctx, tenantID, req.Slug)
			if err != nil {
				return ExecResult{Err: err}
			}
			return ExecResult{Summary: fmt.Sprintf("Fetched dashboard %q", d.Slug), Result: d}
		},
	})

	add(Tool{
		Name:        "create_dashboard",
		Mutates:     true,
		Description: "Create a new dashboard. Optionally include an initial list of reports (widgets). Every report has a query_spec; see the system prompt for the schema.",
		Schema: objectSchema(map[string]any{
			"name":        stringProp("Human-facing title, e.g. 'Financial overview'."),
			"slug":        stringProp("Optional machine slug; auto-generated from name if omitted."),
			"description": stringProp("Optional one-line description."),
			"reports": map[string]any{
				"type":        "array",
				"description": "Initial reports to include on this dashboard.",
				"items":       objectSchema(reportSchemaProperties(), "title", "widget_type", "query_spec"),
			},
		}, "name"),
		Handler: func(ctx context.Context, input json.RawMessage) ExecResult {
			var in reports.CreateDashboardInput
			if err := json.Unmarshal(input, &in); err != nil {
				return ExecResult{Err: err}
			}
			d, err := dash.Create(ctx, tenantID, in)
			if err != nil {
				return ExecResult{Err: err}
			}
			return ExecResult{
				Summary: fmt.Sprintf("Created dashboard %q with %d reports", d.Slug, len(d.Reports)),
				Result:  d,
			}
		},
	})

	add(Tool{
		Name:        "add_report",
		Mutates:     true,
		Description: "Add a report to an existing dashboard.",
		Schema: objectSchema(map[string]any{
			"slug":   stringProp("Dashboard slug."),
			"report": objectSchema(reportSchemaProperties(), "title", "widget_type", "query_spec"),
		}, "slug", "report"),
		Handler: func(ctx context.Context, input json.RawMessage) ExecResult {
			var req struct {
				Slug   string                    `json:"slug"`
				Report reports.CreateReportInput `json:"report"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return ExecResult{Err: err}
			}
			r, err := dash.AddReport(ctx, tenantID, req.Slug, req.Report)
			if err != nil {
				return ExecResult{Err: err}
			}
			return ExecResult{Summary: fmt.Sprintf("Added report %q to %q", r.Title, req.Slug), Result: r}
		},
	})

	add(Tool{
		Name:        "update_report",
		Mutates:     true,
		Description: "Edit an existing report's title, subtitle, widget type, query spec, or width.",
		Schema: objectSchema(map[string]any{
			"id":          stringProp("Report id."),
			"title":       stringProp("New title."),
			"subtitle":    stringProp("New subtitle."),
			"widget_type": stringProp("kpi | bar | line | area | pie | table."),
			"width":       map[string]any{"type": "integer", "description": "New width (1-12)."},
			"query_spec":  map[string]any{"type": "object", "description": "Replacement query_spec."},
		}, "id"),
		Handler: func(ctx context.Context, input json.RawMessage) ExecResult {
			var req struct {
				ID         string              `json:"id"`
				Title      *string             `json:"title,omitempty"`
				Subtitle   *string             `json:"subtitle,omitempty"`
				WidgetType *reports.WidgetType `json:"widget_type,omitempty"`
				QuerySpec  *reports.QuerySpec  `json:"query_spec,omitempty"`
				Width      *int                `json:"width,omitempty"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return ExecResult{Err: err}
			}
			if err := dash.UpdateReport(ctx, tenantID, req.ID, reports.UpdateReportInput{
				Title: req.Title, Subtitle: req.Subtitle,
				WidgetType: req.WidgetType, QuerySpec: req.QuerySpec, Width: req.Width,
			}); err != nil {
				return ExecResult{Err: err}
			}
			return ExecResult{Summary: "Updated report", Result: map[string]string{"id": req.ID}}
		},
	})

	add(Tool{
		Name:        "delete_report",
		Mutates:     true,
		Description: "Delete a report by id.",
		Schema:      objectSchema(map[string]any{"id": stringProp("Report id.")}, "id"),
		Handler: func(ctx context.Context, input json.RawMessage) ExecResult {
			var req struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return ExecResult{Err: err}
			}
			if err := dash.DeleteReport(ctx, tenantID, req.ID); err != nil {
				return ExecResult{Err: err}
			}
			return ExecResult{Summary: "Deleted report"}
		},
	})

	add(Tool{
		Name:        "delete_dashboard",
		Mutates:     true,
		Description: "Delete a dashboard and all its reports. Destructive; confirm with the user first.",
		Schema:      objectSchema(map[string]any{"slug": stringProp("Dashboard slug.")}, "slug"),
		Handler: func(ctx context.Context, input json.RawMessage) ExecResult {
			var req struct {
				Slug string `json:"slug"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return ExecResult{Err: err}
			}
			if err := dash.Delete(ctx, tenantID, req.Slug); err != nil {
				return ExecResult{Err: err}
			}
			return ExecResult{Summary: fmt.Sprintf("Deleted dashboard %q", req.Slug)}
		},
	})

	add(Tool{
		Name:        "query_rows",
		Description: "Return recent rows from an entity. Use this to answer questions about current data or verify state before mutating.",
		Schema: objectSchema(map[string]any{
			"entity": stringProp("Entity name."),
			"limit": map[string]any{
				"type":        "integer",
				"description": "Max rows. Default 20, max 200.",
			},
		}, "entity"),
		Handler: func(ctx context.Context, input json.RawMessage) ExecResult {
			var req struct {
				Entity string `json:"entity"`
				Limit  int    `json:"limit"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return ExecResult{Err: err}
			}
			if req.Limit <= 0 || req.Limit > 200 {
				req.Limit = 20
			}
			ent, err := svc.Get(ctx, tenantID, req.Entity)
			if err != nil {
				return ExecResult{Err: fmt.Errorf("entity %q not found", req.Entity)}
			}
			rows, err := svc.ListRows(ctx, pgSchema, ent, entities.ListOptions{Limit: req.Limit})
			if err != nil {
				return ExecResult{Err: err}
			}
			return ExecResult{
				Summary:    fmt.Sprintf("Fetched %d rows from %q", len(rows), ent.Name),
				EntityName: ent.Name,
				Result:     map[string]any{"count": len(rows), "rows": rows},
			}
		},
	})

	a.addConnectorTools(ctx, tenantID, add)

	return ts
}

// addConnectorTools appends one tool per Action on each installed + enabled
// connector the tenant has configured. Tools are named
// "connector_<id>_<action_id>" so allowlists can target them precisely
// (dots aren't allowed in OpenAI/Anthropic tool-name regexes, so we use
// underscores throughout). Connectors that aren't StatusAvailable are
// skipped.
func (a *Agent) addConnectorTools(ctx context.Context, tenantID string, add func(Tool)) {
	if a.connectors == nil {
		return
	}
	configs, err := a.connectors.List(ctx, tenantID)
	if err != nil {
		return
	}
	for _, cfgLoop := range configs {
		cfg := cfgLoop
		if !cfg.Enabled {
			continue
		}
		descriptor := connectors.Get(cfg.ConnectorID)
		if descriptor == nil || descriptor.Status != connectors.StatusAvailable {
			continue
		}
		for _, actionLoop := range descriptor.Actions {
			action := actionLoop
			toolName := "connector_" + cfg.ConnectorID + "_" + action.ID
			add(Tool{
				Name:        toolName,
				Description: action.Description,
				Schema:      action.Schema,
				Mutates:     action.Mutates,
				Handler: func(ctx context.Context, input json.RawMessage) ExecResult {
					result, err := action.Handler(ctx, cfg.Credentials, input)
					if err != nil {
						return ExecResult{Err: err}
					}
					return ExecResult{
						Summary: action.Name,
						Result:  result,
					}
				},
			})
		}
	}
}

func stringProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func entitySchemaProperties() map[string]any {
	return map[string]any{
		"name":         stringProp("Machine identifier, lowercase snake_case, matches ^[a-z][a-z0-9_]{0,62}$."),
		"display_name": stringProp("Human-facing label."),
		"description":  stringProp("Optional one-sentence description."),
		"fields": map[string]any{
			"type":        "array",
			"description": "Columns. Do not include id, created_at, or updated_at.",
			"items":       objectSchema(fieldSchemaProperties(), "name", "display_name", "data_type"),
		},
	}
}

func reportSchemaProperties() map[string]any {
	return map[string]any{
		"title":    stringProp("Report title, e.g. 'Revenue this month'."),
		"subtitle": stringProp("Optional secondary label."),
		"widget_type": map[string]any{
			"type": "string",
			"enum": []string{"kpi", "bar", "line", "area", "pie", "table"},
		},
		"width": map[string]any{
			"type":        "integer",
			"description": "Grid columns (1-12). Omit to default to 6.",
		},
		"query_spec": objectSchema(querySpecProperties(), "entity"),
		"options": map[string]any{
			"type":        "object",
			"description": "Per-widget render options (all optional).",
			"properties": map[string]any{
				"stacked": map[string]any{
					"type":        "boolean",
					"description": "Bar charts only. When true with series_by, renders stacked bars; otherwise grouped.",
				},
				"number_format": map[string]any{
					"type":        "string",
					"enum":        []string{"integer", "decimal", "currency", "percent"},
					"description": "Formats KPI values, chart tooltips, and axis ticks. Default is decimal.",
				},
				"currency_code": stringProp("ISO 4217 code like CZK, USD, EUR. Used when number_format is currency."),
				"locale":        stringProp("BCP 47 locale like en-US, cs-CZ. Default is the browser's."),
			},
		},
	}
}

func querySpecProperties() map[string]any {
	return map[string]any{
		"entity": stringProp("Entity (table) to query."),
		"filters": map[string]any{
			"type":        "array",
			"description": "WHERE clauses combined with AND.",
			"items": objectSchema(map[string]any{
				"field": stringProp("Column name."),
				"op": map[string]any{
					"type": "string",
					"enum": []string{
						"eq", "ne", "gt", "gte", "lt", "lte",
						"contains", "in", "is_null", "is_not_null",
					},
				},
				"value": map[string]any{
					"description": "Scalar or array (for in).",
				},
			}, "field", "op"),
		},
		"group_by": objectSchema(map[string]any{
			"field": stringProp("Field to group by."),
			"bucket": map[string]any{
				"type": "string",
				"enum": []string{"", "day", "week", "month", "quarter", "year"},
			},
		}, "field"),
		"series_by": objectSchema(map[string]any{
			"field": stringProp("Field to split series by."),
			"bucket": map[string]any{
				"type": "string",
				"enum": []string{"", "day", "week", "month", "quarter", "year"},
			},
		}, "field"),
		"aggregate": objectSchema(map[string]any{
			"fn": map[string]any{
				"type": "string",
				"enum": []string{"count", "sum", "avg", "min", "max"},
			},
			"field": stringProp("Required for sum/avg/min/max. For count, use empty string for count(*)."),
		}, "fn"),
		"sort": objectSchema(map[string]any{
			"field": stringProp("Field name, or 'label'/'value' for aggregated queries."),
			"dir":   map[string]any{"type": "string", "enum": []string{"asc", "desc"}},
		}, "field", "dir"),
		"limit": map[string]any{"type": "integer", "description": "Max rows; default 500 for series, 100 for tables."},
		"date_filter_field": stringProp(
			"Optional: date/timestamp field that should respond to the dashboard's date-range selector. " +
				"Set this on reports whose numbers should change when the user picks a different time window. " +
				"Leave empty on all-time KPIs.",
		),
		"compare_period": map[string]any{
			"type":        "string",
			"enum":        []string{"", "previous_period", "previous_year"},
			"description": "KPI only. When a dashboard range is set, also queries the prior window and renders a delta.",
		},
	}
}

func fieldSchemaProperties() map[string]any {
	return map[string]any{
		"name":         stringProp("Machine identifier."),
		"display_name": stringProp("Human-facing label."),
		"data_type": map[string]any{
			"type": "string",
			"enum": []string{
				"text", "integer", "bigint", "numeric", "boolean",
				"date", "timestamptz", "uuid", "jsonb", "reference",
			},
		},
		"is_required":      map[string]any{"type": "boolean"},
		"is_unique":        map[string]any{"type": "boolean"},
		"reference_entity": stringProp("If data_type is reference, the target entity name."),
	}
}
