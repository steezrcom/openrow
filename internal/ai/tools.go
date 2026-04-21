package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/jackc/pgx/v5"

	"github.com/steezrcom/steezr-erp/internal/entities"
)

// tool describes one action the agent can invoke.
type tool struct {
	name        string
	description string
	schema      anthropic.ToolInputSchemaParam
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

// toolset bundles the tools for one request (closed over tenantID/pgSchema).
type toolset struct {
	tools []tool
	index map[string]tool
}

func (ts toolset) toolParams() []anthropic.ToolUnionParam {
	out := make([]anthropic.ToolUnionParam, len(ts.tools))
	for i, t := range ts.tools {
		p := anthropic.ToolParam{
			Name:        t.name,
			Description: anthropic.String(t.description),
			InputSchema: t.schema,
		}
		out[i] = anthropic.ToolUnionParam{OfTool: &p}
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

func (a *Agent) buildTools(_ context.Context, tenantID, pgSchema string) toolset {
	svc := a.entities

	ts := toolset{index: map[string]tool{}}
	add := func(t tool) {
		ts.tools = append(ts.tools, t)
		ts.index[t.name] = t
	}

	add(tool{
		name:        "list_entities",
		description: "Return all entities in the current organization with their fields. Call this whenever you're unsure which entities exist or need to verify a name.",
		schema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{},
		},
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
		schema: anthropic.ToolInputSchemaParam{
			Properties: entitySchemaProperties(),
			Required:   []string{"name", "display_name", "fields"},
		},
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
		schema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"entity": stringProp("Entity name (lowercase identifier)."),
				"values": map[string]any{
					"type":                 "object",
					"description":          "Field name to value, as strings. For boolean, use \"true\"/\"false\".",
					"additionalProperties": map[string]any{"type": "string"},
				},
			},
			Required: []string{"entity", "values"},
		},
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
		schema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"entity": stringProp("Entity name."),
				"id":     stringProp("Row UUID."),
				"values": map[string]any{
					"type":                 "object",
					"additionalProperties": map[string]any{"type": "string"},
				},
			},
			Required: []string{"entity", "id", "values"},
		},
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
		schema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"entity": stringProp("Entity name."),
				"id":     stringProp("Row UUID."),
			},
			Required: []string{"entity", "id"},
		},
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
		schema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"entity": stringProp("Target entity name."),
				"field": map[string]any{
					"type":       "object",
					"properties": fieldSchemaProperties(),
					"required":   []string{"name", "display_name", "data_type"},
				},
			},
			Required: []string{"entity", "field"},
		},
		handler: func(ctx context.Context, input json.RawMessage) execResult {
			var req struct {
				Entity string               `json:"entity"`
				Field  entities.FieldSpec   `json:"field"`
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
		schema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"entity": stringProp("Entity name."),
				"field":  stringProp("Field name to drop."),
			},
			Required: []string{"entity", "field"},
		},
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
		name:        "query_rows",
		description: "Return recent rows from an entity. Use this to answer questions about current data or verify state before mutating.",
		schema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"entity": stringProp("Entity name."),
				"limit": map[string]any{
					"type":        "integer",
					"description": "Max rows. Default 20, max 200.",
				},
			},
			Required: []string{"entity"},
		},
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
			"items": map[string]any{
				"type":       "object",
				"properties": fieldSchemaProperties(),
				"required":   []string{"name", "display_name", "data_type"},
			},
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

