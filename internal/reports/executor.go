package reports

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steezrcom/steezr-erp/internal/entities"
)

// Result is the normalized output of a QuerySpec.
//
// Shapes:
//   - KPI (aggregate, no group_by):        [{"value": N}]
//   - Chart (aggregate + group_by):        [{"label": ..., "value": N}, ...]
//   - Table (no aggregate, no group_by):   [{... entity fields}]
type Result struct {
	Shape   string                   `json:"shape"` // "kpi" | "series" | "table"
	Columns []string                 `json:"columns,omitempty"`
	Rows    []map[string]interface{} `json:"rows"`
}

type Executor struct {
	pool *pgxpool.Pool
}

func NewExecutor(pool *pgxpool.Pool) *Executor {
	return &Executor{pool: pool}
}

// Execute runs a spec against the tenant schema. Entity is the validated
// metadata (so field names/types are known).
func (e *Executor) Execute(ctx context.Context, schema string, ent *entities.Entity, spec *QuerySpec) (*Result, error) {
	if !identRe.MatchString(schema) {
		return nil, fmt.Errorf("invalid schema")
	}
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	fields := indexFields(ent)

	// Build where clause first so filter params come before aggregate/group params (there are none for those).
	var params []interface{}
	where, err := buildWhere(fields, spec.Filters, &params)
	if err != nil {
		return nil, err
	}

	table := pgx.Identifier{schema, ent.Name}.Sanitize()

	switch {
	case spec.Aggregate != nil && spec.GroupBy != nil:
		return e.runSeries(ctx, table, fields, spec, where, params)
	case spec.Aggregate != nil:
		return e.runKPI(ctx, table, fields, spec, where, params)
	default:
		return e.runTable(ctx, table, ent, fields, spec, where, params)
	}
}

func (e *Executor) runKPI(ctx context.Context, table string, fields map[string]entities.Field, spec *QuerySpec, where string, params []interface{}) (*Result, error) {
	aggExpr, err := aggregateExpr(fields, spec.Aggregate)
	if err != nil {
		return nil, err
	}
	q := fmt.Sprintf("SELECT %s AS value FROM %s%s", aggExpr, table, where)
	row := e.pool.QueryRow(ctx, q, params...)
	var value interface{}
	if err := row.Scan(&value); err != nil {
		return nil, err
	}
	return &Result{
		Shape:   "kpi",
		Columns: []string{"value"},
		Rows:    []map[string]interface{}{{"value": normalize(value)}},
	}, nil
}

func (e *Executor) runSeries(ctx context.Context, table string, fields map[string]entities.Field, spec *QuerySpec, where string, params []interface{}) (*Result, error) {
	groupExpr, groupType, err := groupByExpr(fields, spec.GroupBy)
	if err != nil {
		return nil, err
	}
	aggExpr, err := aggregateExpr(fields, spec.Aggregate)
	if err != nil {
		return nil, err
	}

	limit := spec.Limit
	if limit <= 0 || limit > 1000 {
		limit = 500
	}

	orderBy := "label ASC"
	if spec.Sort != nil {
		col := "label"
		if spec.Sort.Field == "value" {
			col = "value"
		}
		orderBy = col + " " + strings.ToUpper(spec.Sort.Dir)
	}

	q := fmt.Sprintf(
		"SELECT %s AS label, %s AS value FROM %s%s GROUP BY label ORDER BY %s LIMIT %d",
		groupExpr, aggExpr, table, where, orderBy, limit,
	)

	rows, err := e.pool.Query(ctx, q, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]interface{}, 0)
	for rows.Next() {
		var label, value interface{}
		if err := rows.Scan(&label, &value); err != nil {
			return nil, err
		}
		out = append(out, map[string]interface{}{
			"label": normalize(label),
			"value": normalize(value),
		})
	}
	_ = groupType
	return &Result{Shape: "series", Columns: []string{"label", "value"}, Rows: out}, rows.Err()
}

func (e *Executor) runTable(ctx context.Context, table string, ent *entities.Entity, fields map[string]entities.Field, spec *QuerySpec, where string, params []interface{}) (*Result, error) {
	cols := []string{"id"}
	for _, f := range ent.Fields {
		cols = append(cols, f.Name)
	}
	cols = append(cols, "created_at", "updated_at")

	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = pgx.Identifier{c}.Sanitize()
	}

	limit := spec.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	orderBy := "created_at DESC"
	if spec.Sort != nil {
		if _, ok := fields[spec.Sort.Field]; ok || spec.Sort.Field == "id" || spec.Sort.Field == "created_at" || spec.Sort.Field == "updated_at" {
			orderBy = pgx.Identifier{spec.Sort.Field}.Sanitize() + " " + strings.ToUpper(spec.Sort.Dir)
		}
	}

	q := fmt.Sprintf("SELECT %s FROM %s%s ORDER BY %s LIMIT %d",
		strings.Join(quoted, ", "), table, where, orderBy, limit,
	)
	rows, err := e.pool.Query(ctx, q, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]interface{}, 0)
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make(map[string]interface{}, len(cols))
		for i, c := range cols {
			row[c] = normalize(vals[i])
		}
		out = append(out, row)
	}
	return &Result{Shape: "table", Columns: cols, Rows: out}, rows.Err()
}

func indexFields(ent *entities.Entity) map[string]entities.Field {
	out := map[string]entities.Field{}
	for _, f := range ent.Fields {
		out[f.Name] = f
	}
	return out
}

func resolveField(fields map[string]entities.Field, name string) (entities.DataType, bool) {
	switch name {
	case "id":
		return entities.TypeUUID, true
	case "created_at", "updated_at":
		return entities.TypeTimestampTZ, true
	}
	f, ok := fields[name]
	if !ok {
		return "", false
	}
	return f.DataType, true
}

func buildWhere(fields map[string]entities.Field, filters []Filter, params *[]interface{}) (string, error) {
	if len(filters) == 0 {
		return "", nil
	}
	var parts []string
	for i, f := range filters {
		dt, ok := resolveField(fields, f.Field)
		if !ok {
			return "", fmt.Errorf("filters[%d]: unknown field %q", i, f.Field)
		}
		col := pgx.Identifier{f.Field}.Sanitize()

		switch f.Op {
		case OpIsNull:
			parts = append(parts, col+" IS NULL")
			continue
		case OpIsNotNull:
			parts = append(parts, col+" IS NOT NULL")
			continue
		}

		if len(f.Value) == 0 {
			return "", fmt.Errorf("filters[%d]: value required for op %s", i, f.Op)
		}

		if f.Op == OpIn {
			var raw []json.RawMessage
			if err := json.Unmarshal(f.Value, &raw); err != nil {
				return "", fmt.Errorf("filters[%d]: value must be an array for in", i)
			}
			if len(raw) == 0 {
				return "", fmt.Errorf("filters[%d]: in expects non-empty array", i)
			}
			placeholders := make([]string, 0, len(raw))
			for _, item := range raw {
				v, err := coerceScalar(dt, item)
				if err != nil {
					return "", fmt.Errorf("filters[%d]: %w", i, err)
				}
				*params = append(*params, v)
				placeholders = append(placeholders, fmt.Sprintf("$%d", len(*params)))
			}
			parts = append(parts, fmt.Sprintf("%s IN (%s)", col, strings.Join(placeholders, ", ")))
			continue
		}

		v, err := coerceScalar(dt, f.Value)
		if err != nil {
			return "", fmt.Errorf("filters[%d]: %w", i, err)
		}

		if f.Op == OpContains {
			if dt != entities.TypeText {
				return "", fmt.Errorf("filters[%d]: contains only works on text fields", i)
			}
			*params = append(*params, v)
			parts = append(parts, fmt.Sprintf("%s ILIKE '%%' || $%d || '%%'", col, len(*params)))
			continue
		}

		op, err := sqlOp(f.Op)
		if err != nil {
			return "", fmt.Errorf("filters[%d]: %w", i, err)
		}
		*params = append(*params, v)
		parts = append(parts, fmt.Sprintf("%s %s $%d", col, op, len(*params)))
	}
	return " WHERE " + strings.Join(parts, " AND "), nil
}

func sqlOp(op FilterOp) (string, error) {
	switch op {
	case OpEq:
		return "=", nil
	case OpNe:
		return "<>", nil
	case OpGt:
		return ">", nil
	case OpGte:
		return ">=", nil
	case OpLt:
		return "<", nil
	case OpLte:
		return "<=", nil
	}
	return "", fmt.Errorf("unsupported op %s", op)
}

func coerceScalar(dt entities.DataType, raw json.RawMessage) (interface{}, error) {
	var out interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	// Most types can be handed to pgx as-is (string, number, bool).
	// UUIDs must be strings; timestamps must be strings Postgres can parse.
	_ = dt
	return out, nil
}

func aggregateExpr(fields map[string]entities.Field, a *Aggregate) (string, error) {
	switch a.Fn {
	case AggCount:
		if a.Field == "" {
			return "COUNT(*)", nil
		}
		if _, ok := resolveField(fields, a.Field); !ok {
			return "", fmt.Errorf("aggregate: unknown field %q", a.Field)
		}
		return "COUNT(" + pgx.Identifier{a.Field}.Sanitize() + ")", nil
	case AggSum, AggAvg, AggMin, AggMax:
		dt, ok := resolveField(fields, a.Field)
		if !ok {
			return "", fmt.Errorf("aggregate: unknown field %q", a.Field)
		}
		if a.Fn != AggMin && a.Fn != AggMax && !isNumericType(dt) {
			return "", fmt.Errorf("aggregate: %s requires a numeric field", a.Fn)
		}
		return strings.ToUpper(string(a.Fn)) + "(" + pgx.Identifier{a.Field}.Sanitize() + ")", nil
	}
	return "", fmt.Errorf("unsupported aggregate %s", a.Fn)
}

func groupByExpr(fields map[string]entities.Field, g *GroupBy) (string, entities.DataType, error) {
	dt, ok := resolveField(fields, g.Field)
	if !ok {
		return "", "", fmt.Errorf("group_by: unknown field %q", g.Field)
	}
	col := pgx.Identifier{g.Field}.Sanitize()
	if g.Bucket == BucketNone {
		return col, dt, nil
	}
	if dt != entities.TypeDate && dt != entities.TypeTimestampTZ {
		return "", "", fmt.Errorf("group_by: bucket only works on date/timestamp fields")
	}
	return fmt.Sprintf("date_trunc(%s, %s)", quoteLiteral(string(g.Bucket)), col), dt, nil
}

func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func isNumericType(t entities.DataType) bool {
	return t == entities.TypeInteger || t == entities.TypeBigInt || t == entities.TypeNumeric
}

func normalize(v interface{}) interface{} {
	switch x := v.(type) {
	case [16]byte:
		return uuid.UUID(x).String()
	case []byte:
		return string(x)
	}
	return v
}
