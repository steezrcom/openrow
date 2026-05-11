package discovery

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping db test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestBindingCreateAndGet(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewRepo(pool)

	tenantID := ensureTenant(t, pool)

	b, err := repo.Create(ctx, tenantID, "abra-flexi")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if b.State != StateDiscovering {
		t.Fatalf("state = %q, want discovering", b.State)
	}

	got, err := repo.GetByID(ctx, tenantID, b.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != b.ID {
		t.Fatalf("id mismatch")
	}
}

func TestBindingUpdateMappingAndState(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewRepo(pool)
	tenantID := ensureTenant(t, pool)

	b, _ := repo.Create(ctx, tenantID, "abra-flexi")
	art := &Artifact{
		Version: 1, Connector: "abra-flexi", DiscoveredAt: time.Now(),
		Evidences: map[string]*Evidence{"adresar": {Role: RoleCustomer, Confidence: 1}},
	}
	if err := repo.SaveMapping(ctx, b.ID, art); err != nil {
		t.Fatalf("save mapping: %v", err)
	}
	if err := repo.UpdateState(ctx, b.ID, StateProposed, ""); err != nil {
		t.Fatalf("update state: %v", err)
	}
	got, _ := repo.GetByID(ctx, tenantID, b.ID)
	if got.State != StateProposed {
		t.Fatalf("state not updated")
	}
	if got.Mapping == nil || got.Mapping.Evidences["adresar"].Role != RoleCustomer {
		t.Fatalf("mapping not persisted")
	}
}

func ensureTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO openrow.tenants (slug, name, pg_schema) VALUES ($1, $2, $3)
		 ON CONFLICT (slug) DO UPDATE SET slug = EXCLUDED.slug
		 RETURNING id`,
		"test-discovery", "Test Discovery", "tenant_test_discovery",
	).Scan(&id)
	if err != nil {
		t.Fatalf("ensure tenant: %v", err)
	}
	return id
}
