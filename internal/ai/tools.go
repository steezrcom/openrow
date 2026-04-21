package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/sashabaranov/go-openai"

	"github.com/openrow/openrow/internal/entities"
	"github.com/openrow/openrow/internal/reports"
	"github.com/openrow/openrow/internal/templates"
)

// tool describes one action the agent can invoke.
type tool struct {
	name        string
	description string
	schema      map[string]any // full JSON Schema "object" — {type, properties, required}
	handler     func(ctx context.Context, input json.RawMessage) execResult
}

type execResult struct {
	Summary    string
	EntityName string
	Result     any
	Err        error
}

func (e execResult) ErrMsg() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e execResult) ResultText() string {
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

// toolset bundles tools for one request (closed over tenantID/pgSchema).
type toolset struct {
	tools []tool
	index map[string]tool
}

func (ts toolset) toolParams() []openai.Tool {
	out := make([]openai.Tool, len(ts.tools))
	for i, t := range ts.tools {
		out[i] = openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.name,
				Description: t.description,
				Parameters:  t.schema,
			},
		}
	}
	return out
}

func (ts toolset) run(ctx context.Context, name string, input json.RawMessage) execResult {
	t, ok := ts.index[name]
	if !ok {
		return execResult{Err: fmt.Errorf("unknown tool %q", name)}
	}
	return t.handler(ctx, input)
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

func (a *Agent) buildTools(_ context.Context, tenantID, pgSchema string) toolset {
	svc := a.entities
	dash := a.dashboards

	ts := toolset{index: map[string]tool{}}
	add := func(t tool) {
		ts.tools = append(ts.tools, t)
		ts.index[t.name] = t
	}

	add(tool{
		name:        "list_entities",
		description: "Return all entities in the current organization with their fields. Call this whenever you're unsure which entities exist or need to verify a name.",
		schema:      objectSchema(map[string]any{}),
		handler: func(ctx context.Context, _ json.RawMessage) execResult {
			es, err := svc.List(ctx, tenantID)
			if err != nil {
				return execResult{Err: err}
			}
			out := make([]entities.Entity, len(es))
			for i, e := range es {
				detailed, err := svc.Get(ctx, tenantID, e.Name)
				if err != nil {
					return execResult{Err: err}
				}
				out[i] = *detailed
			}
			return execResult{Summary: fmt.Sprintf("Listed %d entities", len(out)), Result: out}
		},
	})

	add(tool{
		name:        "create_entity",
		description: "Create a new entity (database table). Validates field names; id/created_at/updated_at are added automatically.",
		schema:      objectSchema(entitySchemaProperties(), "name", "display_name", "fields"),
		handler: func(ctx context.Context, input json.RawMessage) execResult {
			var spec entities.EntitySpec
			if err := json.Unmarshal(input, &spec); err != nil {
				return execResult{Err: fmt.Errorf("invalid input: %w", err)}
			}
			ent, err := svc.Create(ctx, tenantID, pgSchema, &spec)
			if err != nil {
				return execResult{Err: err}
			}
			return execResult{
				Summary:    fmt.Sprintf("Created entity %q with %d fields", ent.Name, len(ent.Fields)),
				EntityName: ent.Name,
				Result:     ent,
			}
		},
	})

	add(tool{
		name:        "add_row",
		description: "Insert a new row into an entity's table. values is a string→string map; the server coerces by field type.",
		schema: objectSchema(map[string]any{
			"entity": stringProp("Entity name (lowercase identifier)."),
			"values": map[string]any{
				"type":                 "object",
				"description":          "Field name to value, as strings. For boolean, use \"true\"/\"false\".",
				"additionalProperties": map[string]any{"type": "string"},
			},
		}, "entity", "values"),
		handler: func(ctx context.Context, input json.RawMessage) execResult {
			var req struct {
				Entity string            `json:"entity"`
				Values map[string]string `json:"values"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return execResult{Err: err}
			}
			ent, err := svc.Get(ctx, tenantID, req.Entity)
			if err != nil {
				return execResult{Err: fmt.Errorf("entity %q not found", req.Entity)}
			}
			id, err := svc.InsertRow(ctx, pgSchema, ent, req.Values)
			if err != nil {
				return execResult{Err: err}
			}
			return execResult{
				Summary:    fmt.Sprintf("Added row to %q", ent.Name),
				EntityName: ent.Name,
				Result:     map[string]string{"id": id},
			}
		},
	})

	add(tool{
		name:        "update_row",
		description: "Update an existing row. Only include the fields you want to change. To clear a non-required field, pass an empty string.",
		schema: objectSchema(map[string]any{
			"entity": stringProp("Entity name."),
			"id":     stringProp("Row UUID."),
			"values": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
			},
		}, "entity", "id", "values"),
		handler: func(ctx context.Context, input json.RawMessage) execResult {
			var req struct {
				Entity string            `json:"entity"`
				ID     string            `json:"id"`
				Values map[string]string `json:"values"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return execResult{Err: err}
			}
			ent, err := svc.Get(ctx, tenantID, req.Entity)
			if err != nil {
				return execResult{Err: fmt.Errorf("entity %q not found", req.Entity)}
			}
			if err := svc.UpdateRow(ctx, pgSchema, ent, req.ID, req.Values); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return execResult{Err: fmt.Errorf("row %s not found", req.ID)}
				}
				return execResult{Err: err}
			}
			return execResult{
				Summary:    fmt.Sprintf("Updated row in %q", ent.Name),
				EntityName: ent.Name,
				Result:     map[string]string{"id": req.ID},
			}
		},
	})

	add(tool{
		name:        "delete_row",
		description: "Delete a row by id.",
		schema: objectSchema(map[string]any{
			"entity": stringProp("Entity name."),
			"id":     stringProp("Row UUID."),
		}, "entity", "id"),
		handler: func(ctx context.Context, input json.RawMessage) execResult {
			var req struct {
				Entity string `json:"entity"`
				ID     string `json:"id"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return execResult{Err: err}
			}
			ent, err := svc.Get(ctx, tenantID, req.Entity)
			if err != nil {
				return execResult{Err: fmt.Errorf("entity %q not found", req.Entity)}
			}
			if err := svc.DeleteRow(ctx, pgSchema, ent, req.ID); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return execResult{Err: fmt.Errorf("row %s not found", req.ID)}
				}
				return execResult{Err: err}
			}
			return execResult{
				Summary:    fmt.Sprintf("Deleted row from %q", ent.Name),
				EntityName: ent.Name,
			}
		},
	})

	add(tool{
		name:        "add_field",
		description: "Add a new column to an existing entity. Use when the user asks to extend a table with a new attribute.",
		schema: objectSchema(map[string]any{
			"entity": stringProp("Target entity name."),
			"field":  objectSchema(fieldSchemaProperties(), "name", "display_name", "data_type"),
		}, "entity", "field"),
		handler: func(ctx context.Context, input json.RawMessage) execResult {
			var req struct {
				Entity string             `json:"entity"`
				Field  entities.FieldSpec `json:"field"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return execResult{Err: err}
			}
			ent, err := svc.AddField(ctx, tenantID, pgSchema, req.Entity, req.Field)
			if err != nil {
				return execResult{Err: err}
			}
			return execResult{
				Summary:    fmt.Sprintf("Added field %q to %q", req.Field.Name, req.Entity),
				EntityName: req.Entity,
				Result:     ent,
			}
		},
	})

	add(tool{
		name:        "drop_field",
		description: "Remove a column from an entity. Data in that column is lost. Only call after confirming with the user or when explicitly requested.",
		schema: objectSchema(map[string]any{
			"entity": stringProp("Entity name."),
			"field":  stringProp("Field name to drop."),
		}, "entity", "field"),
		handler: func(ctx context.Context, input json.RawMessage) execResult {
			var req struct {
				Entity string `json:"entity"`
				Field  string `json:"field"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return execResult{Err: err}
			}
			if err := svc.DropField(ctx, tenantID, pgSchema, req.Entity, req.Field); err != nil {
				return execResult{Err: err}
			}
			return execResult{
				Summary:    fmt.Sprintf("Dropped field %q from %q", req.Field, req.Entity),
				EntityName: req.Entity,
			}
		},
	})

	add(tool{
		name:        "list_templates",
		description: "List pre-built workspace templates (entities + starter dashboards).",
		schema:      objectSchema(map[string]any{}),
		handler: func(_ context.Context, _ json.RawMessage) execResult {
			ts := templates.All()
			out := make([]map[string]string, len(ts))
			for i, t := range ts {
				out[i] = map[string]string{"id": t.ID, "name": t.Name, "description": t.Description}
			}
			return execResult{Summary: fmt.Sprintf("Listed %d templates", len(out)), Result: out}
		},
	})

	add(tool{
		name: "apply_template",
		description: "Install a pre-built template (entities + default dashboard) in the current workspace. " +
			"Fails if any template entity name conflicts with an existing entity, so prefer fresh workspaces.",
		schema: objectSchema(map[string]any{
			"id": stringProp("Template id; currently 'agency' is the supported value."),
		}, "id"),
		handler: func(ctx context.Context, input json.RawMessage) execResult {
			var req struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return execResult{Err: err}
			}
			t, ok := templates.Get(req.ID)
			if !ok {
				return execResult{Err: fmt.Errorf("template %q not found", req.ID)}
			}
			if err := t.Install(ctx, tenantID, pgSchema, svc, dash); err != nil {
				return execResult{Err: err}
			}
			return execResult{Summary: fmt.Sprintf("Installed template %q", req.ID)}
		},
	})

	add(tool{
		name:        "list_dashboards",
		description: "List dashboards in the current organization (with report titles).",
		schema:      objectSchema(map[string]any{}),
		handler: func(ctx context.Context, _ json.RawMessage) execResult {
			ds, err := dash.List(ctx, tenantID)
			if err != nil {
				return execResult{Err: err}
			}
			return execResult{
				Summary: fmt.Sprintf("Listed %d dashboards", len(ds)),
				Result:  ds,
			}
		},
	})

	add(tool{
		name:        "get_dashboard",
		description: "Get one dashboard with its reports.",
		schema:      objectSchema(map[string]any{"slug": stringProp("Dashboard slug.")}, "slug"),
		handler: func(ctx context.Context, input json.RawMessage) execResult {
			var req struct {
				Slug string `json:"slug"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return execResult{Err: err}
			}
			d, err := dash.Get(ctx, tenantID, req.Slug)
			if err != nil {
				return execResult{Err: err}
			}
			return execResult{Summary: fmt.Sprintf("Fetched dashboard %q", d.Slug), Result: d}
		},
	})

	add(tool{
		name:        "create_dashboard",
		description: "Create a new dashboard. Optionally include an initial list of reports (widgets). Every report has a query_spec; see the system prompt for the schema.",
		schema: objectSchema(map[string]any{
			"name":        stringProp("Human-facing title, e.g. 'Financial overview'."),
			"slug":        stringProp("Optional machine slug; auto-generated from name if omitted."),
			"description": stringProp("Optional one-line description."),
			"reports": map[string]any{
				"type":        "array",
				"description": "Initial reports to include on this dashboard.",
				"items":       objectSchema(reportSchemaProperties(), "title", "widget_type", "query_spec"),
			},
		}, "name"),
		handler: func(ctx context.Context, input json.RawMessage) execResult {
			var in reports.CreateDashboardInput
			if err := json.Unmarshal(input, &in); err != nil {
				return execResult{Err: err}
			}
			d, err := dash.Create(ctx, tenantID, in)
			if err != nil {
				return execResult{Err: err}
			}
			return execResult{
				Summary: fmt.Sprintf("Created dashboard %q with %d reports", d.Slug, len(d.Reports)),
				Result:  d,
			}
		},
	})

	add(tool{
		name:        "add_report",
		description: "Add a report to an existing dashboard.",
		schema: objectSchema(map[string]any{
			"slug":   stringProp("Dashboard slug."),
			"report": objectSchema(reportSchemaProperties(), "title", "widget_type", "query_spec"),
		}, "slug", "report"),
		handler: func(ctx context.Context, input json.RawMessage) execResult {
			var req struct {
				Slug   string                    `json:"slug"`
				Report reports.CreateReportInput `json:"report"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return execResult{Err: err}
			}
			r, err := dash.AddReport(ctx, tenantID, req.Slug, req.Report)
			if err != nil {
				return execResult{Err: err}
			}
			return execResult{Summary: fmt.Sprintf("Added report %q to %q", r.Title, req.Slug), Result: r}
		},
	})

	add(tool{
		name:        "update_report",
		description: "Edit an existing report's title, subtitle, widget type, query spec, or width.",
		schema: objectSchema(map[string]any{
			"id":          stringProp("Report id."),
			"title":       stringProp("New title."),
			"subtitle":    stringProp("New subtitle."),
			"widget_type": stringProp("kpi | bar | line | area | pie | table."),
			"width":       map[string]any{"type": "integer", "description": "New width (1-12)."},
			"query_spec":  map[string]any{"type": "object", "description": "Replacement query_spec."},
		}, "id"),
		handler: func(ctx context.Context, input json.RawMessage) execResult {
			var req struct {
				ID         string              `json:"id"`
				Title      *string             `json:"title,omitempty"`
				Subtitle   *string             `json:"subtitle,omitempty"`
				WidgetType *reports.WidgetType `json:"widget_type,omitempty"`
				QuerySpec  *reports.QuerySpec  `json:"query_spec,omitempty"`
				Width      *int                `json:"width,omitempty"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return execResult{Err: err}
			}
			if err := dash.UpdateReport(ctx, tenantID, req.ID, reports.UpdateReportInput{
				Title: req.Title, Subtitle: req.Subtitle,
				WidgetType: req.WidgetType, QuerySpec: req.QuerySpec, Width: req.Width,
			}); err != nil {
				return execResult{Err: err}
			}
			return execResult{Summary: "Updated report", Result: map[string]string{"id": req.ID}}
		},
	})

	add(tool{
		name:        "delete_report",
		description: "Delete a report by id.",
		schema:      objectSchema(map[string]any{"id": stringProp("Report id.")}, "id"),
		handler: func(ctx context.Context, input json.RawMessage) execResult {
			var req struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return execResult{Err: err}
			}
			if err := dash.DeleteReport(ctx, tenantID, req.ID); err != nil {
				return execResult{Err: err}
			}
			return execResult{Summary: "Deleted report"}
		},
	})

	add(tool{
		name:        "delete_dashboard",
		description: "Delete a dashboard and all its reports. Destructive; confirm with the user first.",
		schema:      objectSchema(map[string]any{"slug": stringProp("Dashboard slug.")}, "slug"),
		handler: func(ctx context.Context, input json.RawMessage) execResult {
			var req struct {
				Slug string `json:"slug"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return execResult{Err: err}
			}
			if err := dash.Delete(ctx, tenantID, req.Slug); err != nil {
				return execResult{Err: err}
			}
			return execResult{Summary: fmt.Sprintf("Deleted dashboard %q", req.Slug)}
		},
	})

	add(tool{
		name:        "query_rows",
		description: "Return recent rows from an entity. Use this to answer questions about current data or verify state before mutating.",
		schema: objectSchema(map[string]any{
			"entity": stringProp("Entity name."),
			"limit": map[string]any{
				"type":        "integer",
				"description": "Max rows. Default 20, max 200.",
			},
		}, "entity"),
		handler: func(ctx context.Context, input json.RawMessage) execResult {
			var req struct {
				Entity string `json:"entity"`
				Limit  int    `json:"limit"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return execResult{Err: err}
			}
			if req.Limit <= 0 || req.Limit > 200 {
				req.Limit = 20
			}
			ent, err := svc.Get(ctx, tenantID, req.Entity)
			if err != nil {
				return execResult{Err: fmt.Errorf("entity %q not found", req.Entity)}
			}
			rows, err := svc.ListRows(ctx, pgSchema, ent, entities.ListOptions{Limit: req.Limit})
			if err != nil {
				return execResult{Err: err}
			}
			return execResult{
				Summary:    fmt.Sprintf("Fetched %d rows from %q", len(rows), ent.Name),
				EntityName: ent.Name,
				Result:     map[string]any{"count": len(rows), "rows": rows},
			}
		},
	})

	return ts
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
