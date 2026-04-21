package entities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Row is a single record from an entity's underlying table, keyed by field name.
// Always contains "id", "created_at", "updated_at".
type Row map[string]any

// ListRows returns rows from the entity's table, ordered newest first.
func (s *Service) ListRows(ctx context.Context, schema string, ent *Entity, limit, offset int) ([]Row, error) {
	if !identRe.MatchString(schema) {
		return nil, fmt.Errorf("invalid schema")
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	cols := rowColumns(ent)
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = pgx.Identifier{c}.Sanitize()
	}
	q := fmt.Sprintf(
		"SELECT %s FROM %s ORDER BY created_at DESC LIMIT $1 OFFSET $2",
		strings.Join(quoted, ", "),
		pgx.Identifier{schema, ent.Name}.Sanitize(),
	)
	rows, err := s.pool.Query(ctx, q, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make(Row, len(cols))
		for i, c := range cols {
			row[c] = normalizeValue(vals[i])
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// InsertRow inserts one row and returns its id. input is a map of field-name to raw form value (string, except jsonb which can be []byte).
func (s *Service) InsertRow(ctx context.Context, schema string, ent *Entity, input map[string]string) (string, error) {
	if !identRe.MatchString(schema) {
		return "", fmt.Errorf("invalid schema")
	}

	var (
		cols   []string
		params []any
		place  []string
	)
	idx := 1
	for _, f := range ent.Fields {
		raw, present := input[f.Name]
		raw = strings.TrimSpace(raw)

		if f.DataType == TypeBoolean {
			// Browsers omit unchecked checkboxes. Treat absent as false.
			val := present && (raw == "on" || raw == "true" || raw == "1")
			cols = append(cols, pgx.Identifier{f.Name}.Sanitize())
			place = append(place, fmt.Sprintf("$%d", idx))
			params = append(params, val)
			idx++
			continue
		}

		if !present || raw == "" {
			if f.IsRequired {
				return "", fmt.Errorf("%s is required", f.Name)
			}
			continue
		}

		val, err := coerceValue(f.DataType, raw)
		if err != nil {
			return "", fmt.Errorf("%s: %w", f.Name, err)
		}
		cols = append(cols, pgx.Identifier{f.Name}.Sanitize())
		place = append(place, fmt.Sprintf("$%d", idx))
		params = append(params, val)
		idx++
	}

	table := pgx.Identifier{schema, ent.Name}.Sanitize()
	var q string
	if len(cols) == 0 {
		q = fmt.Sprintf("INSERT INTO %s DEFAULT VALUES RETURNING id", table)
	} else {
		q = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING id",
			table, strings.Join(cols, ", "), strings.Join(place, ", "))
	}

	var id string
	if err := s.pool.QueryRow(ctx, q, params...).Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

// DeleteRow removes one row by id. Returns pgx.ErrNoRows if not found.
func (s *Service) DeleteRow(ctx context.Context, schema string, ent *Entity, id string) error {
	if !identRe.MatchString(schema) {
		return fmt.Errorf("invalid schema")
	}
	q := fmt.Sprintf("DELETE FROM %s WHERE id = $1",
		pgx.Identifier{schema, ent.Name}.Sanitize())
	tag, err := s.pool.Exec(ctx, q, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// RefOption labels a row from a referenced entity, for use in dropdowns.
type RefOption struct {
	ID    string
	Label string
}

// ListRefOptions returns id+label pairs for populating a reference dropdown.
// Labels come from the first "name" / "title" / "label" / "email" text field if one exists, else the id.
func (s *Service) ListRefOptions(ctx context.Context, schema string, target *Entity) ([]RefOption, error) {
	if !identRe.MatchString(schema) {
		return nil, fmt.Errorf("invalid schema")
	}
	labelCol := pickLabelField(target)
	var q string
	if labelCol == "" {
		q = fmt.Sprintf("SELECT id, id::text AS label FROM %s ORDER BY created_at DESC LIMIT 500",
			pgx.Identifier{schema, target.Name}.Sanitize())
	} else {
		q = fmt.Sprintf("SELECT id, COALESCE(%s::text, id::text) AS label FROM %s ORDER BY label ASC LIMIT 500",
			pgx.Identifier{labelCol}.Sanitize(),
			pgx.Identifier{schema, target.Name}.Sanitize())
	}
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RefOption
	for rows.Next() {
		var opt RefOption
		if err := rows.Scan(&opt.ID, &opt.Label); err != nil {
			return nil, err
		}
		out = append(out, opt)
	}
	return out, rows.Err()
}

func pickLabelField(ent *Entity) string {
	priorities := []string{"name", "title", "label", "display_name", "email"}
	fieldNames := make(map[string]bool, len(ent.Fields))
	for _, f := range ent.Fields {
		if f.DataType == TypeText {
			fieldNames[f.Name] = true
		}
	}
	for _, p := range priorities {
		if fieldNames[p] {
			return p
		}
	}
	return ""
}

func rowColumns(ent *Entity) []string {
	cols := make([]string, 0, len(ent.Fields)+3)
	cols = append(cols, "id")
	for _, f := range ent.Fields {
		cols = append(cols, f.Name)
	}
	cols = append(cols, "created_at", "updated_at")
	return cols
}

// normalizeValue converts pgx's raw scan values into JSON-friendly shapes.
// Most notably UUIDs arrive as [16]byte and would serialize as a number array.
func normalizeValue(v any) any {
	switch x := v.(type) {
	case [16]byte:
		return uuid.UUID(x).String()
	}
	return v
}

func coerceValue(t DataType, raw string) (any, error) {
	switch t {
	case TypeText, TypeUUID, TypeDate:
		return raw, nil
	case TypeInteger, TypeBigInt:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid integer %q", raw)
		}
		return n, nil
	case TypeNumeric:
		// Pass through as string; pgx + Postgres handle the numeric cast. Validate format.
		if _, err := strconv.ParseFloat(raw, 64); err != nil {
			return nil, fmt.Errorf("invalid number %q", raw)
		}
		return raw, nil
	case TypeBoolean:
		return raw == "on" || raw == "true" || raw == "1", nil
	case TypeTimestampTZ:
		// <input type=datetime-local> returns "2006-01-02T15:04" (no tz). Parse as local then convert.
		if ts, err := time.Parse("2006-01-02T15:04", raw); err == nil {
			return ts, nil
		}
		if ts, err := time.Parse(time.RFC3339, raw); err == nil {
			return ts, nil
		}
		return nil, fmt.Errorf("invalid timestamp %q", raw)
	case TypeJSONB:
		if !json.Valid([]byte(raw)) {
			return nil, errors.New("invalid json")
		}
		return []byte(raw), nil
	case TypeReference:
		return raw, nil
	}
	return nil, fmt.Errorf("unsupported type %q", t)
}
