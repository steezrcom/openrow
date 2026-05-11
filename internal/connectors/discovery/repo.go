package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type State string

const (
	StateDiscovering State = "discovering"
	StateProposed    State = "proposed"
	StateActive      State = "active"
	StateError       State = "error"
)

type Binding struct {
	ID                string    `json:"id"`
	TenantID          string    `json:"tenant_id"`
	ConnectorID       string    `json:"connector_id"`
	State             State     `json:"state"`
	Mapping           *Artifact `json:"mapping,omitempty"`
	LLMClassification bool      `json:"llm_classification"`
	LastError         string    `json:"last_error,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

var ErrNotFound = errors.New("binding not found")

func (r *Repo) Create(ctx context.Context, tenantID, connectorID string) (*Binding, error) {
	var b Binding
	err := r.pool.QueryRow(ctx, `
		INSERT INTO openrow.external_bindings (tenant_id, connector_id, state)
		VALUES ($1, $2, 'discovering')
		ON CONFLICT (tenant_id, connector_id) DO UPDATE
		   SET state = 'discovering', last_error = NULL, updated_at = now()
		RETURNING id, tenant_id, connector_id, state, llm_classification, created_at, updated_at`,
		tenantID, connectorID,
	).Scan(&b.ID, &b.TenantID, &b.ConnectorID, &b.State, &b.LLMClassification, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *Repo) GetByID(ctx context.Context, tenantID, bindingID string) (*Binding, error) {
	var (
		b    Binding
		raw  []byte
		lerr *string
	)
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, connector_id, state, mapping, llm_classification,
		       last_error, created_at, updated_at
		FROM openrow.external_bindings
		WHERE id = $1 AND tenant_id = $2`,
		bindingID, tenantID,
	).Scan(&b.ID, &b.TenantID, &b.ConnectorID, &b.State, &raw, &b.LLMClassification,
		&lerr, &b.CreatedAt, &b.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if lerr != nil {
		b.LastError = *lerr
	}
	if len(raw) > 0 {
		var art Artifact
		if err := json.Unmarshal(raw, &art); err != nil {
			return nil, fmt.Errorf("decode mapping: %w", err)
		}
		b.Mapping = &art
	}
	return &b, nil
}

func (r *Repo) GetByConnector(ctx context.Context, tenantID, connectorID string) (*Binding, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM openrow.external_bindings WHERE tenant_id = $1 AND connector_id = $2`,
		tenantID, connectorID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, tenantID, id)
}

func (r *Repo) SaveMapping(ctx context.Context, bindingID string, art *Artifact) error {
	raw, err := json.Marshal(art)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx,
		`UPDATE openrow.external_bindings SET mapping = $2, updated_at = now() WHERE id = $1`,
		bindingID, raw)
	return err
}

func (r *Repo) UpdateState(ctx context.Context, bindingID string, state State, lastErr string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE openrow.external_bindings
		SET state = $2, last_error = NULLIF($3, ''), updated_at = now()
		WHERE id = $1`,
		bindingID, state, lastErr)
	return err
}

type ReviewItem struct {
	ID         string          `json:"id"`
	BindingID  string          `json:"binding_id"`
	Evidence   string          `json:"evidence"`
	Field      string          `json:"field,omitempty"`
	Proposed   json.RawMessage `json:"proposed"`
	Status     string          `json:"status"`
	CreatedAt  time.Time       `json:"created_at"`
	ResolvedAt *time.Time      `json:"resolved_at,omitempty"`
}

func (r *Repo) ReplaceReviewItems(ctx context.Context, bindingID string, items []ReviewItem) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`DELETE FROM openrow.external_binding_review_items WHERE binding_id = $1 AND status = 'pending'`,
		bindingID); err != nil {
		return err
	}
	for _, it := range items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO openrow.external_binding_review_items (binding_id, evidence, field, proposed)
			VALUES ($1, $2, NULLIF($3,''), $4)`,
			bindingID, it.Evidence, it.Field, []byte(it.Proposed)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Repo) ListReviewItems(ctx context.Context, bindingID string) ([]ReviewItem, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, binding_id, evidence, COALESCE(field, ''), proposed, status, created_at, resolved_at
		FROM openrow.external_binding_review_items
		WHERE binding_id = $1
		ORDER BY status, created_at`, bindingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReviewItem{}
	for rows.Next() {
		var it ReviewItem
		var raw []byte
		if err := rows.Scan(&it.ID, &it.BindingID, &it.Evidence, &it.Field, &raw,
			&it.Status, &it.CreatedAt, &it.ResolvedAt); err != nil {
			return nil, err
		}
		it.Proposed = raw
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *Repo) ListByTenant(ctx context.Context, tenantID string) ([]*Binding, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id FROM openrow.external_bindings WHERE tenant_id = $1 ORDER BY created_at DESC`,
		tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]*Binding, 0, len(ids))
	for _, id := range ids {
		b, err := r.GetByID(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

// ResolveReviewItem transitions a review item to its terminal status. The
// action verb (accept/edit/reject) maps to the persisted status enum
// (accepted/edited/rejected). The bindingID scope guards against cross-tenant
// resolution by item ID alone.
func (r *Repo) ResolveReviewItem(ctx context.Context, bindingID, itemID, action string) error {
	var status string
	switch action {
	case "accept":
		status = "accepted"
	case "edit":
		status = "edited"
	case "reject":
		status = "rejected"
	default:
		return fmt.Errorf("invalid action %q", action)
	}
	var id string
	err := r.pool.QueryRow(ctx, `
		UPDATE openrow.external_binding_review_items
		SET status = $3, resolved_at = now()
		WHERE id = $1 AND binding_id = $2
		RETURNING id`, itemID, bindingID, status).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// ResolveRole returns (evidence, field_map) for a canonical role on
// (tenant, connector). Errors if the binding is not in 'active' state.
func (r *Repo) ResolveRole(ctx context.Context, tenantID, connectorID, role string) (string, map[string]string, error) {
	b, err := r.GetByConnector(ctx, tenantID, connectorID)
	if err != nil {
		return "", nil, err
	}
	if b.State != StateActive {
		return "", nil, fmt.Errorf("binding state is %q, not active", b.State)
	}
	if b.Mapping == nil {
		return "", nil, fmt.Errorf("binding has no mapping")
	}
	for name, ev := range b.Mapping.Evidences {
		if string(ev.Role) == role {
			fm := map[string]string{}
			for fname, f := range ev.Fields {
				if f.Role != RoleUnknown && f.Role != "" {
					fm[string(f.Role)] = fname
				}
			}
			return name, fm, nil
		}
	}
	return "", nil, fmt.Errorf("role %q not mapped", role)
}
