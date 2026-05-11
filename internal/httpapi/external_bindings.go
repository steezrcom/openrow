package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/openrow/openrow/internal/auth"
	"github.com/openrow/openrow/internal/connectors"
	"github.com/openrow/openrow/internal/connectors/catalog/flexi"
	"github.com/openrow/openrow/internal/connectors/discovery"
	"github.com/openrow/openrow/internal/llm"
)

type ExternalBindings struct {
	Log      *slog.Logger
	Repo     *discovery.Repo
	Conns    *connectors.Service
	LLM      *llm.Service
	AllowInt bool
}

func (h *ExternalBindings) Mount(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/connectors/external-bindings", auth.RequireMembership(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/v1/connectors/external-bindings", auth.RequireMembership(http.HandlerFunc(h.create)))
	mux.Handle("GET /api/v1/connectors/external-bindings/{id}", auth.RequireMembership(http.HandlerFunc(h.get)))
	mux.Handle("GET /api/v1/connectors/external-bindings/{id}/mapping", auth.RequireMembership(http.HandlerFunc(h.getMapping)))
	mux.Handle("PATCH /api/v1/connectors/external-bindings/{id}/mapping", auth.RequireMembership(http.HandlerFunc(h.patchMapping)))
	mux.Handle("GET /api/v1/connectors/external-bindings/{id}/review-items", auth.RequireMembership(http.HandlerFunc(h.listReview)))
	mux.Handle("POST /api/v1/connectors/external-bindings/{id}/review-items/{item}/resolve", auth.RequireMembership(http.HandlerFunc(h.resolveReview)))
	mux.Handle("POST /api/v1/connectors/external-bindings/{id}/activate", auth.RequireMembership(http.HandlerFunc(h.activate)))
	mux.Handle("POST /api/v1/connectors/external-bindings/{id}/rediscover", auth.RequireMembership(http.HandlerFunc(h.rediscover)))
}

type createBindingIn struct {
	ConnectorID       string            `json:"connector_id"`
	Credentials       map[string]string `json:"credentials"`
	LLMClassification *bool             `json:"llm_classification,omitempty"`
}

func (h *ExternalBindings) list(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	out, err := h.Repo.ListByTenant(r.Context(), m.TenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"bindings": out})
}

func (h *ExternalBindings) create(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	var in createBindingIn
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if in.ConnectorID != flexi.ConnectorID {
		writeErr(w, http.StatusBadRequest, "unsupported connector")
		return
	}
	fields := make(map[string]*string, len(in.Credentials))
	for k, v := range in.Credentials {
		s := v
		fields[k] = &s
	}
	if _, err := h.Conns.Upsert(r.Context(), m.TenantID, in.ConnectorID, connectors.UpsertInput{Fields: fields}); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	b, err := h.Repo.Create(r.Context(), m.TenantID, in.ConnectorID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	go h.runDiscovery(context.Background(), m.TenantID, b, in.Credentials)
	writeJSON(w, http.StatusAccepted, map[string]any{"binding": b})
}

func (h *ExternalBindings) runDiscovery(ctx context.Context, tenantID string, b *discovery.Binding, creds map[string]string) {
	fetcher, err := flexi.NewFetcher(creds, h.AllowInt)
	if err != nil {
		h.discoveryFailed(ctx, b.ID, err)
		return
	}
	svc := &discovery.Service{
		Fetcher:       fetcher,
		Heuristic:     flexi.NewHeuristic(),
		Classifier:    &discovery.WorkspaceClassifier{LLM: h.LLM, TenantID: tenantID},
		PromptBuilder: flexi.BuildPrompt,
	}
	art, items, err := svc.Run(ctx, b)
	if err != nil {
		h.discoveryFailed(ctx, b.ID, err)
		return
	}
	if err := h.Repo.SaveMapping(ctx, b.ID, art); err != nil {
		h.discoveryFailed(ctx, b.ID, err)
		return
	}
	if err := h.Repo.ReplaceReviewItems(ctx, b.ID, items); err != nil {
		h.discoveryFailed(ctx, b.ID, err)
		return
	}
	if err := h.Repo.UpdateState(ctx, b.ID, discovery.StateProposed, ""); err != nil && h.Log != nil {
		h.Log.Error("discovery: update state proposed", "binding_id", b.ID, "err", err)
	}
}

func (h *ExternalBindings) discoveryFailed(ctx context.Context, bindingID string, err error) {
	if h.Log != nil {
		h.Log.Error("discovery failed", "binding_id", bindingID, "err", err)
	}
	if uerr := h.Repo.UpdateState(ctx, bindingID, discovery.StateError, err.Error()); uerr != nil && h.Log != nil {
		h.Log.Error("discovery: update state error", "binding_id", bindingID, "err", uerr)
	}
}

func (h *ExternalBindings) get(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	b, err := h.Repo.GetByID(r.Context(), m.TenantID, r.PathValue("id"))
	if errors.Is(err, discovery.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "binding not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"binding": b})
}

func (h *ExternalBindings) getMapping(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	b, err := h.Repo.GetByID(r.Context(), m.TenantID, r.PathValue("id"))
	if errors.Is(err, discovery.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "binding not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mapping": b.Mapping})
}

type patchMappingReq struct {
	Evidence string         `json:"evidence"`
	Field    string         `json:"field"`
	Role     discovery.Role `json:"role"`
	Mirror   *bool          `json:"mirror,omitempty"`
}

func (h *ExternalBindings) patchMapping(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	b, err := h.Repo.GetByID(r.Context(), m.TenantID, r.PathValue("id"))
	if errors.Is(err, discovery.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "binding not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var patch patchMappingReq
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if patch.Evidence == "" {
		writeErr(w, http.StatusBadRequest, "evidence is required")
		return
	}
	if b.Mapping == nil {
		writeErr(w, http.StatusBadRequest, "no mapping yet")
		return
	}
	ev := b.Mapping.Evidences[patch.Evidence]
	if ev == nil {
		writeErr(w, http.StatusBadRequest, "unknown evidence")
		return
	}
	if patch.Field != "" {
		f := ev.Fields[patch.Field]
		if f == nil {
			writeErr(w, http.StatusBadRequest, "unknown field")
			return
		}
		if patch.Role != "" {
			f.Role = patch.Role
			f.Source = discovery.SourceUser
			f.Confidence = 1.0
			f.Review = discovery.ReviewAuto
		}
	} else {
		if patch.Role != "" {
			ev.Role = patch.Role
			ev.Source = discovery.SourceUser
			ev.Confidence = 1.0
			ev.Review = discovery.ReviewAuto
		}
		if patch.Mirror != nil {
			ev.Mirror = *patch.Mirror
		}
	}
	if err := h.Repo.SaveMapping(r.Context(), b.ID, b.Mapping); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"evidence": b.Mapping.Evidences[patch.Evidence]})
}

func (h *ExternalBindings) listReview(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	bindingID := r.PathValue("id")
	if _, err := h.Repo.GetByID(r.Context(), m.TenantID, bindingID); err != nil {
		if errors.Is(err, discovery.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "binding not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	items, err := h.Repo.ListReviewItems(r.Context(), bindingID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *ExternalBindings) resolveReview(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	bindingID := r.PathValue("id")
	itemID := r.PathValue("item")
	if _, err := h.Repo.GetByID(r.Context(), m.TenantID, bindingID); err != nil {
		if errors.Is(err, discovery.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "binding not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var in struct {
		Action string         `json:"action"`
		Patch  map[string]any `json:"patch,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := h.Repo.ResolveReviewItem(r.Context(), itemID, in.Action); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ExternalBindings) activate(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	bindingID := r.PathValue("id")
	b, err := h.Repo.GetByID(r.Context(), m.TenantID, bindingID)
	if errors.Is(err, discovery.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "binding not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if b.State != discovery.StateProposed {
		writeErr(w, http.StatusBadRequest, "binding is not in 'proposed' state")
		return
	}
	if err := h.Repo.UpdateState(r.Context(), b.ID, discovery.StateActive, ""); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"state": string(discovery.StateActive)})
}

func (h *ExternalBindings) rediscover(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	b, err := h.Repo.GetByID(r.Context(), m.TenantID, r.PathValue("id"))
	if errors.Is(err, discovery.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "binding not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	cfg, err := h.Conns.Get(r.Context(), m.TenantID, b.ConnectorID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "credentials missing")
		return
	}
	if err := h.Repo.UpdateState(r.Context(), b.ID, discovery.StateDiscovering, ""); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	go h.runDiscovery(context.Background(), m.TenantID, b, cfg.Credentials)
	writeJSON(w, http.StatusAccepted, map[string]string{"state": string(discovery.StateDiscovering)})
}
