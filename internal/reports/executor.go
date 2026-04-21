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
//   - Table (no aggregate, no group_by):   [{... entity fields, ref__label pairs}]
type Result struct {
	Shape   string                   `json:"shape"` // "kpi" | "series" | "table"
	Columns []string                 `json:"columns,omitempty"`
	Rows    []map[string]interface{} `json:"rows"`
}

type Executor struct {
	pool *pgxpool.Pool
	ents *entities.Service
}

func NewExecutor(pool *pgxpool.Pool, ents *entities.Service) *Executor {
	return &Executor{pool: pool, ents: ents}
}

const sourceAlias = "src"

// Execute runs a spec against the tenant schema. entity is the validated
// metadata; tenantID lets us resolve referenced entities for label JOINs.
func (e *Executor) Execute(ctx context.Context, schema, tenantID string, ent *entities.Entity, spec *QuerySpec) (*Result, error) {
	if !identRe.MatchString(schema) {
		return nil, fmt.Errorf("invalid schema")
	}
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	fields := indexFields(ent)

	b := &builder{
		schema:    schema,
		tenantID:  tenantID,
		ent:       ent,
		fields:    fields,
		ents:      e.ents,
		tableExpr: fmt.Sprintf("%s AS %s", pgx.Identifier{schema, ent.Name}.Sanitize(), sourceAlias),
	}

	where, err := b.buildWhere(spec.Filters)
	if err != nil {
		return nil, err
	}

	switch {
	case spec.Aggregate != nil && spec.GroupBy != nil:
		return e.runSeries(ctx, b, spec, where)
	case spec.Aggregate != nil:
		return e.runKPI(ctx, b, spec, where)
	default:
		return e.runTable(ctx, b, spec, where)
	}
}

// builder accumulates the FROM clause (including JOINs) and parameters.
type builder struct {
	schema    string
	tenantID  string
	ent       *entities.Entity
	fields    map[string]entities.Field
	ents      *entities.Service
	tableExpr string

	params []interface{}
	joins  []joinInfo

	// addedJoins keys by source field name so we don't double-join.
	addedJoins map[string]bool
}

type joinInfo struct {
	alias       string
	targetTable string
	sourceCol   string
	labelExpr   string // unqualified label expression that works once alias is joined, or "" if target has no good label column
	labelCol    string // column on target table, empty if no label
}

func (b *builder) nextParam(v interface{}) string {
	b.params = append(b.params, v)
	return fmt.Sprintf("$%d", len(b.params))
}

// addRefJoin joins the target entity for a reference field and returns the
// join info (idempotent per field name).
func (b *builder) addRefJoin(ctx context.Context, field entities.Field) (joinInfo, error) {
	if b.addedJoins == nil {
		b.addedJoins = map[string]bool{}
	}
	alias := "j_" + field.Name
	if b.addedJoins[field.Name] {
		// find existing
		for _, j := range b.joins {
			if j.alias == alias {
				return j, nil
			}
		}
	}
	target, err := b.ents.Get(ctx, b.tenantID, field.ReferenceEntity)
	if err != nil {
		return joinInfo{}, fmt.Errorf("resolve reference %q: %w", field.ReferenceEntity, err)
	}
	labelCol := pickRefLabel(target)
	srcCol := fmt.Sprintf("%s.%s", sourceAlias, pgx.Identifier{field.Name}.Sanitize())
	info := joinInfo{
		alias:       alias,
		targetTable: pgx.Identifier{b.schema, target.Name}.Sanitize(),
		sourceCol:   srcCol,
		labelCol:    labelCol,
	}
	if labelCol != "" {
		info.labelExpr = fmt.Sprintf(
			"COALESCE(%s.%s, %s::text)",
			pgx.Identifier{alias}.Sanitize(),
			pgx.Identifier{labelCol}.Sanitize(),
			srcCol,
		)
	} else {
		info.labelExpr = srcCol + "::text"
	}
	b.joins = append(b.joins, info)
	b.addedJoins[field.Name] = true
	return info, nil
}

func (b *builder) fromClause() string {
	if len(b.joins) == 0 {
		return b.tableExpr
	}
	var parts []string
	parts = append(parts, b.tableExpr)
	for _, j := range b.joins {
		parts = append(parts,
			fmt.Sprintf("LEFT JOIN %s %s ON %s.id = %s",
				j.targetTable,
				pgx.Identifier{j.alias}.Sanitize(),
				pgx.Identifier{j.alias}.Sanitize(),
				j.sourceCol,
			))
	}
	return strings.Join(parts, " ")
}

func (b *builder) buildWhere(filters []Filter) (string, error) {
	if len(filters) == 0 {
		return "", nil
	}
	var parts []string
	for i, f := range filters {
		dt, ok := resolveField(b.fields, f.Field)
		if !ok {
			return "", fmt.Errorf("filters[%d]: unknown field %q", i, f.Field)
		}
		col := sourceAlias + "." + pgx.Identifier{f.Field}.Sanitize()

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
				placeholders = append(placeholders, b.nextParam(v))
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
			parts = append(parts, fmt.Sprintf("%s ILIKE '%%' || %s || '%%'", col, b.nextParam(v)))
			continue
		}
		op, err := sqlOp(f.Op)
		if err != nil {
			return "", fmt.Errorf("filters[%d]: %w", i, err)
		}
		parts = append(parts, fmt.Sprintf("%s %s %s", col, op, b.nextParam(v)))
	}
	return " WHERE " + strings.Join(parts, " AND "), nil
}

func (e *Executor) runKPI(ctx context.Context, b *builder, spec *QuerySpec, where string) (*Result, error) {
	aggExpr, err := aggregateExpr(b.fields, spec.Aggregate)
	if err != nil {
		return nil, err
	}
	q := fmt.Sprintf("SELECT %s AS value FROM %s%s", aggExpr, b.fromClause(), where)
	row := e.pool.QueryRow(ctx, q, b.params...)
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

func (e *Executor) runSeries(ctx context.Context, b *builder, spec *QuerySpec, where string) (*Result, error) {
	labelExpr, labelGroupExpr, err := b.groupByExpr(ctx, spec.GroupBy)
	if err != nil {
		return nil, err
	}
	aggExpr, err := aggregateExpr(b.fields, spec.Aggregate)
	if err != nil {
		return nil, err
	}

	var seriesExpr, seriesGroupExpr string
	hasSeries := spec.SeriesBy != nil
	if hasSeries {
		seriesExpr, seriesGroupExpr, err = b.groupByExpr(ctx, spec.SeriesBy)
		if err != nil {
			return nil, err
		}
	}

	limit := spec.Limit
	if limit <= 0 || limit > 2000 {
		limit = 1000
	}

	orderBy := "label ASC"
	if hasSeries {
		orderBy = "label ASC, series ASC"
	}
	if spec.Sort != nil {
		col := "label"
		if spec.Sort.Field == "value" {
			col = "value"
		} else if spec.Sort.Field == "series" {
			col = "series"
		}
		orderBy = col + " " + strings.ToUpper(spec.Sort.Dir)
	}

	// GROUP BY the expressions that identify each bucket. For ref labels the
	// label expression is a COALESCE on the join column, so we need both the FK
	// and the label in GROUP BY; same for series.
	groupByParts := []string{labelGroupExpr}
	if labelExpr != labelGroupExpr {
		groupByParts = append(groupByParts, labelExpr)
	}
	if hasSeries {
		groupByParts = append(groupByParts, seriesGroupExpr)
		if seriesExpr != seriesGroupExpr {
			groupByParts = append(groupByParts, seriesExpr)
		}
	}

	selectParts := []string{labelExpr + " AS label"}
	if hasSeries {
		selectParts = append(selectParts, seriesExpr+" AS series")
	}
	selectParts = append(selectParts, aggExpr+" AS value")

	q := fmt.Sprintf(
		"SELECT %s FROM %s%s GROUP BY %s ORDER BY %s LIMIT %d",
		strings.Join(selectParts, ", "),
		b.fromClause(), where,
		strings.Join(groupByParts, ", "),
		orderBy, limit,
	)
	rows, err := e.pool.Query(ctx, q, b.params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]interface{}, 0)
	for rows.Next() {
		if hasSeries {
			var label, series, value interface{}
			if err := rows.Scan(&label, &series, &value); err != nil {
				return nil, err
			}
			out = append(out, map[string]interface{}{
				"label":  normalize(label),
				"series": normalize(series),
				"value":  normalize(value),
			})
		} else {
			var label, value interface{}
			if err := rows.Scan(&label, &value); err != nil {
				return nil, err
			}
			out = append(out, map[string]interface{}{
				"label": normalize(label),
				"value": normalize(value),
			})
		}
	}

	cols := []string{"label", "value"}
	if hasSeries {
		cols = []string{"label", "series", "value"}
	}
	return &Result{Shape: "series", Columns: cols, Rows: out}, rows.Err()
}

func (e *Executor) runTable(ctx context.Context, b *builder, spec *QuerySpec, where string) (*Result, error) {
	// Auto-join every reference field so we can surface labels.
	type selEntry struct {
		expr string
		key  string
	}
	var sel []selEntry
	sel = append(sel, selEntry{sourceAlias + ".id", "id"})
	for _, f := range b.ent.Fields {
		col := pgx.Identifier{f.Name}.Sanitize()
		sel = append(sel, selEntry{sourceAlias + "." + col, f.Name})
		if f.DataType == entities.TypeReference && f.ReferenceEntity != "" {
			j, err := b.addRefJoin(ctx, f)
			if err != nil {
				return nil, err
			}
			sel = append(sel, selEntry{j.labelExpr, f.Name + "__label"})
		}
	}
	sel = append(sel, selEntry{sourceAlias + ".created_at", "created_at"})
	sel = append(sel, selEntry{sourceAlias + ".updated_at", "updated_at"})

	// buildWhere was called before joins were added; reorder is fine because
	// joins only supply ref labels and where already references src.
	limit := spec.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	orderBy := sourceAlias + ".created_at DESC"
	if spec.Sort != nil {
		if _, ok := b.fields[spec.Sort.Field]; ok ||
			spec.Sort.Field == "id" || spec.Sort.Field == "created_at" || spec.Sort.Field == "updated_at" {
			orderBy = sourceAlias + "." + pgx.Identifier{spec.Sort.Field}.Sanitize() + " " + strings.ToUpper(spec.Sort.Dir)
		}
	}
	exprs := make([]string, len(sel))
	cols := make([]string, len(sel))
	for i, s := range sel {
		exprs[i] = s.expr + " AS " + pgx.Identifier{s.key}.Sanitize()
		cols[i] = s.key
	}
	q := fmt.Sprintf("SELECT %s FROM %s%s ORDER BY %s LIMIT %d",
		strings.Join(exprs, ", "), b.fromClause(), where, orderBy, limit,
	)
	rows, err := e.pool.Query(ctx, q, b.params...)
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
		return "COUNT(" + sourceAlias + "." + pgx.Identifier{a.Field}.Sanitize() + ")", nil
	case AggSum, AggAvg, AggMin, AggMax:
		dt, ok := resolveField(fields, a.Field)
		if !ok {
			return "", fmt.Errorf("aggregate: unknown field %q", a.Field)
		}
		if (a.Fn == AggSum || a.Fn == AggAvg) && !isNumericType(dt) {
			return "", fmt.Errorf("aggregate: %s requires a numeric field", a.Fn)
		}
		return strings.ToUpper(string(a.Fn)) + "(" + sourceAlias + "." + pgx.Identifier{a.Field}.Sanitize() + ")", nil
	}
	return "", fmt.Errorf("unsupported aggregate %s", a.Fn)
}

func (b *builder) groupByExpr(ctx context.Context, g *GroupBy) (labelExpr, groupExpr string, err error) {
	dt, ok := resolveField(b.fields, g.Field)
	if !ok {
		return "", "", fmt.Errorf("group_by: unknown field %q", g.Field)
	}
	col := sourceAlias + "." + pgx.Identifier{g.Field}.Sanitize()

	if g.Bucket != BucketNone {
		if dt != entities.TypeDate && dt != entities.TypeTimestampTZ {
			return "", "", fmt.Errorf("group_by: bucket only works on date/timestamp fields")
		}
		expr := fmt.Sprintf("date_trunc(%s, %s)", quoteLiteral(string(g.Bucket)), col)
		return expr, expr, nil
	}

	if dt == entities.TypeReference {
		f := b.fields[g.Field]
		if f.ReferenceEntity != "" {
			j, err := b.addRefJoin(ctx, f)
			if err != nil {
				return "", "", err
			}
			return j.labelExpr, col, nil
		}
	}
	return col, col, nil
}

// pickRefLabel chooses the best text field to display for a reference.
// Mirrors entities.pickLabelField but lives here to avoid exporting internals.
func pickRefLabel(ent *entities.Entity) string {
	priorities := []string{"name", "title", "label", "display_name", "email"}
	text := map[string]bool{}
	for _, f := range ent.Fields {
		if f.DataType == entities.TypeText {
			text[f.Name] = true
		}
	}
	for _, p := range priorities {
		if text[p] {
			return p
		}
	}
	return ""
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
