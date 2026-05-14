package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/openrow/openrow/internal/connectors"
)

// extractWebhookToken reads the per-flow auth token from the request.
// Header carriers are preferred; the query-string fallback stays for senders
// that can't set headers (Fio, some no-code platforms), but is logged so an
// operator can encourage migration. Reading from headers keeps the secret
// out of access logs and TLS-terminating proxy caches.
func extractWebhookToken(r *http.Request, log *slog.Logger, flowID string) string {
	if h := r.Header.Get("Authorization"); h != "" {
		if rest, ok := strings.CutPrefix(h, "Bearer "); ok {
			tok := strings.TrimSpace(rest)
			if tok != "" {
				return tok
			}
		}
	}
	if h := strings.TrimSpace(r.Header.Get("X-Openrow-Webhook-Token")); h != "" {
		return h
	}
	if q := r.URL.Query().Get("token"); q != "" {
		if log != nil {
			log.Warn("webhook token presented in query string; prefer Authorization header",
				"flow_id", flowID)
		}
		return q
	}
	return ""
}

// webhookReceive is the public endpoint flows hook up to external services.
// Path: /webhooks/{tenant_slug}/{flow_id}
// Token (in priority order):
//   1. Authorization: Bearer <token>
//   2. X-Openrow-Webhook-Token: <token>
//   3. ?token=<plaintext> (fallback for senders that can't set headers,
//      e.g. Fio's static webhook config; logged as a warning).
//
// Two auth layers:
//   1. Per-flow token (always required) — rejects unknown senders outright.
//   2. Connector signature (optional) — when the flow's trigger_config
//      includes a webhook_connector_id, the matching connector's
//      VerifyWebhook must approve the payload before it's dispatched.
//      This defends against a token-leak scenario where an attacker
//      knows the URL but can't sign payloads the connector would.
//
// Returns 200 as soon as the run is queued; external senders expect fast
// 2xx and retry on 5xx, so don't do anything slow here.
func (s *Server) webhookReceive(w http.ResponseWriter, r *http.Request) {
	tenantSlug := r.PathValue("tenant_slug")
	flowID := r.PathValue("flow_id")
	token := extractWebhookToken(r, s.log, flowID)

	target, err := s.flows.ResolveWebhookTarget(r.Context(), tenantSlug, flowID, token)
	if err != nil {
		// Deliberately vague: don't leak whether the flow exists.
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		writeErr(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body)) // harmless defence in depth

	// Connector signature check runs AFTER body is read (verifiers need it).
	if target.WebhookConnectorID != "" {
		d := connectors.Get(target.WebhookConnectorID)
		if d == nil || d.VerifyWebhook == nil {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if target.WebhookSigningSecret == "" {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if err := d.VerifyWebhook(r.Context(), target.WebhookSigningSecret, r.Header, body); err != nil {
			// Log for the operator (the verifier's error says what failed),
			// but return a constant-time generic message.
			s.log.Warn("webhook signature rejected",
				"flow_id", flowID,
				"connector", target.WebhookConnectorID,
				"err", err)
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
	}

	headers := map[string]string{}
	for k, v := range r.Header {
		if len(v) == 0 {
			continue
		}
		headers[k] = v[0]
	}
	payload, err := json.Marshal(map[string]any{
		"kind":    "webhook",
		"method":  r.Method,
		"headers": headers,
		"body":    json.RawMessage(bodyAsJSON(body)),
		"query":   r.URL.RawQuery,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.flowDispatcher.Dispatch(r.Context(), target.Flow, payload); err != nil {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// bodyAsJSON returns the body as a JSON value if it parses, otherwise as a
// JSON string. Either way the dispatched payload stays valid JSON.
func bodyAsJSON(body []byte) []byte {
	if len(body) == 0 {
		return []byte("null")
	}
	var v any
	if err := json.Unmarshal(body, &v); err == nil {
		return body
	}
	quoted, _ := json.Marshal(string(body))
	return quoted
}
