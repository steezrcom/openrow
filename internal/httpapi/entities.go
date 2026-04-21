package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/openrow/openrow/internal/auth"
	"github.com/openrow/openrow/internal/entities"
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
	spec, err := s.proposer.Propose(r.Context(), m.TenantID, req.Description, existing)
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

	q := r.URL.Query()
	limit := parseIntDefault(q.Get("limit"), 50, 1, 200)
	page := parseIntDefault(q.Get("page"), 1, 1, 1_000_000)
	offset := (page - 1) * limit

	rows, err := s.entities.ListRows(r.Context(), m.PGSchema, ent, entities.ListOptions{
		Limit:   limit,
		Offset:  offset,
		SortBy:  q.Get("sort"),
		SortDir: q.Get("dir"),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	total, err := s.entities.CountRows(r.Context(), m.PGSchema, ent)
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
		"total":       total,
		"page":        page,
		"limit":       limit,
	})
}

func parseIntDefault(s string, def, min, max int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
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

type updateRowReq struct {
	Values map[string]string `json:"values"`
}

func (s *Server) addField(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	var f entities.FieldSpec
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	ent, err := s.entities.AddField(r.Context(), m.TenantID, m.PGSchema, r.PathValue("name"), f)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"entity": entityDTO(ent)})
}

// listFieldOptions returns label/id pairs for a reference field's target entity.
// Used by filter widgets to render a dropdown instead of a raw UUID input.
// Returns an empty options list for non-reference fields.
func (s *Server) listFieldOptions(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	ent, err := s.entities.Get(r.Context(), m.TenantID, r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "entity not found")
		return
	}
	fieldName := r.PathValue("field")
	var field *entities.Field
	for i := range ent.Fields {
		if ent.Fields[i].Name == fieldName {
			field = &ent.Fields[i]
			break
		}
	}
	if field == nil {
		writeErr(w, http.StatusNotFound, "field not found")
		return
	}
	if field.DataType != entities.TypeReference || field.ReferenceEntity == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{"options": []interface{}{}})
		return
	}
	target, err := s.entities.Get(r.Context(), m.TenantID, field.ReferenceEntity)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "target entity missing")
		return
	}
	opts, err := s.entities.ListRefOptions(r.Context(), m.PGSchema, target)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"options": opts})
}

func (s *Server) dropField(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	err := s.entities.DropField(r.Context(), m.TenantID, m.PGSchema,
		r.PathValue("name"), r.PathValue("field"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) updateRow(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	ent, err := s.entities.Get(r.Context(), m.TenantID, r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "entity not found")
		return
	}
	var req updateRowReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.entities.UpdateRow(r.Context(), m.PGSchema, ent, r.PathValue("id"), req.Values); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "row not found")
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
