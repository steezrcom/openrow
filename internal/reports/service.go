package reports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Dashboard struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenant_id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"`
	// DefaultDateRange is a preset key the UI seeds into its date-range
	// picker on first render (e.g. "mtd", "ytd", "all"). Empty means no
	// server-side opinion; the frontend falls back to its own default.
	DefaultDateRange string    `json:"default_date_range,omitempty"`
	Position         int       `json:"position"`
	Reports          []Report  `json:"reports,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type Report struct {
	ID          string                 `json:"id"`
	DashboardID string                 `json:"dashboard_id"`
	Title       string                 `json:"title"`
	Subtitle    string                 `json:"subtitle,omitempty"`
	WidgetType  WidgetType             `json:"widget_type"`
	QuerySpec   QuerySpec              `json:"query_spec"`
	Options     map[string]interface{} `json:"options"`
	Width       int                    `json:"width"`
	Position    int                    `json:"position"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

var ErrNotFound = errors.New("dashboard not found")

var slugStripRe = regexp.MustCompile(`[^a-z0-9_]+`)
var slugEdgeRe = regexp.MustCompile(`^_+|_+$`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugStripRe.ReplaceAllString(s, "_")
	s = slugEdgeRe.ReplaceAllString(s, "")
	if len(s) > 60 {
		s = s[:60]
	}
	if s == "" {
		s = "dashboard"
	}
	return s
}

// CreateDashboardInput is passed both via HTTP and via the agent tool.
type CreateDashboardInput struct {
	Name             string              `json:"name"`
	Slug             string              `json:"slug,omitempty"`
	Description      string              `json:"description,omitempty"`
	DefaultDateRange string              `json:"default_date_range,omitempty"`
	Reports          []CreateReportInput `json:"reports,omitempty"`
}

type CreateReportInput struct {
	Title      string                 `json:"title"`
	Subtitle   string                 `json:"subtitle,omitempty"`
	WidgetType WidgetType             `json:"widget_type"`
	QuerySpec  QuerySpec              `json:"query_spec"`
	Options    map[string]interface{} `json:"options,omitempty"`
	Width      int                    `json:"width,omitempty"`
}

// Create inserts the dashboard + reports in one transaction.
// Returns the full dashboard with reports hydrated.
func (s *Service) Create(ctx context.Context, tenantID string, in CreateDashboardInput) (*Dashboard, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, errors.New("name is required")
	}
	slug := in.Slug
	if slug == "" {
		slug = slugify(in.Name)
	} else {
		slug = slugify(slug)
	}

	for i, r := range in.Reports {
		if err := validateReportInput(r); err != nil {
			return nil, fmt.Errorf("reports[%d]: %w", i, err)
		}
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO openrow.dashboards (tenant_id, name, slug, description, default_date_range)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		tenantID, in.Name, slug, nullIfEmpty(in.Description), nullIfEmpty(in.DefaultDateRange),
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("insert dashboard: %w", err)
	}
	for i, r := range in.Reports {
		width := r.Width
		if width <= 0 || width > 12 {
			width = 6
		}
		spec, _ := json.Marshal(r.QuerySpec)
		opts, _ := json.Marshal(marshalOptions(r.Options))
		if _, err := tx.Exec(ctx, `
			INSERT INTO openrow.reports (dashboard_id, title, subtitle, widget_type, query_spec, options, width, position)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			id, r.Title, nullIfEmpty(r.Subtitle), string(r.WidgetType), spec, opts, width, i,
		); err != nil {
			return nil, fmt.Errorf("insert report %d: %w", i, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.Get(ctx, tenantID, slug)
}

func validateReportInput(r CreateReportInput) error {
	if strings.TrimSpace(r.Title) == "" {
		return errors.New("title is required")
	}
	if !r.WidgetType.Valid() {
		return fmt.Errorf("invalid widget_type %q", r.WidgetType)
	}
	if err := r.QuerySpec.Validate(); err != nil {
		return err
	}
	// For KPI and pie, aggregate is required. For table, no aggregate.
	switch r.WidgetType {
	case WidgetKPI:
		if r.QuerySpec.Aggregate == nil || r.QuerySpec.GroupBy != nil {
			return errors.New("kpi needs an aggregate and no group_by")
		}
		if r.QuerySpec.SeriesBy != nil {
			return errors.New("kpi does not support series_by")
		}
	case WidgetBar, WidgetLine, WidgetArea:
		if r.QuerySpec.Aggregate == nil || r.QuerySpec.GroupBy == nil {
			return errors.New(string(r.WidgetType) + " needs both aggregate and group_by")
		}
	case WidgetPie:
		if r.QuerySpec.Aggregate == nil || r.QuerySpec.GroupBy == nil {
			return errors.New("pie needs both aggregate and group_by")
		}
		if r.QuerySpec.SeriesBy != nil {
			return errors.New("pie does not support series_by")
		}
	case WidgetTable:
		if r.QuerySpec.Aggregate != nil {
			return errors.New("table must not have an aggregate")
		}
		if r.QuerySpec.SeriesBy != nil {
			return errors.New("table does not support series_by")
		}
	}
	return nil
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func marshalOptions(opts map[string]interface{}) map[string]interface{} {
	if opts == nil {
		return map[string]interface{}{}
	}
	return opts
}

func (s *Service) List(ctx context.Context, tenantID string) ([]Dashboard, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, name, slug, COALESCE(description, ''), COALESCE(default_date_range, ''), position, created_at, updated_at
		FROM openrow.dashboards
		WHERE tenant_id = $1
		ORDER BY position, name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Dashboard, 0)
	for rows.Next() {
		var d Dashboard
		if err := rows.Scan(&d.ID, &d.TenantID, &d.Name, &d.Slug, &d.Description, &d.DefaultDateRange,
			&d.Position, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Service) Get(ctx context.Context, tenantID, slug string) (*Dashboard, error) {
	var d Dashboard
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, slug, COALESCE(description, ''), COALESCE(default_date_range, ''), position, created_at, updated_at
		FROM openrow.dashboards
		WHERE tenant_id = $1 AND slug = $2`,
		tenantID, slug,
	).Scan(&d.ID, &d.TenantID, &d.Name, &d.Slug, &d.Description, &d.DefaultDateRange, &d.Position, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	rep, err := s.pool.Query(ctx, `
		SELECT id, dashboard_id, title, COALESCE(subtitle, ''), widget_type, query_spec, options, width, position, created_at, updated_at
		FROM openrow.reports
		WHERE dashboard_id = $1
		ORDER BY position`, d.ID)
	if err != nil {
		return nil, err
	}
	defer rep.Close()
	reports := make([]Report, 0)
	for rep.Next() {
		var r Report
		var wt string
		var spec, opts []byte
		if err := rep.Scan(&r.ID, &r.DashboardID, &r.Title, &r.Subtitle, &wt, &spec, &opts,
			&r.Width, &r.Position, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.WidgetType = WidgetType(wt)
		if err := json.Unmarshal(spec, &r.QuerySpec); err != nil {
			return nil, fmt.Errorf("decode report %s: %w", r.ID, err)
		}
		r.Options = map[string]interface{}{}
		if len(opts) > 0 {
			_ = json.Unmarshal(opts, &r.Options)
		}
		reports = append(reports, r)
	}
	d.Reports = reports
	return &d, rep.Err()
}

// UpdateDashboardInput is the patchable subset of a dashboard.
type UpdateDashboardInput struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

func (s *Service) UpdateDashboard(ctx context.Context, tenantID, slug string, in UpdateDashboardInput) (*Dashboard, error) {
	sets := []string{}
	params := []interface{}{}
	idx := 1
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" {
			return nil, errors.New("name cannot be empty")
		}
		sets = append(sets, fmt.Sprintf("name = $%d", idx))
		params = append(params, n)
		idx++
	}
	if in.Description != nil {
		sets = append(sets, fmt.Sprintf("description = $%d", idx))
		params = append(params, nullIfEmpty(*in.Description))
		idx++
	}
	if len(sets) == 0 {
		return s.Get(ctx, tenantID, slug)
	}
	sets = append(sets, "updated_at = now()")
	params = append(params, tenantID, slug)
	q := fmt.Sprintf("UPDATE openrow.dashboards SET %s WHERE tenant_id = $%d AND slug = $%d",
		strings.Join(sets, ", "), idx, idx+1)
	tag, err := s.pool.Exec(ctx, q, params...)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return s.Get(ctx, tenantID, slug)
}

func (s *Service) Delete(ctx context.Context, tenantID, slug string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM openrow.dashboards
		WHERE tenant_id = $1 AND slug = $2`, tenantID, slug)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AddReport appends a report to an existing dashboard.
func (s *Service) AddReport(ctx context.Context, tenantID, slug string, in CreateReportInput) (*Report, error) {
	if err := validateReportInput(in); err != nil {
		return nil, err
	}
	dash, err := s.Get(ctx, tenantID, slug)
	if err != nil {
		return nil, err
	}
	width := in.Width
	if width <= 0 || width > 12 {
		width = 6
	}
	spec, _ := json.Marshal(in.QuerySpec)
	opts, _ := json.Marshal(marshalOptions(in.Options))
	var r Report
	var wt string
	var specBytes, optsBytes []byte
	err = s.pool.QueryRow(ctx, `
		INSERT INTO openrow.reports (dashboard_id, title, subtitle, widget_type, query_spec, options, width, position)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, dashboard_id, title, COALESCE(subtitle, ''), widget_type, query_spec, options, width, position, created_at, updated_at`,
		dash.ID, in.Title, nullIfEmpty(in.Subtitle), string(in.WidgetType), spec, opts, width, len(dash.Reports),
	).Scan(&r.ID, &r.DashboardID, &r.Title, &r.Subtitle, &wt, &specBytes, &optsBytes,
		&r.Width, &r.Position, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	r.WidgetType = WidgetType(wt)
	_ = json.Unmarshal(specBytes, &r.QuerySpec)
	r.Options = map[string]interface{}{}
	if len(optsBytes) > 0 {
		_ = json.Unmarshal(optsBytes, &r.Options)
	}
	return &r, nil
}

// UpdateReport replaces mutable fields of a report.
type UpdateReportInput struct {
	Title      *string                 `json:"title,omitempty"`
	Subtitle   *string                 `json:"subtitle,omitempty"`
	WidgetType *WidgetType             `json:"widget_type,omitempty"`
	QuerySpec  *QuerySpec              `json:"query_spec,omitempty"`
	Options    *map[string]interface{} `json:"options,omitempty"`
	Width      *int                    `json:"width,omitempty"`
}

func (s *Service) UpdateReport(ctx context.Context, tenantID, reportID string, in UpdateReportInput) error {
	// Make sure the report belongs to this tenant.
	var dashboardID string
	err := s.pool.QueryRow(ctx, `
		SELECT r.dashboard_id FROM openrow.reports r
		JOIN openrow.dashboards d ON d.id = r.dashboard_id
		WHERE r.id = $1 AND d.tenant_id = $2`,
		reportID, tenantID,
	).Scan(&dashboardID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("report not found")
	}
	if err != nil {
		return err
	}

	sets := []string{}
	params := []interface{}{}
	idx := 1
	if in.Title != nil {
		sets = append(sets, fmt.Sprintf("title = $%d", idx))
		params = append(params, *in.Title)
		idx++
	}
	if in.Subtitle != nil {
		sets = append(sets, fmt.Sprintf("subtitle = $%d", idx))
		params = append(params, nullIfEmpty(*in.Subtitle))
		idx++
	}
	if in.WidgetType != nil {
		if !in.WidgetType.Valid() {
			return fmt.Errorf("invalid widget_type")
		}
		sets = append(sets, fmt.Sprintf("widget_type = $%d", idx))
		params = append(params, string(*in.WidgetType))
		idx++
	}
	if in.QuerySpec != nil {
		if err := in.QuerySpec.Validate(); err != nil {
			return err
		}
		b, _ := json.Marshal(*in.QuerySpec)
		sets = append(sets, fmt.Sprintf("query_spec = $%d", idx))
		params = append(params, b)
		idx++
	}
	if in.Width != nil {
		w := *in.Width
		if w <= 0 || w > 12 {
			w = 6
		}
		sets = append(sets, fmt.Sprintf("width = $%d", idx))
		params = append(params, w)
		idx++
	}
	if in.Options != nil {
		b, _ := json.Marshal(marshalOptions(*in.Options))
		sets = append(sets, fmt.Sprintf("options = $%d", idx))
		params = append(params, b)
		idx++
	}
	if len(sets) == 0 {
		return nil
	}
	sets = append(sets, "updated_at = now()")
	params = append(params, reportID)
	q := fmt.Sprintf("UPDATE openrow.reports SET %s WHERE id = $%d", strings.Join(sets, ", "), idx)
	_, err = s.pool.Exec(ctx, q, params...)
	return err
}

// ReorderReports sets each report's position to its index in ids. All ids must
// belong to the same dashboard in the given tenant.
func (s *Service) ReorderReports(ctx context.Context, tenantID, slug string, ids []string) error {
	d, err := s.Get(ctx, tenantID, slug)
	if err != nil {
		return err
	}
	existing := map[string]bool{}
	for _, r := range d.Reports {
		existing[r.ID] = true
	}
	if len(ids) != len(d.Reports) {
		return fmt.Errorf("reorder: expected %d ids, got %d", len(d.Reports), len(ids))
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !existing[id] {
			return fmt.Errorf("reorder: report %s not on this dashboard", id)
		}
		if seen[id] {
			return fmt.Errorf("reorder: duplicate id %s", id)
		}
		seen[id] = true
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for i, id := range ids {
		if _, err := tx.Exec(ctx,
			`UPDATE openrow.reports SET position = $1, updated_at = now() WHERE id = $2`, i, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Service) DeleteReport(ctx context.Context, tenantID, reportID string) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM openrow.reports r
		USING openrow.dashboards d
		WHERE r.id = $1 AND d.tenant_id = $2 AND r.dashboard_id = d.id`,
		reportID, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("report not found")
	}
	return nil
}

// GetReport returns a single report plus the parent dashboard id for scoping checks.
func (s *Service) GetReport(ctx context.Context, tenantID, reportID string) (*Report, error) {
	var r Report
	var wt string
	var spec, opts []byte
	err := s.pool.QueryRow(ctx, `
		SELECT r.id, r.dashboard_id, r.title, COALESCE(r.subtitle, ''), r.widget_type, r.query_spec, r.options, r.width, r.position, r.created_at, r.updated_at
		FROM openrow.reports r
		JOIN openrow.dashboards d ON d.id = r.dashboard_id
		WHERE r.id = $1 AND d.tenant_id = $2`,
		reportID, tenantID,
	).Scan(&r.ID, &r.DashboardID, &r.Title, &r.Subtitle, &wt, &spec, &opts,
		&r.Width, &r.Position, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("report not found")
	}
	if err != nil {
		return nil, err
	}
	r.WidgetType = WidgetType(wt)
	if err := json.Unmarshal(spec, &r.QuerySpec); err != nil {
		return nil, err
	}
	r.Options = map[string]interface{}{}
	if len(opts) > 0 {
		_ = json.Unmarshal(opts, &r.Options)
	}
	return &r, nil
}
