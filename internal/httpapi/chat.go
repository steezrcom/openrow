package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/openrow/openrow/internal/ai"
	"github.com/openrow/openrow/internal/auth"
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
		writeErr(w, http.StatusServiceUnavailable, "chat not available: LLM not configured")
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

// chatStream handles streaming chat turns via Server-Sent Events. Each line
// is a single JSON object carrying a `type` field the client switches on:
// text_delta (incremental tokens), tool_start / tool_end (pills), done
// (final assistant turn), and error.
func (s *Server) chatStream(w http.ResponseWriter, r *http.Request) {
	if s.agent == nil {
		writeErr(w, http.StatusServiceUnavailable, "chat not available: LLM not configured")
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

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // tell reverse proxies not to buffer

	// emit is called from the agent (same goroutine). Guard with a mutex
	// because if we ever move emission to a worker pool we'll need it.
	var mu sync.Mutex
	emit := func(ev ai.StreamEvent) {
		mu.Lock()
		defer mu.Unlock()
		b, err := json.Marshal(ev)
		if err != nil {
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	if err := s.agent.RunStream(r.Context(), m.TenantID, m.PGSchema, req.History, msg, emit); err != nil {
		emit(ai.StreamEvent{Type: "error", Message: err.Error()})
	}
}
