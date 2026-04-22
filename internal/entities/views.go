package entities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type ViewType string

const (
	ViewTable   ViewType = "table"
	ViewCards   ViewType = "cards"
	ViewKanban  ViewType = "kanban"
	ViewGallery ViewType = "gallery"
)

// View is a per-entity display configuration. Multiple views per entity
// are allowed; each has its own type + config shape (see the frontend
// for the schemas — we keep it opaque here to avoid churn when a view
// type grows new options).
type View struct {
	ID             string          `json:"id"`
	TenantID       string          `json:"tenant_id"`
	EntityID       string          `json:"entity_id"`
	EntityName     string          `json:"entity_name"`
	Name           string          `json:"name"`
	ViewType       ViewType        `json:"view_type"`
	Config         json.RawMessage `json:"config"`
	Position       int             `json:"position"`
	CreatedByUser  *string         `json:"created_by_user_id,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

var ErrViewNotFound = errors.New("view not found")

func validateViewType(t ViewType) error {
	switch t {
	case ViewTable, ViewCards, ViewKanban, ViewGallery:
		return nil
	}
	return fmt.Errorf("unsupported view_type %q", t)
}

type CreateViewInput struct {
	EntityName string
	Name       string
	ViewType   ViewType
	Config     json.RawMessage
	UserID     string
}

// CreateView inserts a new view. Position is set to max+1 for stable
// ordering in the UI tab bar.
func (s *Service) CreateView(ctx context.Context, tenantID string, in CreateViewInput) (*View, error) {
	if in.Name == "" {
		return nil, errors.New("name is required")
	}
	if err := validateViewType(in.ViewType); err != nil {
		return nil, err
	}
	ent, err := s.Get(ctx, tenantID, in.EntityName)
	if err != nil {
		return nil, err
	}
	cfg := in.Config
	if len(cfg) == 0 {
		cfg = json.RawMessage("{}")
	}
	var createdBy *string
	if in.UserID != "" {
		createdBy = &in.UserID
	}

	var v View
	var cfgOut []byte
	err = s.pool.QueryRow(ctx, `
		INSERT INTO openrow.entity_views
			(tenant_id, entity_id, name, view_type, config, position, created_by_user_id)
		VALUES (
			$1, $2, $3, $4, $5,
			COALESCE((SELECT max(position)+1 FROM openrow.entity_views WHERE entity_id = $2), 0),
			$6
		)
		RETURNING id, tenant_id, entity_id, name, view_type, config, position,
		          created_by_user_id, created_at, updated_at`,
		tenantID, ent.ID, in.Name, string(in.ViewType), cfg, createdBy,
	).Scan(&v.ID, &v.TenantID, &v.EntityID, &v.Name, &v.ViewType, &cfgOut,
		&v.Position, &v.CreatedByUser, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return nil, err
	}
	v.Config = cfgOut
	v.EntityName = ent.Name
	return &v, nil
}

// ListViews returns all views for an entity, ordered by position.
func (s *Service) ListViews(ctx context.Context, tenantID, entityName string) ([]View, error) {
	ent, err := s.Get(ctx, tenantID, entityName)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, entity_id, name, view_type, config, position,
		       created_by_user_id, created_at, updated_at
		FROM openrow.entity_views
		WHERE entity_id = $1
		ORDER BY position, created_at`, ent.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]View, 0)
	for rows.Next() {
		var v View
		var cfg []byte
		if err := rows.Scan(&v.ID, &v.TenantID, &v.EntityID, &v.Name, &v.ViewType, &cfg,
			&v.Position, &v.CreatedByUser, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		v.Config = cfg
		v.EntityName = ent.Name
		out = append(out, v)
	}
	return out, rows.Err()
}

type UpdateViewInput struct {
	Name     *string
	ViewType *ViewType
	Config   *json.RawMessage
	Position *int
}

func (s *Service) UpdateView(ctx context.Context, tenantID, id string, in UpdateViewInput) (*View, error) {
	existing, err := s.getView(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		existing.Name = *in.Name
	}
	if in.ViewType != nil {
		if err := validateViewType(*in.ViewType); err != nil {
			return nil, err
		}
		existing.ViewType = *in.ViewType
	}
	if in.Config != nil {
		existing.Config = *in.Config
	}
	if in.Position != nil {
		existing.Position = *in.Position
	}
	cfg := existing.Config
	if len(cfg) == 0 {
		cfg = json.RawMessage("{}")
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE openrow.entity_views
		SET name = $1, view_type = $2, config = $3, position = $4, updated_at = now()
		WHERE tenant_id = $5 AND id = $6`,
		existing.Name, string(existing.ViewType), cfg, existing.Position, tenantID, id)
	if err != nil {
		return nil, err
	}
	return s.getView(ctx, tenantID, id)
}

func (s *Service) DeleteView(ctx context.Context, tenantID, id string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM openrow.entity_views WHERE tenant_id = $1 AND id = $2`,
		tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrViewNotFound
	}
	return nil
}

func (s *Service) getView(ctx context.Context, tenantID, id string) (*View, error) {
	var v View
	var cfg []byte
	err := s.pool.QueryRow(ctx, `
		SELECT v.id, v.tenant_id, v.entity_id, e.name, v.name, v.view_type, v.config, v.position,
		       v.created_by_user_id, v.created_at, v.updated_at
		FROM openrow.entity_views v
		JOIN openrow.entities e ON e.id = v.entity_id
		WHERE v.tenant_id = $1 AND v.id = $2`, tenantID, id).
		Scan(&v.ID, &v.TenantID, &v.EntityID, &v.EntityName, &v.Name, &v.ViewType, &cfg,
			&v.Position, &v.CreatedByUser, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrViewNotFound
	}
	if err != nil {
		return nil, err
	}
	v.Config = cfg
	return &v, nil
}
