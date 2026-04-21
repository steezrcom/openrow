package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

type Membership struct {
	ID         string
	UserID     string
	TenantID   string
	TenantSlug string
	TenantName string
	PGSchema   string
	Role       Role
}

type MembershipService struct {
	pool *pgxpool.Pool
}

func NewMembershipService(pool *pgxpool.Pool) *MembershipService {
	return &MembershipService{pool: pool}
}

// Add creates a membership. Idempotent on (user_id, tenant_id).
func (s *MembershipService) Add(ctx context.Context, userID, tenantID string, role Role) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO steezr.memberships (user_id, tenant_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, tenant_id) DO NOTHING`,
		userID, tenantID, string(role))
	return err
}

// ForUser returns the orgs a user has access to, newest org first.
func (s *MembershipService) ForUser(ctx context.Context, userID string) ([]Membership, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.id, m.user_id, m.tenant_id, t.slug, t.name, t.pg_schema, m.role::text
		FROM steezr.memberships m
		JOIN steezr.tenants t ON t.id = m.tenant_id
		WHERE m.user_id = $1
		ORDER BY t.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Membership, 0)
	for rows.Next() {
		var m Membership
		var role string
		if err := rows.Scan(&m.ID, &m.UserID, &m.TenantID, &m.TenantSlug, &m.TenantName, &m.PGSchema, &role); err != nil {
			return nil, err
		}
		m.Role = Role(role)
		out = append(out, m)
	}
	return out, rows.Err()
}

// Get returns the user's membership for a specific tenant, or ErrNoMembership.
func (s *MembershipService) Get(ctx context.Context, userID, tenantID string) (*Membership, error) {
	var m Membership
	var role string
	err := s.pool.QueryRow(ctx, `
		SELECT m.id, m.user_id, m.tenant_id, t.slug, t.name, t.pg_schema, m.role::text
		FROM steezr.memberships m
		JOIN steezr.tenants t ON t.id = m.tenant_id
		WHERE m.user_id = $1 AND m.tenant_id = $2`,
		userID, tenantID,
	).Scan(&m.ID, &m.UserID, &m.TenantID, &m.TenantSlug, &m.TenantName, &m.PGSchema, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoMembership
	}
	if err != nil {
		return nil, err
	}
	m.Role = Role(role)
	return &m, nil
}

var ErrNoMembership = errors.New("no membership")
