package web

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strings"

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
	tmpl     *template.Template
}

func NewHandler(t *tenant.Service, e *entities.Service, p *ai.Proposer, log *slog.Logger) (*Handler, error) {
	tmpl, err := template.ParseFS(tmplFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return &Handler{tenants: t, entities: e, proposer: p, log: log, tmpl: tmpl}, nil
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.home)
	mux.HandleFunc("POST /tenants", h.createTenant)
	mux.HandleFunc("GET /t/{slug}", h.dashboard)
	mux.HandleFunc("POST /t/{slug}/entities", h.createEntity)
	mux.HandleFunc("GET /t/{slug}/entities/{name}", h.entityDetail)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

type viewData struct {
	Title    string
	Tenant   *tenant.Tenant
	Tenants  []tenant.Tenant
	Entities []entities.Entity
	Entity   *entities.Entity
	Error    string
}

func (h *Handler) render(w http.ResponseWriter, name string, data viewData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, name, data); err != nil {
		h.log.Error("render template", "name", name, "err", err)
	}
}

func (h *Handler) renderFragment(w http.ResponseWriter, name string, data viewData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, name, data); err != nil {
		h.log.Error("render fragment", "name", name, "err", err)
	}
}

func (h *Handler) home(w http.ResponseWriter, r *http.Request) {
	ts, err := h.tenants.List(r.Context())
	if err != nil {
		h.serverError(w, err)
		return
	}
	h.render(w, "layout", viewData{Title: "Tenants", Tenants: ts})
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
	h.render(w, "layout", viewData{
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
	h.renderFragment(w, "entities", viewData{Tenant: t, Entities: es})
}

func (h *Handler) entityDetail(w http.ResponseWriter, r *http.Request) {
	t, err := h.tenants.BySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		h.tenantOrServerError(w, err)
		return
	}
	e, err := h.entities.Get(r.Context(), t.ID, r.PathValue("name"))
	if err != nil {
		http.Error(w, "entity not found", http.StatusNotFound)
		return
	}
	h.render(w, "layout", viewData{
		Title: e.DisplayName, Tenant: t, Entity: e,
	})
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
