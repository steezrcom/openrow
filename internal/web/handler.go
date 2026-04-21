package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/steezrcom/steezr-erp/internal/ai"
	"github.com/steezrcom/steezr-erp/internal/entities"
	"github.com/steezrcom/steezr-erp/internal/tenant"
)

//go:embed templates/*.html
var tmplFS embed.FS

type Handler struct {
	tenants  *tenant.Service
	entities *entities.Service
	proposer *ai.Proposer
	log      *slog.Logger
	pages    map[string]*template.Template
}

func NewHandler(t *tenant.Service, e *entities.Service, p *ai.Proposer, log *slog.Logger) (*Handler, error) {
	pages, err := buildPages()
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return &Handler{tenants: t, entities: e, proposer: p, log: log, pages: pages}, nil
}

func buildPages() (map[string]*template.Template, error) {
	layout, err := tmplFS.ReadFile("templates/layout.html")
	if err != nil {
		return nil, err
	}
	out := map[string]*template.Template{}
	for _, name := range []string{"home", "dashboard", "entity"} {
		body, err := tmplFS.ReadFile("templates/" + name + ".html")
		if err != nil {
			return nil, err
		}
		t := template.New(name).Funcs(templateFuncs())
		if _, err := t.Parse(string(layout) + string(body)); err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		out[name] = t
	}
	return out, nil
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"display": displayValue,
		"ts":      displayTime,
	}
}

func displayValue(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	case time.Time:
		return x.Format("2006-01-02 15:04")
	case bool:
		if x {
			return "yes"
		}
		return ""
	default:
		return fmt.Sprintf("%v", x)
	}
}

func displayTime(v any) string {
	if t, ok := v.(time.Time); ok {
		return t.Format("2006-01-02 15:04")
	}
	return displayValue(v)
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.home)
	mux.HandleFunc("POST /tenants", h.createTenant)
	mux.HandleFunc("GET /t/{slug}", h.dashboard)
	mux.HandleFunc("POST /t/{slug}/entities", h.createEntity)
	mux.HandleFunc("POST /t/{slug}/entities/spec", h.createEntityFromSpec)
	mux.HandleFunc("GET /t/{slug}/entities/{name}", h.entityDetail)
	mux.HandleFunc("POST /t/{slug}/entities/{name}/rows", h.createRow)
	mux.HandleFunc("DELETE /t/{slug}/entities/{name}/rows/{id}", h.deleteRow)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

type viewData struct {
	Title      string
	Tenant     *tenant.Tenant
	Tenants    []tenant.Tenant
	Entities   []entities.Entity
	Entity     *entities.Entity
	Rows       []entities.Row
	RefOptions map[string][]entities.RefOption
	Error      string
}

func (h *Handler) render(w http.ResponseWriter, page string, data viewData) {
	tmpl, ok := h.pages[page]
	if !ok {
		h.log.Error("unknown page template", "page", page)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		h.log.Error("render page", "page", page, "err", err)
	}
}

func (h *Handler) renderFragment(w http.ResponseWriter, page, block string, data viewData) {
	tmpl, ok := h.pages[page]
	if !ok {
		h.log.Error("unknown page template", "page", page)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, block, data); err != nil {
		h.log.Error("render fragment", "page", page, "block", block, "err", err)
	}
}

func (h *Handler) home(w http.ResponseWriter, r *http.Request) {
	ts, err := h.tenants.List(r.Context())
	if err != nil {
		h.serverError(w, err)
		return
	}
	h.render(w, "home", viewData{Title: "Tenants", Tenants: ts})
}

func (h *Handler) createTenant(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("slug"))
	name := strings.TrimSpace(r.FormValue("name"))
	t, err := h.tenants.Create(r.Context(), slug, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/t/"+t.Slug, http.StatusSeeOther)
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	t, es, err := h.loadTenantEntities(r.Context(), r.PathValue("slug"))
	if err != nil {
		h.tenantOrServerError(w, err)
		return
	}
	h.render(w, "dashboard", viewData{
		Title: t.Name, Tenant: t, Entities: es,
	})
}

func (h *Handler) createEntity(w http.ResponseWriter, r *http.Request) {
	t, err := h.tenants.BySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		h.tenantOrServerError(w, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	description := strings.TrimSpace(r.FormValue("description"))
	if description == "" {
		http.Error(w, "description is required", http.StatusBadRequest)
		return
	}

	existing, err := h.entities.List(r.Context(), t.ID)
	if err != nil {
		h.serverError(w, err)
		return
	}
	spec, err := h.proposer.Propose(r.Context(), description, existing)
	if err != nil {
		h.log.Error("propose", "err", err)
		http.Error(w, "proposer: "+err.Error(), http.StatusBadGateway)
		return
	}
	if _, err := h.entities.Create(r.Context(), t.ID, t.PGSchema, spec); err != nil {
		h.log.Error("create entity", "err", err)
		http.Error(w, "create: "+err.Error(), http.StatusBadRequest)
		return
	}

	es, err := h.entities.List(r.Context(), t.ID)
	if err != nil {
		h.serverError(w, err)
		return
	}
	h.renderFragment(w, "dashboard", "entities", viewData{Tenant: t, Entities: es})
}

func (h *Handler) createEntityFromSpec(w http.ResponseWriter, r *http.Request) {
	t, err := h.tenants.BySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		h.tenantOrServerError(w, err)
		return
	}
	var spec entities.EntitySpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	e, err := h.entities.Create(r.Context(), t.ID, t.PGSchema, &spec)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":           e.ID,
		"name":         e.Name,
		"display_name": e.DisplayName,
	})
}

func (h *Handler) entityDetail(w http.ResponseWriter, r *http.Request) {
	data, err := h.loadEntityView(r.Context(), r.PathValue("slug"), r.PathValue("name"))
	if err != nil {
		h.tenantOrServerError(w, err)
		return
	}
	h.render(w, "entity", *data)
}

func (h *Handler) createRow(w http.ResponseWriter, r *http.Request) {
	t, err := h.tenants.BySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		h.tenantOrServerError(w, err)
		return
	}
	ent, err := h.entities.Get(r.Context(), t.ID, r.PathValue("name"))
	if err != nil {
		http.Error(w, "entity not found", http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	input := make(map[string]string, len(r.Form))
	for k, vs := range r.Form {
		if len(vs) > 0 {
			input[k] = vs[0]
		}
	}
	if _, err := h.entities.InsertRow(r.Context(), t.PGSchema, ent, input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.writeRowsFragment(w, r.Context(), t, ent)
}

func (h *Handler) deleteRow(w http.ResponseWriter, r *http.Request) {
	t, err := h.tenants.BySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		h.tenantOrServerError(w, err)
		return
	}
	ent, err := h.entities.Get(r.Context(), t.ID, r.PathValue("name"))
	if err != nil {
		http.Error(w, "entity not found", http.StatusNotFound)
		return
	}
	if err := h.entities.DeleteRow(r.Context(), t.PGSchema, ent, r.PathValue("id")); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.writeRowsFragment(w, r.Context(), t, ent)
}

func (h *Handler) loadEntityView(ctx context.Context, slug, name string) (*viewData, error) {
	t, err := h.tenants.BySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	ent, err := h.entities.Get(ctx, t.ID, name)
	if err != nil {
		return nil, err
	}
	refOpts, err := h.loadRefOptions(ctx, t, ent)
	if err != nil {
		return nil, err
	}
	rows, err := h.entities.ListRows(ctx, t.PGSchema, ent, 100, 0)
	if err != nil {
		return nil, err
	}
	return &viewData{
		Title:      ent.DisplayName,
		Tenant:     t,
		Entity:     ent,
		Rows:       rows,
		RefOptions: refOpts,
	}, nil
}

func (h *Handler) loadRefOptions(ctx context.Context, t *tenant.Tenant, ent *entities.Entity) (map[string][]entities.RefOption, error) {
	out := map[string][]entities.RefOption{}
	for _, f := range ent.Fields {
		if f.DataType != entities.TypeReference || f.ReferenceEntity == "" {
			continue
		}
		target, err := h.entities.Get(ctx, t.ID, f.ReferenceEntity)
		if err != nil {
			h.log.Warn("ref target missing", "field", f.Name, "target", f.ReferenceEntity, "err", err)
			continue
		}
		opts, err := h.entities.ListRefOptions(ctx, t.PGSchema, target)
		if err != nil {
			return nil, err
		}
		out[f.Name] = opts
	}
	return out, nil
}

func (h *Handler) writeRowsFragment(w http.ResponseWriter, ctx context.Context, t *tenant.Tenant, ent *entities.Entity) {
	rows, err := h.entities.ListRows(ctx, t.PGSchema, ent, 100, 0)
	if err != nil {
		h.serverError(w, err)
		return
	}
	h.renderFragment(w, "entity", "rows", viewData{Tenant: t, Entity: ent, Rows: rows})
}

func (h *Handler) loadTenantEntities(ctx context.Context, slug string) (*tenant.Tenant, []entities.Entity, error) {
	t, err := h.tenants.BySlug(ctx, slug)
	if err != nil {
		return nil, nil, err
	}
	es, err := h.entities.List(ctx, t.ID)
	if err != nil {
		return nil, nil, err
	}
	return t, es, nil
}

func (h *Handler) tenantOrServerError(w http.ResponseWriter, err error) {
	if errors.Is(err, tenant.ErrNotFound) {
		http.Error(w, "tenant not found", http.StatusNotFound)
		return
	}
	h.serverError(w, err)
}

func (h *Handler) serverError(w http.ResponseWriter, err error) {
	h.log.Error("server error", "err", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
