package entities

import (
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func qualified(schema, table string) string {
	return pgx.Identifier{schema, table}.Sanitize()
}

func quoted(ident string) string {
	return pgx.Identifier{ident}.Sanitize()
}

// CreateTableSQL returns the DDL to create the table for a validated EntitySpec
// inside the given schema. Every row gets id/created_at/updated_at columns.
func CreateTableSQL(schema string, spec *EntitySpec) (string, error) {
	if !identRe.MatchString(schema) {
		return "", fmt.Errorf("invalid schema name %q", schema)
	}
	if err := spec.Validate(); err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "CREATE TABLE %s (\n", qualified(schema, spec.Name))
	b.WriteString("    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),\n")

	for _, f := range spec.Fields {
		sqlType, _ := f.DataType.SQL()
		fmt.Fprintf(&b, "    %s %s", quoted(f.Name), sqlType)
		if f.IsRequired {
			b.WriteString(" NOT NULL")
		}
		if f.IsUnique {
			b.WriteString(" UNIQUE")
		}
		if f.DataType == TypeReference {
			fmt.Fprintf(&b, " REFERENCES %s(id) ON DELETE RESTRICT",
				qualified(schema, f.ReferenceEntity))
		}
		b.WriteString(",\n")
	}

	b.WriteString("    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),\n")
	b.WriteString("    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()\n")
	b.WriteString(");")
	return b.String(), nil
}

// AddColumnSQL returns DDL to add a single new field to an existing table.
func AddColumnSQL(schema, table string, f FieldSpec) (string, error) {
	if !identRe.MatchString(schema) {
		return "", fmt.Errorf("invalid schema name %q", schema)
	}
	if !identRe.MatchString(table) {
		return "", fmt.Errorf("invalid table name %q", table)
	}
	if !identRe.MatchString(f.Name) {
		return "", fmt.Errorf("invalid field name %q", f.Name)
	}
	sqlType, ok := f.DataType.SQL()
	if !ok {
		return "", fmt.Errorf("unknown data_type %q", f.DataType)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "ALTER TABLE %s ADD COLUMN %s %s",
		qualified(schema, table), quoted(f.Name), sqlType)
	// Adding NOT NULL on an existing table with rows would fail; skip for now.
	if f.IsUnique {
		b.WriteString(" UNIQUE")
	}
	if f.DataType == TypeReference {
		if !identRe.MatchString(f.ReferenceEntity) {
			return "", fmt.Errorf("reference_entity required for reference column")
		}
		fmt.Fprintf(&b, " REFERENCES %s(id) ON DELETE RESTRICT",
			qualified(schema, f.ReferenceEntity))
	}
	b.WriteString(";")
	return b.String(), nil
}
