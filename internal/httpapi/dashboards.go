package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/openrow/openrow/internal/auth"
	"github.com/openrow/openrow/internal/entities"
	"github.com/openrow/openrow/internal/reports"
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

type patchDashboardReq struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

func (s *Server) patchDashboard(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	var in patchDashboardReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	d, err := s.dashboards.UpdateDashboard(r.Context(), m.TenantID, r.PathValue("slug"),
		reports.UpdateDashboardInput{Name: in.Name, Description: in.Description})
	if err != nil {
		if errors.Is(err, reports.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "dashboard not found")
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
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

type reorderReq struct {
	ReportIDs []string `json:"report_ids"`
}

func (s *Server) reorderReports(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	var in reorderReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.dashboards.ReorderReports(r.Context(), m.TenantID, r.PathValue("slug"), in.ReportIDs); err != nil {
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
	Title      *string                 `json:"title,omitempty"`
	Subtitle   *string                 `json:"subtitle,omitempty"`
	WidgetType *reports.WidgetType     `json:"widget_type,omitempty"`
	QuerySpec  *reports.QuerySpec      `json:"query_spec,omitempty"`
	Options    *map[string]interface{} `json:"options,omitempty"`
	Width      *int                    `json:"width,omitempty"`
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
		Options:    in.Options,
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
// Supports ?from=<rfc3339>&to=<rfc3339> to inject filters on the report's
// date_filter_field (if set). Reports without date_filter_field ignore the range.
// KPI reports with compare_period also get a previous-window value attached.
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

	q := r.URL.Query()
	from := parseTimeQ(q.Get("from"))
	to := parseTimeQ(q.Get("to"))

	result, err := s.runWithRange(r, ent, &report.QuerySpec, from, to)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// KPI comparison: when compare_period is set + an actual range is in play,
	// run the aggregate once more on the shifted window and attach previous_value.
	if report.WidgetType == reports.WidgetKPI &&
		report.QuerySpec.ComparePeriod != "" &&
		report.QuerySpec.DateFilterField != "" &&
		from != nil && to != nil &&
		len(result.Rows) > 0 {
		prevFrom, prevTo := shiftRange(*from, *to, report.QuerySpec.ComparePeriod)
		prevResult, pErr := s.runWithRange(r, ent, &report.QuerySpec, &prevFrom, &prevTo)
		if pErr == nil && len(prevResult.Rows) > 0 {
			result.Rows[0]["previous_value"] = prevResult.Rows[0]["value"]
			result.Rows[0]["compare_period"] = report.QuerySpec.ComparePeriod
			result.Rows[0]["previous_from"] = prevFrom.Format(time.RFC3339)
			result.Rows[0]["previous_to"] = prevTo.Format(time.RFC3339)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"result": result})
}

// runWithRange runs the spec with optional from/to filters injected on the report's date_filter_field.
func (s *Server) runWithRange(r *http.Request, ent *entities.Entity, base *reports.QuerySpec, from, to *time.Time) (*reports.Result, error) {
	m, _ := auth.MembershipFromContext(r.Context())
	spec := *base
	spec.Filters = append([]reports.Filter(nil), spec.Filters...)
	if spec.DateFilterField != "" {
		if from != nil {
			v, _ := json.Marshal(from.Format(time.RFC3339))
			spec.Filters = append(spec.Filters, reports.Filter{
				Field: spec.DateFilterField, Op: reports.OpGte, Value: v,
			})
		}
		if to != nil {
			v, _ := json.Marshal(to.Format(time.RFC3339))
			spec.Filters = append(spec.Filters, reports.Filter{
				Field: spec.DateFilterField, Op: reports.OpLt, Value: v,
			})
		}
	}
	return s.reportExec.Execute(r.Context(), m.PGSchema, m.TenantID, ent, &spec)
}

// shiftRange returns the comparison window for a compare_period setting.
// "previous_period" = shift back by the window's length; "previous_year" = shift back 1 year.
func shiftRange(from, to time.Time, period string) (time.Time, time.Time) {
	switch period {
	case "previous_year":
		return from.AddDate(-1, 0, 0), to.AddDate(-1, 0, 0)
	case "previous_period":
		d := to.Sub(from)
		return from.Add(-d), from
	}
	return from, to
}

// parseTimeQ accepts RFC3339 or YYYY-MM-DD. Returns nil for empty or unparseable input.
func parseTimeQ(s string) *time.Time {
	if s == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return &t
	}
	return nil
}
