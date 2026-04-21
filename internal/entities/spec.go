package entities

import (
	"fmt"
	"regexp"
)

type DataType string

const (
	TypeText        DataType = "text"
	TypeInteger     DataType = "integer"
	TypeBigInt      DataType = "bigint"
	TypeNumeric     DataType = "numeric"
	TypeBoolean     DataType = "boolean"
	TypeDate        DataType = "date"
	TypeTimestampTZ DataType = "timestamptz"
	TypeUUID        DataType = "uuid"
	TypeJSONB       DataType = "jsonb"
	TypeReference   DataType = "reference"
)

func (t DataType) SQL() (string, bool) {
	switch t {
	case TypeText:
		return "TEXT", true
	case TypeInteger:
		return "INTEGER", true
	case TypeBigInt:
		return "BIGINT", true
	case TypeNumeric:
		return "NUMERIC", true
	case TypeBoolean:
		return "BOOLEAN", true
	case TypeDate:
		return "DATE", true
	case TypeTimestampTZ:
		return "TIMESTAMPTZ", true
	case TypeUUID:
		return "UUID", true
	case TypeJSONB:
		return "JSONB", true
	case TypeReference:
		return "UUID", true
	}
	return "", false
}

type FieldSpec struct {
	Name              string   `json:"name"`
	DisplayName       string   `json:"display_name"`
	DataType          DataType `json:"data_type"`
	IsRequired        bool     `json:"is_required"`
	IsUnique          bool     `json:"is_unique"`
	ReferenceEntity   string   `json:"reference_entity,omitempty"`
}

type EntitySpec struct {
	Name        string      `json:"name"`
	DisplayName string      `json:"display_name"`
	Description string      `json:"description,omitempty"`
	Fields      []FieldSpec `json:"fields"`
}

var identRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

var reservedNames = map[string]struct{}{
	"id": {}, "created_at": {}, "updated_at": {},
}

func (s *EntitySpec) Validate() error {
	if !identRe.MatchString(s.Name) {
		return fmt.Errorf("entity name %q must match [a-z][a-z0-9_]{0,62}", s.Name)
	}
	if s.DisplayName == "" {
		return fmt.Errorf("entity %q: display_name required", s.Name)
	}
	seen := make(map[string]struct{}, len(s.Fields))
	for i, f := range s.Fields {
		if !identRe.MatchString(f.Name) {
			return fmt.Errorf("field[%d] name %q must match [a-z][a-z0-9_]{0,62}", i, f.Name)
		}
		if _, bad := reservedNames[f.Name]; bad {
			return fmt.Errorf("field name %q is reserved", f.Name)
		}
		if _, dup := seen[f.Name]; dup {
			return fmt.Errorf("duplicate field name %q", f.Name)
		}
		seen[f.Name] = struct{}{}
		if _, ok := f.DataType.SQL(); !ok {
			return fmt.Errorf("field %q: unknown data_type %q", f.Name, f.DataType)
		}
		if f.DataType == TypeReference && !identRe.MatchString(f.ReferenceEntity) {
			return fmt.Errorf("field %q: reference_entity required for reference type", f.Name)
		}
	}
	return nil
}
