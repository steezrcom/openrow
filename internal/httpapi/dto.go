package httpapi

import (
	"time"

	"github.com/openrow/openrow/internal/auth"
	"github.com/openrow/openrow/internal/entities"
)

type User struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	Name            string     `json:"name"`
	EmailVerifiedAt *time.Time `json:"email_verified_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

type Membership struct {
	ID         string `json:"id"`
	TenantID   string `json:"org_id"`
	TenantSlug string `json:"org_slug"`
	TenantName string `json:"org_name"`
	Role       string `json:"role"`
}

type Field struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	DisplayName     string `json:"display_name"`
	DataType        string `json:"data_type"`
	IsRequired      bool   `json:"is_required"`
	IsUnique        bool   `json:"is_unique"`
	ReferenceEntity string `json:"reference_entity,omitempty"`
}

type Entity struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	Description string    `json:"description,omitempty"`
	Fields      []Field   `json:"fields"`
	CreatedAt   time.Time `json:"created_at"`
}

func userDTO(u *auth.User) *User {
	if u == nil {
		return nil
	}
	return &User{
		ID:              u.ID,
		Email:           u.Email,
		Name:            u.Name,
		EmailVerifiedAt: u.EmailVerifiedAt,
		CreatedAt:       u.CreatedAt,
	}
}

func membershipDTO(m *auth.Membership) *Membership {
	if m == nil {
		return nil
	}
	return &Membership{
		ID:         m.ID,
		TenantID:   m.TenantID,
		TenantSlug: m.TenantSlug,
		TenantName: m.TenantName,
		Role:       string(m.Role),
	}
}

func membershipDTOOrNil(m *auth.Membership) *Membership { return membershipDTO(m) }

func membershipsDTO(ms []auth.Membership) []Membership {
	out := make([]Membership, len(ms))
	for i := range ms {
		out[i] = *membershipDTO(&ms[i])
	}
	return out
}

func entityDTO(e *entities.Entity) *Entity {
	if e == nil {
		return nil
	}
	fields := make([]Field, len(e.Fields))
	for i, f := range e.Fields {
		fields[i] = Field{
			ID:              f.ID,
			Name:            f.Name,
			DisplayName:     f.DisplayName,
			DataType:        string(f.DataType),
			IsRequired:      f.IsRequired,
			IsUnique:        f.IsUnique,
			ReferenceEntity: f.ReferenceEntity,
		}
	}
	return &Entity{
		ID:          e.ID,
		Name:        e.Name,
		DisplayName: e.DisplayName,
		Description: e.Description,
		Fields:      fields,
		CreatedAt:   e.CreatedAt,
	}
}

func entitiesDTO(es []entities.Entity) []Entity {
	out := make([]Entity, len(es))
	for i := range es {
		out[i] = *entityDTO(&es[i])
	}
	return out
}
