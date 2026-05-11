package flexi

import (
	"context"
	"encoding/json"
	"testing"
)

func TestActionQueryByRole(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()

	creds := map[string]string{
		"base_url": srv.URL, "company": "demo", "username": "user1", "password": "pw",
	}
	resolver := stubMappingResolver{
		role: "adresar",
		fieldMap: map[string]string{
			"display_name":       "nazev",
			"registration_id_cz": "ic",
			"vat_id_cz":          "dic",
			"email":              "email",
		},
	}
	in := json.RawMessage(`{"role":"customer","limit":10}`)
	result, err := queryWithResolver(context.Background(), creds, in, resolver, true)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	rows, ok := result.([]map[string]any)
	if !ok || len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %T %+v", result, result)
	}
	if rows[0]["display_name"] != "Acme s.r.o." {
		t.Fatalf("expected mapped display_name, got %+v", rows[0])
	}
}

type stubMappingResolver struct {
	role     string
	fieldMap map[string]string
}

func (s stubMappingResolver) Resolve(ctx context.Context, tenantID, connectorID, role string) (string, map[string]string, error) {
	return s.role, s.fieldMap, nil
}
