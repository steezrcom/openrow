package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openrow/openrow/internal/auth"
	"github.com/openrow/openrow/internal/connectors"
)

// connectorDTO is the wire shape for the catalog — enriches the
// descriptor with capability flags that aren't otherwise introspectable
// from JSON (verifier, actions are func-valued).
type connectorDTO struct {
	*connectors.Connector
	HasVerifyWebhook bool `json:"has_verify_webhook"`
}

func (s *Server) listConnectors(w http.ResponseWriter, r *http.Request) {
	all := connectors.All()
	out := make([]connectorDTO, 0, len(all))
	for _, c := range all {
		out = append(out, connectorDTO{Connector: c, HasVerifyWebhook: c.VerifyWebhook != nil})
	}
	writeJSON(w, http.StatusOK, map[string]any{"connectors": out})
}

func (s *Server) listConnectorConfigs(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	configs, err := s.connectors.List(r.Context(), m.TenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]connectors.SafeConfig, 0, len(configs))
	for _, c := range configs {
		out = append(out, c.Safe(connectors.Get(c.ConnectorID)))
	}
	writeJSON(w, http.StatusOK, map[string]any{"configs": out})
}

type putConnectorConfigReq struct {
	Fields  map[string]*string `json:"fields"`
	Enabled *bool              `json:"enabled,omitempty"`
}

func (s *Server) putConnectorConfig(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	id := r.PathValue("id")
	if connectors.Get(id) == nil {
		writeErr(w, http.StatusNotFound, "unknown connector")
		return
	}

	var in putConnectorConfigReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	cfg, err := s.connectors.Upsert(r.Context(), m.TenantID, id, connectors.UpsertInput{
		Fields:  in.Fields,
		Enabled: in.Enabled,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"config": cfg.Safe(connectors.Get(id))})
}

func (s *Server) testConnectorConfig(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	id := r.PathValue("id")
	err := s.connectors.Test(r.Context(), m.TenantID, id)
	if errors.Is(err, connectors.ErrNotConfigured) {
		writeErr(w, http.StatusNotFound, "not configured")
		return
	}
	if errors.Is(err, connectors.ErrTestNotSupported) {
		writeErr(w, http.StatusBadRequest, "this connector does not support credential tests")
		return
	}
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) deleteConnectorConfig(w http.ResponseWriter, r *http.Request) {
	m, _ := auth.MembershipFromContext(r.Context())
	id := r.PathValue("id")
	if err := s.connectors.Delete(r.Context(), m.TenantID, id); err != nil {
		if errors.Is(err, connectors.ErrNotConfigured) {
			writeErr(w, http.StatusNotFound, "not configured")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
