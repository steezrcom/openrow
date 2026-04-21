package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/steezrcom/steezr-erp/internal/ai"
	"github.com/steezrcom/steezr-erp/internal/auth"
)

type chatRequest struct {
	History []ai.ChatTurn `json:"history"`
	Message string        `json:"message"`
}

type chatResponse struct {
	Assistant ai.ChatTurn `json:"assistant"`
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	if s.agent == nil {
		writeErr(w, http.StatusServiceUnavailable, "chat not available: ANTHROPIC_API_KEY not configured")
		return
	}
	m, _ := auth.MembershipFromContext(r.Context())

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		writeErr(w, http.StatusBadRequest, "message is required")
		return
	}

	assistant, err := s.agent.Run(r.Context(), m.TenantID, m.PGSchema, req.History, msg)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, chatResponse{Assistant: *assistant})
}
