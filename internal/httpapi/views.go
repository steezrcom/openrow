package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openrow/openrow/internal/auth"
	"github.com/openrow/openrow/internal/entities"
)

func (s *Server) listViews(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	views, err := s.entities.ListViews(r.Context(), m.TenantID, r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"views": views})
}

type createViewReq struct {
	Name     string              `json:"name"`
	ViewType entities.ViewType   `json:"view_type"`
	Config   json.RawMessage     `json:"config,omitempty"`
}

func (s *Server) createView(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	u, _, _ := auth.FromContext(r.Context())
	var in createViewReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	userID := ""
	if u != nil {
		userID = u.ID
	}
	v, err := s.entities.CreateView(r.Context(), m.TenantID, entities.CreateViewInput{
		EntityName: r.PathValue("name"),
		Name:       in.Name,
		ViewType:   in.ViewType,
		Config:     in.Config,
		UserID:     userID,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"view": v})
}

type patchViewReq struct {
	Name     *string             `json:"name,omitempty"`
	ViewType *entities.ViewType  `json:"view_type,omitempty"`
	Config   *json.RawMessage    `json:"config,omitempty"`
	Position *int                `json:"position,omitempty"`
}

func (s *Server) patchView(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	var in patchViewReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	v, err := s.entities.UpdateView(r.Context(), m.TenantID, r.PathValue("id"), entities.UpdateViewInput{
		Name:     in.Name,
		ViewType: in.ViewType,
		Config:   in.Config,
		Position: in.Position,
	})
	if err != nil {
		if errors.Is(err, entities.ErrViewNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"view": v})
}

func (s *Server) deleteView(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	if err := s.entities.DeleteView(r.Context(), m.TenantID, r.PathValue("id")); err != nil {
		if errors.Is(err, entities.ErrViewNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
