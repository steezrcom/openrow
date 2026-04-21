package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openrow/openrow/internal/auth"
	"github.com/openrow/openrow/internal/flows"
)

// --- tool discovery ------------------------------------------------------

type toolInfoDTO struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Mutates     bool   `json:"mutates"`
}

// listFlowTools returns the set of tools available for flow authoring in
// this workspace. Same toolset the chat agent has, with Mutates metadata
// so the UI can render "write" vs "read" badges.
func (s *Server) listFlowTools(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	ts := s.agent.BuildToolset(r.Context(), m.TenantID, m.PGSchema)
	out := make([]toolInfoDTO, 0, len(ts.Tools()))
	for _, t := range ts.Tools() {
		out = append(out, toolInfoDTO{Name: t.Name, Description: t.Description, Mutates: t.Mutates})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": out})
}

// --- flows ---------------------------------------------------------------

type createFlowReq struct {
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	Goal          string             `json:"goal"`
	TriggerKind   flows.TriggerKind  `json:"trigger_kind"`
	TriggerConfig json.RawMessage    `json:"trigger_config,omitempty"`
	ToolAllowlist []string           `json:"tool_allowlist"`
	Mode          flows.Mode         `json:"mode"`
}

func (s *Server) listFlows(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	list, err := s.flows.List(r.Context(), m.TenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"flows": list})
}

func (s *Server) createFlow(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	u, _, _ := auth.FromContext(r.Context())
	var in createFlowReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	userID := ""
	if u != nil {
		userID = u.ID
	}
	res, err := s.flows.Create(r.Context(), m.TenantID, flows.CreateFlowInput{
		Name:          in.Name,
		Description:   in.Description,
		Goal:          in.Goal,
		TriggerKind:   defaultTriggerKind(in.TriggerKind),
		TriggerConfig: in.TriggerConfig,
		ToolAllowlist: in.ToolAllowlist,
		Mode:          defaultMode(in.Mode),
		CreatedByUser: userID,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	resp := map[string]any{"flow": res.Flow}
	if res.WebhookTokenOnce != "" {
		resp["webhook_url"] = webhookURL(r, m.TenantSlug, res.Flow.ID, res.WebhookTokenOnce)
		resp["webhook_token_once"] = res.WebhookTokenOnce
	}
	writeJSON(w, http.StatusOK, resp)
}

// rotateWebhookToken regenerates a flow's webhook token. Returns the new
// plaintext — it won't be retrievable later.
func (s *Server) rotateFlowWebhookToken(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	flow, err := s.flows.Get(r.Context(), m.TenantID, r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if flow.TriggerKind != flows.TriggerWebhook {
		writeErr(w, http.StatusBadRequest, "flow is not webhook-triggered")
		return
	}
	token, err := s.flows.RotateWebhookToken(r.Context(), m.TenantID, flow.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"webhook_url":        webhookURL(r, m.TenantSlug, flow.ID, token),
		"webhook_token_once": token,
	})
}

func webhookURL(r *http.Request, tenantSlug, flowID, token string) string {
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") == "" {
		scheme = "http"
	}
	return scheme + "://" + r.Host + "/webhooks/" + tenantSlug + "/" + flowID + "?token=" + token
}

func defaultMode(m flows.Mode) flows.Mode {
	if m == "" {
		return flows.ModeDryRun
	}
	return m
}

func defaultTriggerKind(t flows.TriggerKind) flows.TriggerKind {
	if t == "" {
		return flows.TriggerManual
	}
	return t
}

func (s *Server) getFlow(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	f, err := s.flows.Get(r.Context(), m.TenantID, r.PathValue("id"))
	if err != nil {
		if errors.Is(err, flows.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"flow": f})
}

type patchFlowReq struct {
	Name          *string            `json:"name,omitempty"`
	Description   *string            `json:"description,omitempty"`
	Goal          *string            `json:"goal,omitempty"`
	TriggerKind   *flows.TriggerKind `json:"trigger_kind,omitempty"`
	TriggerConfig *json.RawMessage   `json:"trigger_config,omitempty"`
	ToolAllowlist *[]string          `json:"tool_allowlist,omitempty"`
	Mode          *flows.Mode        `json:"mode,omitempty"`
	Enabled       *bool              `json:"enabled,omitempty"`
}

func (s *Server) patchFlow(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	var in patchFlowReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	f, err := s.flows.Update(r.Context(), m.TenantID, r.PathValue("id"), flows.UpdateFlowInput{
		Name: in.Name, Description: in.Description, Goal: in.Goal,
		TriggerKind: in.TriggerKind, TriggerConfig: in.TriggerConfig,
		ToolAllowlist: in.ToolAllowlist, Mode: in.Mode, Enabled: in.Enabled,
	})
	if err != nil {
		if errors.Is(err, flows.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"flow": f})
}

func (s *Server) deleteFlow(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	if err := s.flows.Delete(r.Context(), m.TenantID, r.PathValue("id")); err != nil {
		if errors.Is(err, flows.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// triggerFlow enqueues a manual run. The run executes on the dispatcher's
// worker pool; the handler returns immediately so the UI can poll.
func (s *Server) triggerFlow(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	f, err := s.flows.Get(r.Context(), m.TenantID, r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if !f.Enabled {
		writeErr(w, http.StatusBadRequest, "flow is disabled")
		return
	}
	run, err := s.flowDispatcher.Dispatch(r.Context(), f, json.RawMessage(`{"kind":"manual"}`))
	if err != nil {
		if run != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"run": run, "error": err.Error()})
			return
		}
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"run": run})
}

// --- runs ----------------------------------------------------------------

func (s *Server) listFlowRuns(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	runs, err := s.flows.ListRuns(r.Context(), m.TenantID, r.PathValue("id"), 50)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (s *Server) getFlowRun(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	run, err := s.flows.GetRun(r.Context(), m.TenantID, r.PathValue("run_id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	steps, err := s.flows.ListSteps(r.Context(), run.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run, "steps": steps})
}

// --- approvals -----------------------------------------------------------

func (s *Server) listFlowApprovals(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	list, err := s.flows.ListPendingApprovals(r.Context(), m.TenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"approvals": list})
}

type resolveApprovalReq struct {
	Approve         bool   `json:"approve"`
	RejectionReason string `json:"rejection_reason,omitempty"`
}

func (s *Server) resolveFlowApproval(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	u, _, _ := auth.FromContext(r.Context())
	var in resolveApprovalReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	userID := ""
	if u != nil {
		userID = u.ID
	}
	approval, err := s.flows.ResolveApproval(r.Context(), m.TenantID, r.PathValue("id"), userID, in.Approve, in.RejectionReason)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	run, err := s.flows.GetRun(r.Context(), m.TenantID, approval.RunID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	flow, err := s.flows.Get(r.Context(), m.TenantID, run.FlowID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	resumedRun, err := s.flowRunner.Resume(r.Context(), flow, run, approval)
	if err != nil {
		// The approval did resolve; resume failure returns the run for inspection.
		writeJSON(w, http.StatusOK, map[string]any{
			"approval": approval, "run": resumedRun, "error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"approval": approval, "run": resumedRun})
}
