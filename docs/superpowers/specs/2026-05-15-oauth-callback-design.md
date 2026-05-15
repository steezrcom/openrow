# Connector OAuth callback flow

Status: approved
Date: 2026-05-15
Owner: johnny@steezr.com

## Problem

Three of openrow's connectors (Česká spořitelna, ČSOB, Revolut Business) use
OAuth 2.0 with a long-lived refresh token. Today the user runs the one-time
consent dance outside openrow — typically with `curl`, a one-off Python
script, or Postman — captures `refresh_token` from the token response, and
pastes it into Settings → Connectors. That works but it's hostile to anyone
who isn't comfortable around HTTP.

We want a button that opens the bank's login page and, on consent, returns
the user to openrow with the refresh token already saved.

## Goals

- One-click "Authorize" from the connector settings modal.
- Same code path for self-hosters and managed openrow.app users; the
  callback URL differs only in host.
- Cross-tenant safe: a callback from the bank lands on the workspace that
  initiated it, with no possibility of writing a token into someone else's
  config.
- Extensible: CSAS first, but the framework also serves CSOB and Revolut
  without further plumbing.

## Non-goals

- PKCE. Each connector here is a "private client" (the secret is stored
  server-side), and the providers don't require PKCE for confidential
  clients. We can add it later if a provider demands it.
- A dynamic OAuth client registration flow. The user still creates the
  application in the provider's portal and pastes client_id /
  client_secret manually. The callback only automates the consent step.
- A token-rotation UI separate from the existing automatic rotation on
  every API call. Re-authorising overwrites the refresh token; that's the
  only manual rotation hook we need.

## Design

### Connector framework

`connectors.Connector` gains an optional pointer:

```go
type Connector struct {
    // …existing fields…
    OAuth *OAuthMeta
}

type OAuthMeta struct {
    // AuthorizeURL / TokenURL keyed by the value of the connector's
    // environment field. The empty key is the default; "sandbox" /
    // "production" / other values are explicit overrides.
    AuthorizeURL map[string]string
    TokenURL     map[string]string

    // Scope sent on the authorize request and (where the provider
    // requires it) the token exchange.
    Scope string

    // EnvField names the credential field that selects which endpoint
    // map entry to use. Empty defaults to "environment".
    EnvField string

    // ClientIDField / ClientSecretField name the fields holding the
    // OAuth client credentials. Default to "client_id" / "client_secret".
    ClientIDField     string
    ClientSecretField string

    // RefreshTokenField names the field the captured refresh_token is
    // written into. Default "refresh_token".
    RefreshTokenField string

    // ExtraAuthorizeParams adds connector-specific query params to the
    // authorize URL.
    ExtraAuthorizeParams map[string]string
}
```

For CSAS:

```go
OAuth: &connectors.OAuthMeta{
    AuthorizeURL: map[string]string{
        "":        "https://bezpecnost.csas.cz/api/psd2/fl/oidc/v1/auth",
        "sandbox": "https://webapi.developers.erstegroup.com/api/csas/sandbox/v1/sandbox-idp/auth",
    },
    TokenURL: map[string]string{
        "":        "https://bezpecnost.csas.cz/api/psd2/fl/oidc/v1/token",
        "sandbox": "https://webapi.developers.erstegroup.com/api/csas/sandbox/v1/sandbox-idp/token",
    },
    Scope: "siblings.accounts",
}
```

Production URLs verified via the OIDC discovery document at
`https://bezpecnost.csas.cz/api/psd2/fl/oidc/v1/.well-known/openid-configuration`.

### Signed state

Each authorize request carries a `state` parameter that the callback
verifies before doing anything. Format:

```
base64url(payload) "." base64url(hmac_sha256(payload, key))
```

Payload is a small JSON object:

```json
{ "t": "<tenant_id>", "c": "<connector_id>", "e": <unix_seconds>, "n": "<base64url 16 bytes>" }
```

- `t` binds the dance to one workspace.
- `c` binds it to one connector descriptor.
- `e` is 600 seconds in the future. Past `e`, the callback rejects.
- `n` is per-request random, so two simultaneous starts from the same
  workspace produce different states.

Key derivation: the deployment's existing `OPENROW_SECRET_KEY` (already 32
bytes) is fed to SHA-256 with a domain separator (`"openrow-oauth-state-v1"`)
to produce the HMAC key. This means the same key is used across all
replicas of a deployment without separate config, while staying
domain-separated from the AES key used by `internal/secrets`.

Lives in a new tiny package `internal/signedstate` so the secrets package
stays focused on encryption.

### HTTP routes

Two endpoints. The first is authed, the second is public.

`GET /api/v1/connectors/{id}/oauth/start`

- Requires admin role.
- Loads the connector descriptor; aborts with 400 if `OAuth == nil`.
- Loads the stored config for the caller's tenant; aborts with 400 if
  client_id or client_secret aren't present yet.
- Builds the redirect URI: `<app_origin>/oauth/callback/{id}`.
- Builds the signed state bound to (tenant_id, connector_id).
- Constructs the authorize URL with `response_type=code`, `client_id`,
  `redirect_uri`, `scope`, `state`, and any `ExtraAuthorizeParams`.
- Returns `302 Location: <authorize_url>`. The browser follows it
  straight to the bank.

`GET /oauth/callback/{id}`

- Public. Receives `code` + `state` from the bank.
- Verifies the state HMAC and decodes the payload. On any failure,
  302-redirects to `/app/settings/connectors?oauth_error=invalid_state`.
- Confirms `state.c == {id}`. (Defence in depth — the HMAC already
  prevents tampering.)
- Loads the connector descriptor + the tenant's config (using
  `state.t`). Aborts with `oauth_error=missing_config` if the config
  was deleted while the user was authorising.
- POSTs to the provider's token endpoint with
  `grant_type=authorization_code`, `code`, `redirect_uri`,
  `client_id`, `client_secret`. Uses the resolved env-specific
  TokenURL.
- Parses the response, extracts `refresh_token` (and `access_token` for
  diagnostics).
- Calls `connectors.Service.UpdateCredentialField` to persist the
  refresh token into the encrypted blob.
- 302-redirects to `/app/settings/connectors?oauth_connected=<id>`.

Failure modes always redirect with a query-string error code; the SPA
turns these into toasts so the user sees the outcome where they
started.

### Frontend

`Connector` DTO gains an `oauth_supported` boolean (server-side from
`descriptor.OAuth != nil`) and a `callback_url` string (server-side from
`<app_origin>/oauth/callback/<id>`).

`SafeConfig` adds nothing new — we already know which fields are
present via `fields_present`.

In `app.settings.connectors.index.tsx`:

- The Configure modal of an OAuth-supported connector shows a banner:
  *"openrow callback URL: `<url>` — paste this into the provider's
  OAuth application."*
- The `refresh_token` field is hidden when `oauth_supported` is true.
  The user no longer pastes it; the callback writes it.
- A primary action "Authorize with <Provider name>" appears once
  `client_id` and `client_secret` are saved (the modal first asks the
  user to Save those; Authorize unlocks after a save round-trip).
- Clicking Authorize navigates the same window to
  `/api/v1/connectors/<id>/oauth/start`. The redirect chain takes the
  user to the bank and back. On return, the SPA reads
  `?oauth_connected=<id>` or `?oauth_error=<code>` and toasts.

The settings index route also strips those query params after toasting
so a refresh doesn't re-trigger.

### Docs

`/docs/connectors/csas` Phase 3 step 3 ("Add a redirect URI…") becomes
concrete:

> The redirect URI is openrow's OAuth callback for this connector.
> Managed users: `https://openrow.app/oauth/callback/csas`. Self-hosters:
> `https://<your-openrow-host>/oauth/callback/csas`. The exact URL is
> shown in Settings → Connectors → Česká spořitelna.

Phase 4 sandbox dance shrinks to: save your sandbox credentials, click
Authorize, sign in with the portal's test user. The phase 6 production
dance becomes the same against the live login.

The manual `curl` instructions stay in an "If you want to capture the
refresh token by hand" appendix for users on deployments that haven't
upgraded yet, or who prefer to script it.

## Security

- The state is the entire CSRF defence. We use HMAC-SHA256 with a
  per-deployment key, constant-time compare, 10-minute TTL. Even with
  TTL, the state is single-use de-facto because the callback rotates
  the refresh_token; replays land harmlessly on the same value.
- The bank never sees `tenant_id` directly. The `state` parameter is
  opaque to the bank and round-trips back through it.
- The callback handler runs without a session. All authorisation comes
  from the signed state. This is correct: the bank is the only party
  that can produce a valid code paired with a valid state, and the
  state determines whose config receives the token.
- No secrets in logs. The OAuth code, state, refresh token, and access
  token are redacted at the logger; we log only short hints
  (`first4...last4`).
- Error redirects never echo back user-supplied content. Error codes
  are from a fixed enum.
- TLS: managed openrow.app is HTTPS-only. Self-hosters running on plain
  HTTP work in dev because banks accept `http://localhost`; production
  self-hosting behind HTTPS is the standard pattern.

## Migration

No DB migration. The existing `connector_configs` row format already
holds the credential blob; we just write `refresh_token` via the same
field.

Existing tenants who pasted a manually-captured refresh_token keep
working. If they re-click Authorize, the new flow overwrites their
refresh_token with a fresh one from the bank. Idempotent on the bank's
side (each consent issues a new refresh_token; the old one becomes
invalid after the new one is used once).

## Testing

- `internal/signedstate`: round-trip, tampering detection, wrong-key
  rejection, base64 padding tolerance.
- `internal/httpapi/oauth_test.go`: handler integration test with a
  fake token endpoint via `httptest.NewServer`. Cases:
  - happy path → refresh_token persisted, redirect with `oauth_connected`
  - invalid state → redirect with `oauth_error=invalid_state`
  - expired state → redirect with `oauth_error=expired_state`
  - mismatched connector id in state vs URL → redirect with
    `oauth_error=invalid_state`
  - token endpoint returns 5xx → redirect with `oauth_error=token_exchange`
  - config disappeared between start and callback → redirect with
    `oauth_error=missing_config`

## Open questions

None blocking. Future:

- CSOB and Revolut adoption — same framework, populate `OAuthMeta`
  with their endpoints once the per-provider quirks are verified.
- PKCE if any provider starts requiring it.
- A token-expiry indicator in the UI ("refresh token captured 14 days
  ago, valid 90 days from then"). Out of scope here.
