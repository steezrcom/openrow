package tenant

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Tenant struct {
	ID       string
	Slug     string
	Name     string
	PGSchema string
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

var slugRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,30}$`)

func (s *Service) Create(ctx context.Context, slug, name string) (*Tenant, error) {
	if !slugRe.MatchString(slug) {
		return nil, fmt.Errorf("slug %q must match [a-z][a-z0-9_]{0,30}", slug)
	}
	pgSchema := "tenant_" + slug

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO openrow.tenants (slug, name, pg_schema)
		VALUES ($1, $2, $3)
		RETURNING id`,
		slug, name, pgSchema,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("insert tenant: %w", err)
	}

	if _, err := tx.Exec(ctx,
		fmt.Sprintf("CREATE SCHEMA %s", pgx.Identifier{pgSchema}.Sanitize())); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &Tenant{ID: id, Slug: slug, Name: name, PGSchema: pgSchema}, nil
}

func (s *Service) ByID(ctx context.Context, id string) (*Tenant, error) {
	var t Tenant
	err := s.pool.QueryRow(ctx, `
		SELECT id, slug, name, pg_schema
		FROM openrow.tenants
		WHERE id = $1`, id,
	).Scan(&t.ID, &t.Slug, &t.Name, &t.PGSchema)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Service) BySlug(ctx context.Context, slug string) (*Tenant, error) {
	var t Tenant
	err := s.pool.QueryRow(ctx, `
		SELECT id, slug, name, pg_schema
		FROM openrow.tenants
		WHERE slug = $1`, slug,
	).Scan(&t.ID, &t.Slug, &t.Name, &t.PGSchema)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Service) List(ctx context.Context) ([]Tenant, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, slug, name, pg_schema
		FROM openrow.tenants
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tenant
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.Slug, &t.Name, &t.PGSchema); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

var ErrNotFound = errors.New("tenant not found")
