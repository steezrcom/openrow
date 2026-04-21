package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/openrow/openrow/internal/ai"
	"github.com/openrow/openrow/internal/auth"
)

type chatRequest struct {
	History []ai.ChatTurn `json:"history"`
	Message string        `json:"message"`
}

// chatStream is the only chat endpoint. Each SSE `data:` line is one JSON
// event: text_delta (incremental tokens), tool_start / tool_end (pills), done
// (final turn + actions), error.
//
// Client disconnects are detected via r.Context(); the agent loop bails
// between iterations and we don't bother emitting a trailing error for a
// connection that's already gone.
func (s *Server) chatStream(w http.ResponseWriter, r *http.Request) {
	if s.agent == nil {
		writeErr(w, http.StatusServiceUnavailable, "chat not available: LLM not configured")
		return
	}
	user, _, _ := auth.FromContext(r.Context())
	m, _ := auth.MembershipFromContext(r.Context())

	if ok, retry := s.chatLimiter.Allow(user.ID); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		writeErr(w, http.StatusTooManyRequests,
			"Slow down. You're sending messages faster than the rate limit allows.")
		return
	}

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

	// Guarded for future use; current emission is single-goroutine.
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

	err := s.agent.RunStream(r.Context(), m.TenantID, m.PGSchema, req.History, msg, emit)
	if err == nil {
		return
	}
	// Client went away mid-turn — nothing to emit to.
	if errors.Is(err, context.Canceled) || errors.Is(r.Context().Err(), context.Canceled) {
		return
	}
	emit(ai.StreamEvent{Type: "error", Message: err.Error()})
}
