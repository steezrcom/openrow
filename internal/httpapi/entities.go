package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/steezrcom/steezr-erp/internal/auth"
	"github.com/steezrcom/steezr-erp/internal/entities"
)

func (s *Server) listEntities(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	es, err := s.entities.List(r.Context(), m.TenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entities": entitiesDTO(es)})
}

type proposeReq struct {
	Description string `json:"description"`
}

func (s *Server) proposeEntity(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	var req proposeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Description == "" {
		writeErr(w, http.StatusBadRequest, "description is required")
		return
	}
	existing, err := s.entities.List(r.Context(), m.TenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	spec, err := s.proposer.Propose(r.Context(), req.Description, existing)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "proposer: "+err.Error())
		return
	}
	ent, err := s.entities.Create(r.Context(), m.TenantID, m.PGSchema, spec)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"entity": entityDTO(ent)})
}

func (s *Server) createEntityFromSpec(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	var spec entities.EntitySpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	ent, err := s.entities.Create(r.Context(), m.TenantID, m.PGSchema, &spec)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"entity": entityDTO(ent)})
}

func (s *Server) getEntity(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	ent, err := s.entities.Get(r.Context(), m.TenantID, r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "entity not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entity": entityDTO(ent)})
}

func (s *Server) listRows(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	ent, err := s.entities.Get(r.Context(), m.TenantID, r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "entity not found")
		return
	}
	rows, err := s.entities.ListRows(r.Context(), m.PGSchema, ent, 200, 0)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	refOpts, err := s.loadRefOptions(r, ent, m.PGSchema, m.TenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"entity":      entityDTO(ent),
		"rows":        rows,
		"ref_options": refOpts,
	})
}

func (s *Server) loadRefOptions(r *http.Request, ent *entities.Entity, schema, tenantID string) (map[string][]entities.RefOption, error) {
	out := map[string][]entities.RefOption{}
	for _, f := range ent.Fields {
		if f.DataType != entities.TypeReference || f.ReferenceEntity == "" {
			continue
		}
		target, err := s.entities.Get(r.Context(), tenantID, f.ReferenceEntity)
		if err != nil {
			continue
		}
		opts, err := s.entities.ListRefOptions(r.Context(), schema, target)
		if err != nil {
			return nil, err
		}
		out[f.Name] = opts
	}
	return out, nil
}

type createRowReq struct {
	Values map[string]string `json:"values"`
}

func (s *Server) createRow(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	ent, err := s.entities.Get(r.Context(), m.TenantID, r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "entity not found")
		return
	}
	var req createRowReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	id, err := s.entities.InsertRow(r.Context(), m.PGSchema, ent, req.Values)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) deleteRow(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	ent, err := s.entities.Get(r.Context(), m.TenantID, r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "entity not found")
		return
	}
	if err := s.entities.DeleteRow(r.Context(), m.PGSchema, ent, r.PathValue("id")); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "row not found")
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
