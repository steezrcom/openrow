package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/openrow/openrow/internal/auth"
	"github.com/openrow/openrow/internal/flows"
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
	if err := s.seedFlows(r, m.TenantID, t.FlowSeeds); err != nil {
		// Entities landed successfully; surface flow-seed errors as 200
		// with a warning so the caller can reapply only the flows later.
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":      true,
			"warning": "entities installed but some flows failed: " + err.Error(),
		})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// seedFlows converts the template's FlowSeeds into CreateFlowInput and
// creates them. Name conflicts are ignored (the workspace likely has the
// flow from a previous apply); other errors bubble up.
func (s *Server) seedFlows(r *http.Request, tenantID string, seeds []templates.FlowSeed) error {
	if len(seeds) == 0 || s.flows == nil {
		return nil
	}
	for _, seed := range seeds {
		cfg, err := json.Marshal(seed.TriggerConfig)
		if err != nil {
			return err
		}
		if _, err := s.flows.Create(r.Context(), tenantID, flows.CreateFlowInput{
			Name:          seed.Name,
			Description:   seed.Description,
			Goal:          seed.Goal,
			TriggerKind:   flows.TriggerKind(seed.TriggerKind),
			TriggerConfig: cfg,
			ToolAllowlist: seed.ToolAllowlist,
			Mode:          flows.Mode(seed.Mode),
		}); err != nil {
			// Duplicate name is the common "already seeded" signal; skip it.
			if isDuplicateFlowName(err) {
				continue
			}
			return err
		}
	}
	return nil
}

func isDuplicateFlowName(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique") || errors.Is(err, flows.ErrNotFound)
}
