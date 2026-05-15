package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openrow/openrow/internal/auth"
	"github.com/openrow/openrow/internal/connectors"
	"github.com/openrow/openrow/internal/signedstate"
)

// oauth state TTL — long enough to cover a slow user signing into their
// bank, short enough to keep replay windows small.
const oauthStateTTL = 10 * time.Minute

// oauthStatePayload is the JSON body of the signed `state` parameter.
// Field names are short to keep the URL compact.
type oauthStatePayload struct {
	TenantID    string `json:"t"`
	ConnectorID string `json:"c"`
	ExpiresAt   int64  `json:"e"`
	Nonce       string `json:"n"`
}

// handleOAuthStart kicks off the OAuth 2.0 authorization-code dance for
// a connector. Requires an admin session and a saved config that
// already has client_id + client_secret. Redirects the browser to the
// provider's authorize URL.
func (s *Server) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	descriptor := connectors.Get(id)
	if descriptor == nil {
		writeErr(w, http.StatusNotFound, "unknown connector")
		return
	}
	if descriptor.OAuth == nil {
		writeErr(w, http.StatusBadRequest, "connector does not support OAuth")
		return
	}

	m, _ := auth.MembershipFromContext(r.Context())
	cfg, err := s.connectors.Get(r.Context(), m.TenantID, id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "save the client credentials before authorizing")
		return
	}

	envField, clientIDField, clientSecretField, _ := descriptor.OAuth.Resolved()
	clientID := strings.TrimSpace(cfg.Credentials[clientIDField])
	clientSecret := strings.TrimSpace(cfg.Credentials[clientSecretField])
	if clientID == "" || clientSecret == "" {
		writeErr(w, http.StatusBadRequest, "save the client credentials before authorizing")
		return
	}

	env := strings.TrimSpace(cfg.Credentials[envField])
	authorizeURL := descriptor.OAuth.EndpointFor(descriptor.OAuth.AuthorizeURL, env)
	if authorizeURL == "" {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("authorize URL not configured for environment %q", env))
		return
	}

	redirectURI := s.oauthRedirectURI(id)

	state, err := s.buildOAuthState(m.TenantID, id)
	if err != nil {
		s.log.Error("oauth: build state failed", "err", err, "connector", id)
		writeErr(w, http.StatusInternalServerError, "could not start authorization")
		return
	}

	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", descriptor.OAuth.Scope)
	q.Set("state", state)
	for k, v := range descriptor.OAuth.ExtraAuthorizeParams {
		q.Set(k, v)
	}

	sep := "?"
	if strings.Contains(authorizeURL, "?") {
		sep = "&"
	}
	http.Redirect(w, r, authorizeURL+sep+q.Encode(), http.StatusFound)
}

// handleOAuthCallback receives the authorization-code redirect from the
// provider, validates the signed state, exchanges the code for tokens,
// and persists the refresh_token into the tenant's connector config.
//
// This endpoint is public — the bank reaches it without a user session.
// Authorisation comes entirely from the signed state parameter, which
// names the tenant and connector that authorised the dance.
func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	q := r.URL.Query()
	code := q.Get("code")
	state := q.Get("state")
	providerErr := q.Get("error")
	if providerErr != "" {
		s.oauthFinish(w, r, id, "provider_error")
		return
	}
	if code == "" || state == "" {
		s.oauthFinish(w, r, id, "missing_params")
		return
	}

	payload, ok := s.openOAuthState(state)
	if !ok {
		s.oauthFinish(w, r, id, "invalid_state")
		return
	}
	if payload.ConnectorID != id {
		s.oauthFinish(w, r, id, "invalid_state")
		return
	}
	if time.Now().Unix() > payload.ExpiresAt {
		s.oauthFinish(w, r, id, "expired_state")
		return
	}

	descriptor := connectors.Get(id)
	if descriptor == nil || descriptor.OAuth == nil {
		s.oauthFinish(w, r, id, "unsupported")
		return
	}

	cfg, err := s.connectors.Get(r.Context(), payload.TenantID, id)
	if err != nil {
		s.oauthFinish(w, r, id, "missing_config")
		return
	}

	envField, clientIDField, clientSecretField, refreshField := descriptor.OAuth.Resolved()
	clientID := strings.TrimSpace(cfg.Credentials[clientIDField])
	clientSecret := strings.TrimSpace(cfg.Credentials[clientSecretField])
	if clientID == "" || clientSecret == "" {
		s.oauthFinish(w, r, id, "missing_config")
		return
	}

	env := strings.TrimSpace(cfg.Credentials[envField])
	tokenURL := descriptor.OAuth.EndpointFor(descriptor.OAuth.TokenURL, env)
	if tokenURL == "" {
		s.oauthFinish(w, r, id, "missing_token_endpoint")
		return
	}

	redirectURI := s.oauthRedirectURI(id)
	refreshToken, err := exchangeAuthorizationCode(r.Context(), tokenURL, clientID, clientSecret, code, redirectURI)
	if err != nil {
		s.log.Warn("oauth: code exchange failed",
			"connector", id,
			"tenant", payload.TenantID,
			"err", err)
		s.oauthFinish(w, r, id, "token_exchange")
		return
	}
	if refreshToken == "" {
		s.oauthFinish(w, r, id, "no_refresh_token")
		return
	}

	if err := s.connectors.UpdateCredentialField(r.Context(), payload.TenantID, id, refreshField, refreshToken); err != nil {
		s.log.Error("oauth: persist refresh_token failed",
			"connector", id,
			"tenant", payload.TenantID,
			"err", err)
		s.oauthFinish(w, r, id, "persist_failed")
		return
	}

	s.oauthFinish(w, r, id, "")
}

// oauthRedirectURI returns the public callback URL for a connector.
// Falls back to a localhost path during dev when appOrigin is empty.
func (s *Server) oauthRedirectURI(connectorID string) string {
	origin := strings.TrimRight(s.appOrigin, "/")
	if origin == "" {
		origin = "http://localhost:8080"
	}
	return origin + "/oauth/callback/" + connectorID
}

// buildOAuthState returns a base64url HMAC-signed state value binding
// the OAuth dance to (tenant, connector) with a 10-minute TTL.
func (s *Server) buildOAuthState(tenantID, connectorID string) (string, error) {
	if s.oauthSigner == nil {
		return "", fmt.Errorf("oauth signer not configured")
	}
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", err
	}
	payload := oauthStatePayload{
		TenantID:    tenantID,
		ConnectorID: connectorID,
		ExpiresAt:   time.Now().Add(oauthStateTTL).Unix(),
		Nonce:       base64.RawURLEncoding.EncodeToString(nonceBytes),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return s.oauthSigner.Build(raw), nil
}

// openOAuthState verifies and parses a state value. Returns ok=false
// for any failure — callers redirect with a generic error code.
func (s *Server) openOAuthState(state string) (oauthStatePayload, bool) {
	if s.oauthSigner == nil {
		return oauthStatePayload{}, false
	}
	raw, err := s.oauthSigner.Open(state)
	if err != nil {
		return oauthStatePayload{}, false
	}
	var p oauthStatePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return oauthStatePayload{}, false
	}
	if p.TenantID == "" || p.ConnectorID == "" {
		return oauthStatePayload{}, false
	}
	return p, true
}

// oauthFinish redirects the user back to the settings page with the
// outcome encoded in the query string. errCode == "" means success.
func (s *Server) oauthFinish(w http.ResponseWriter, r *http.Request, connectorID, errCode string) {
	dest := strings.TrimRight(s.appURL, "/")
	if dest == "" {
		dest = "/"
	}
	q := url.Values{}
	if errCode == "" {
		q.Set("oauth_connected", connectorID)
	} else {
		q.Set("oauth_error", errCode)
		q.Set("oauth_connector", connectorID)
	}
	http.Redirect(w, r, dest+"/app/settings/connectors?"+q.Encode(), http.StatusFound)
}

// exchangeAuthorizationCode POSTs to the provider's token endpoint with
// the standard authorization-code grant params. Returns the refresh
// token from the response, or an error.
func exchangeAuthorizationCode(ctx context.Context, tokenURL, clientID, clientSecret, code, redirectURI string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token endpoint: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 32<<10))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("token endpoint: status %d", res.StatusCode)
	}
	var out struct {
		RefreshToken string `json:"refresh_token"`
		AccessToken  string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	return out.RefreshToken, nil
}

// _ = signedstate.Signer ensures the package import isn't elided if the
// signer is plumbed in but not yet referenced elsewhere.
var _ = (*signedstate.Signer)(nil)
