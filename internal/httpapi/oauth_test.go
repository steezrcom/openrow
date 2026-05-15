package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/openrow/openrow/internal/signedstate"
)

func testSigner(t *testing.T) *signedstate.Signer {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	s, err := signedstate.New(key)
	if err != nil {
		t.Fatalf("New signer: %v", err)
	}
	return s
}

func TestOAuthStateRoundTrip(t *testing.T) {
	s := &Server{oauthSigner: testSigner(t)}
	state, err := s.buildOAuthState("tenant-123", "csas")
	if err != nil {
		t.Fatalf("buildOAuthState: %v", err)
	}
	p, ok := s.openOAuthState(state)
	if !ok {
		t.Fatalf("openOAuthState rejected its own state")
	}
	if p.TenantID != "tenant-123" {
		t.Errorf("tenant id mismatch: %q", p.TenantID)
	}
	if p.ConnectorID != "csas" {
		t.Errorf("connector id mismatch: %q", p.ConnectorID)
	}
	if p.ExpiresAt <= 0 {
		t.Errorf("expires_at not set")
	}
	if p.Nonce == "" {
		t.Errorf("nonce not set")
	}
}

func TestOAuthStateRejectsTampering(t *testing.T) {
	s := &Server{oauthSigner: testSigner(t)}
	state, err := s.buildOAuthState("tenant-a", "csas")
	if err != nil {
		t.Fatalf("buildOAuthState: %v", err)
	}
	// Flip one char in the payload portion.
	idx := 0
	if state[idx] == 'A' {
		state = "B" + state[1:]
	} else {
		state = "A" + state[1:]
	}
	if _, ok := s.openOAuthState(state); ok {
		t.Fatalf("openOAuthState accepted tampered state")
	}
}

func TestOAuthStateRejectsWrongSigner(t *testing.T) {
	a := &Server{oauthSigner: testSigner(t)}
	b := &Server{oauthSigner: testSigner(t)}
	state, _ := a.buildOAuthState("tenant-a", "csas")
	if _, ok := b.openOAuthState(state); ok {
		t.Fatalf("openOAuthState accepted state from a different signer")
	}
}

func TestOAuthStateRejectsEmpty(t *testing.T) {
	s := &Server{oauthSigner: testSigner(t)}
	if _, ok := s.openOAuthState(""); ok {
		t.Fatalf("openOAuthState accepted empty state")
	}
	if _, ok := s.openOAuthState("garbage"); ok {
		t.Fatalf("openOAuthState accepted garbage state")
	}
}

func TestOAuthStateNilSigner(t *testing.T) {
	s := &Server{}
	if _, err := s.buildOAuthState("a", "b"); err == nil {
		t.Fatalf("buildOAuthState should fail without a signer")
	}
	if _, ok := s.openOAuthState("anything"); ok {
		t.Fatalf("openOAuthState should fail without a signer")
	}
}

func TestExchangeAuthorizationCodeHappyPath(t *testing.T) {
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		gotForm = r.PostForm
		_ = json.NewEncoder(w).Encode(map[string]string{
			"refresh_token": "rt-test-value",
			"access_token":  "at-test-value",
		})
	}))
	defer srv.Close()

	rt, err := exchangeAuthorizationCode(context.Background(), srv.URL,
		"client-id", "client-secret", "code-value", "https://example.com/cb")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if rt != "rt-test-value" {
		t.Fatalf("refresh_token mismatch: %q", rt)
	}
	if got := gotForm.Get("grant_type"); got != "authorization_code" {
		t.Errorf("grant_type=%q", got)
	}
	if got := gotForm.Get("code"); got != "code-value" {
		t.Errorf("code=%q", got)
	}
	if got := gotForm.Get("client_id"); got != "client-id" {
		t.Errorf("client_id=%q", got)
	}
	if got := gotForm.Get("client_secret"); got != "client-secret" {
		t.Errorf("client_secret=%q", got)
	}
	if got := gotForm.Get("redirect_uri"); got != "https://example.com/cb" {
		t.Errorf("redirect_uri=%q", got)
	}
}

func TestExchangeAuthorizationCodeNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()

	_, err := exchangeAuthorizationCode(context.Background(), srv.URL,
		"id", "secret", "code", "https://example.com/cb")
	if err == nil {
		t.Fatalf("expected error on 400")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should mention status: %v", err)
	}
}

func TestExchangeAuthorizationCodeBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	_, err := exchangeAuthorizationCode(context.Background(), srv.URL,
		"id", "secret", "code", "https://example.com/cb")
	if err == nil {
		t.Fatalf("expected decode error")
	}
}

func TestExchangeAuthorizationCodeEmptyRefresh(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "at-only"})
	}))
	defer srv.Close()

	rt, err := exchangeAuthorizationCode(context.Background(), srv.URL,
		"id", "secret", "code", "https://example.com/cb")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if rt != "" {
		t.Fatalf("expected empty refresh token, got %q", rt)
	}
}
