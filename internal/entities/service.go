package entities

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Entity struct {
	ID          string
	TenantID    string
	Name        string
	DisplayName string
	Description string
	Fields      []Field
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Field struct {
	ID              string
	Name            string
	DisplayName     string
	DataType        DataType
	IsRequired      bool
	IsUnique        bool
	ReferenceEntity string
	Position        int
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Create persists the entity metadata and creates the underlying table atomically.
func (s *Service) Create(ctx context.Context, tenantID, schema string, spec *EntitySpec) (*Entity, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}

	ddl, err := resolveAndBuildCreate(ctx, s.pool, tenantID, schema, spec)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var entityID string
	err = tx.QueryRow(ctx, `
		INSERT INTO steezr.entities (tenant_id, name, display_name, description)
		VALUES ($1, $2, $3, $4)
		RETURNING id`,
		tenantID, spec.Name, spec.DisplayName, spec.Description,
	).Scan(&entityID)
	if err != nil {
		return nil, fmt.Errorf("insert entity: %w", err)
	}

	for i, f := range spec.Fields {
		var refEntityID *string
		if f.DataType == TypeReference {
			id, err := lookupEntityID(ctx, tx, tenantID, f.ReferenceEntity)
			if err != nil {
				return nil, err
			}
			refEntityID = &id
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO steezr.fields (entity_id, name, display_name, data_type, is_required, is_unique, reference_entity_id, position)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			entityID, f.Name, f.DisplayName, string(f.DataType),
			f.IsRequired, f.IsUnique, refEntityID, i,
		); err != nil {
			return nil, fmt.Errorf("insert field %q: %w", f.Name, err)
		}
	}

	if _, err := tx.Exec(ctx, ddl); err != nil {
		return nil, fmt.Errorf("apply ddl: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return s.Get(ctx, tenantID, spec.Name)
}

func (s *Service) Get(ctx context.Context, tenantID, name string) (*Entity, error) {
	var e Entity
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, display_name, COALESCE(description, ''), created_at, updated_at
		FROM steezr.entities
		WHERE tenant_id = $1 AND name = $2`,
		tenantID, name,
	).Scan(&e.ID, &e.TenantID, &e.Name, &e.DisplayName, &e.Description, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT f.id, f.name, f.display_name, f.data_type, f.is_required, f.is_unique,
		       COALESCE(re.name, ''), f.position
		FROM steezr.fields f
		LEFT JOIN steezr.entities re ON re.id = f.reference_entity_id
		WHERE f.entity_id = $1
		ORDER BY f.position`, e.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var f Field
		var dt string
		if err := rows.Scan(&f.ID, &f.Name, &f.DisplayName, &dt,
			&f.IsRequired, &f.IsUnique, &f.ReferenceEntity, &f.Position); err != nil {
			return nil, err
		}
		f.DataType = DataType(dt)
		e.Fields = append(e.Fields, f)
	}
	return &e, rows.Err()
}

func (s *Service) List(ctx context.Context, tenantID string) ([]Entity, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, name, display_name, COALESCE(description, ''), created_at, updated_at
		FROM steezr.entities
		WHERE tenant_id = $1
		ORDER BY display_name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Entity, 0)
	for rows.Next() {
		var e Entity
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Name, &e.DisplayName, &e.Description,
			&e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// AddField appends a new column to an existing entity. Adds the metadata row
// and the ALTER TABLE in one transaction. Fails if the field name already exists.
func (s *Service) AddField(ctx context.Context, tenantID, schema string, entityName string, f FieldSpec) (*Entity, error) {
	if !identRe.MatchString(f.Name) {
		return nil, fmt.Errorf("field name %q must match [a-z][a-z0-9_]{0,62}", f.Name)
	}
	if _, bad := reservedNames[f.Name]; bad {
		return nil, fmt.Errorf("field name %q is reserved", f.Name)
	}
	if _, ok := f.DataType.SQL(); !ok {
		return nil, fmt.Errorf("unknown data_type %q", f.DataType)
	}

	ent, err := s.Get(ctx, tenantID, entityName)
	if err != nil {
		return nil, err
	}

	ddl, err := AddColumnSQL(schema, ent.Name, f)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var refEntityID *string
	if f.DataType == TypeReference {
		id, err := lookupEntityID(ctx, tx, tenantID, f.ReferenceEntity)
		if err != nil {
			return nil, err
		}
		refEntityID = &id
	}
	nextPos := len(ent.Fields)
	if _, err := tx.Exec(ctx, `
		INSERT INTO steezr.fields (entity_id, name, display_name, data_type, is_required, is_unique, reference_entity_id, position)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		ent.ID, f.Name, f.DisplayName, string(f.DataType),
		f.IsRequired, f.IsUnique, refEntityID, nextPos,
	); err != nil {
		return nil, fmt.Errorf("insert field: %w", err)
	}
	if _, err := tx.Exec(ctx, ddl); err != nil {
		return nil, fmt.Errorf("apply ddl: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE steezr.entities SET updated_at = now() WHERE id = $1`, ent.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, entityName)
}

// DropField removes a column from an existing entity. Rejects if the field
// doesn't exist or if any other entity references it via FK (Postgres enforces).
func (s *Service) DropField(ctx context.Context, tenantID, schema, entityName, fieldName string) error {
	if !identRe.MatchString(fieldName) {
		return fmt.Errorf("invalid field name")
	}
	if !identRe.MatchString(entityName) {
		return fmt.Errorf("invalid entity name")
	}
	if !identRe.MatchString(schema) {
		return fmt.Errorf("invalid schema")
	}
	if _, reserved := reservedNames[fieldName]; reserved {
		return fmt.Errorf("field name %q is built-in; cannot drop", fieldName)
	}

	ent, err := s.Get(ctx, tenantID, entityName)
	if err != nil {
		return err
	}
	var fieldID string
	for _, f := range ent.Fields {
		if f.Name == fieldName {
			fieldID = f.ID
			break
		}
	}
	if fieldID == "" {
		return fmt.Errorf("field %q not found on %q", fieldName, entityName)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM steezr.fields WHERE id = $1`, fieldID); err != nil {
		return err
	}
	ddl := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s",
		pgx.Identifier{schema, ent.Name}.Sanitize(),
		pgx.Identifier{fieldName}.Sanitize(),
	)
	if _, err := tx.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("drop column: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE steezr.entities SET updated_at = now() WHERE id = $1`, ent.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func resolveAndBuildCreate(ctx context.Context, pool *pgxpool.Pool, tenantID, schema string, spec *EntitySpec) (string, error) {
	for _, f := range spec.Fields {
		if f.DataType != TypeReference {
			continue
		}
		if _, err := lookupEntityID(ctx, pool, tenantID, f.ReferenceEntity); err != nil {
			return "", err
		}
	}
	return CreateTableSQL(schema, spec)
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func lookupEntityID(ctx context.Context, q querier, tenantID, name string) (string, error) {
	var id string
	err := q.QueryRow(ctx,
		`SELECT id FROM steezr.entities WHERE tenant_id = $1 AND name = $2`,
		tenantID, name,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("referenced entity %q does not exist", name)
	}
	if err != nil {
		return "", fmt.Errorf("lookup reference %q: %w", name, err)
	}
	return id, nil
}
