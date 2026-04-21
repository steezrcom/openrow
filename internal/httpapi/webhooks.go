package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
)

// webhookReceive is the public endpoint flows hook up to external services.
// Path: /webhooks/{tenant_slug}/{flow_id}?token=<plaintext>
//
// Returns 200 as soon as the run is queued; the actual run executes on the
// dispatcher's worker pool. External senders expect fast 2xx and retry on
// 5xx, so don't do anything slow here.
func (s *Server) webhookReceive(w http.ResponseWriter, r *http.Request) {
	tenantSlug := r.PathValue("tenant_slug")
	flowID := r.PathValue("flow_id")
	token := r.URL.Query().Get("token")

	flow, err := s.flows.ResolveWebhookTarget(r.Context(), tenantSlug, flowID, token)
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
	if _, err := s.flowDispatcher.Dispatch(r.Context(), flow, payload); err != nil {
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
	// Non-JSON body: wrap as string.
	quoted, _ := json.Marshal(string(body))
	return quoted
}
