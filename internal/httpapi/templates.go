package httpapi

import (
	"net/http"

	"github.com/openrow/openrow/internal/auth"
	"github.com/openrow/openrow/internal/templates"
)

type templateDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request) {
	ts := templates.All()
	out := make([]templateDTO, len(ts))
	for i, t := range ts {
		out[i] = templateDTO{ID: t.ID, Name: t.Name, Description: t.Description}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"templates": out})
}

func (s *Server) applyTemplate(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	t, ok := templates.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "template not found")
		return
	}
	if err := t.Install(r.Context(), m.TenantID, m.PGSchema, s.entities, s.dashboards); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
