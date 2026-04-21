package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/steezrcom/steezr-erp/internal/auth"
	"github.com/steezrcom/steezr-erp/internal/reports"
)

func (s *Server) listDashboards(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	ds, err := s.dashboards.List(r.Context(), m.TenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"dashboards": ds})
}

func (s *Server) createDashboard(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	var in reports.CreateDashboardInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	d, err := s.dashboards.Create(r.Context(), m.TenantID, in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"dashboard": d})
}

func (s *Server) getDashboard(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	d, err := s.dashboards.Get(r.Context(), m.TenantID, r.PathValue("slug"))
	if err != nil {
		if errors.Is(err, reports.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "dashboard not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"dashboard": d})
}

func (s *Server) deleteDashboard(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	if err := s.dashboards.Delete(r.Context(), m.TenantID, r.PathValue("slug")); err != nil {
		if errors.Is(err, reports.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "dashboard not found")
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) addReport(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	var in reports.CreateReportInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	report, err := s.dashboards.AddReport(r.Context(), m.TenantID, r.PathValue("slug"), in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"report": report})
}

type updateReportReq struct {
	Title      *string             `json:"title,omitempty"`
	Subtitle   *string             `json:"subtitle,omitempty"`
	WidgetType *reports.WidgetType `json:"widget_type,omitempty"`
	QuerySpec  *reports.QuerySpec  `json:"query_spec,omitempty"`
	Width      *int                `json:"width,omitempty"`
}

func (s *Server) patchReport(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	var in updateReportReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	err := s.dashboards.UpdateReport(r.Context(), m.TenantID, r.PathValue("id"), reports.UpdateReportInput{
		Title:      in.Title,
		Subtitle:   in.Subtitle,
		WidgetType: in.WidgetType,
		QuerySpec:  in.QuerySpec,
		Width:      in.Width,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteReport(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	if err := s.dashboards.DeleteReport(r.Context(), m.TenantID, r.PathValue("id")); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// executeReport runs the report's query and returns normalized results.
func (s *Server) executeReport(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	report, err := s.dashboards.GetReport(r.Context(), m.TenantID, r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	ent, err := s.entities.Get(r.Context(), m.TenantID, report.QuerySpec.Entity)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "entity "+report.QuerySpec.Entity+" referenced by report no longer exists")
		return
	}
	result, err := s.reportExec.Execute(r.Context(), m.PGSchema, m.TenantID, ent, &report.QuerySpec)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"result": result})
}
