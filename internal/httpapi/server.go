package httpapi

import (
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/openrow/openrow/internal/ai"
	"github.com/openrow/openrow/internal/auth"
	"github.com/openrow/openrow/internal/connectors"
	"github.com/openrow/openrow/internal/entities"
	"github.com/openrow/openrow/internal/flows"
	"github.com/openrow/openrow/internal/llm"
	"github.com/openrow/openrow/internal/mailer"
	"github.com/openrow/openrow/internal/ratelimit"
	"github.com/openrow/openrow/internal/reports"
	"github.com/openrow/openrow/internal/spa"
	"github.com/openrow/openrow/internal/tenant"
)

type Server struct {
	log              *slog.Logger
	users            *auth.UserService
	sessions         *auth.SessionService
	memberships      *auth.MembershipService
	passwordResets   *auth.PasswordResetService
	tenants          *tenant.Service
	entities         *entities.Service
	dashboards       *reports.Service
	reportExec       *reports.Executor
	proposer         *ai.Proposer
	agent            *ai.Agent
	llm              *llm.Service
	connectors       *connectors.Service
	flows            *flows.Service
	flowRunner       *flows.Runner
	flowDispatcher   flows.Dispatcher
	chatLimiter      *ratelimit.Keyed
	loginLimiter     *ratelimit.Keyed
	signupLimiter    *ratelimit.Keyed
	resetLimiter     *ratelimit.Keyed
	mail             mailer.Mailer
	appURL           string
	appOrigin        string
	appHost          string
	secureCookies    bool
	spaDir           string
	externalBindings *ExternalBindings
}

type Deps struct {
	Log              *slog.Logger
	Users            *auth.UserService
	Sessions         *auth.SessionService
	Memberships      *auth.MembershipService
	PasswordResets   *auth.PasswordResetService
	Tenants          *tenant.Service
	Entities         *entities.Service
	Dashboards       *reports.Service
	ReportExec       *reports.Executor
	Proposer         *ai.Proposer
	Agent            *ai.Agent
	LLM              *llm.Service
	Connectors       *connectors.Service
	Flows            *flows.Service
	FlowRunner       *flows.Runner
	FlowDispatcher   flows.Dispatcher
	Mailer           mailer.Mailer
	ExternalBindings *ExternalBindings
	// AppURL is the public URL users should be directed to (used in email links).
	AppURL string
	// SecureCookies toggles the Secure flag on session cookies. Set true behind HTTPS.
	SecureCookies bool
	// SPADir is the path to the built React app. When empty the SPA route 503s,
	// which is expected in API-only dev mode where Vite serves the UI.
	SPADir string
}

func New(d Deps) *Server {
	appURL := d.AppURL
	if appURL == "" {
		appURL = "http://localhost:5173"
	}
	var appOrigin, appHost string
	if u, err := url.Parse(appURL); err == nil && u.Scheme != "" && u.Host != "" {
		appOrigin = u.Scheme + "://" + u.Host
		appHost = u.Host
	}
	// Chat rate limit: avg 1 message every 2s per user, burst of 5.
	// Plenty for real usage; blocks only pathological loops / abuse.
	chatLim := ratelimit.New(0.5, 5)
	// Auth-endpoint rate limits, all keyed by client IP. Login allows
	// rapid retries (typo recovery) but caps brute-force throughput.
	// Signup and password-reset are stricter — these are the enumeration
	// and email-spam vectors.
	loginLim := ratelimit.New(0.5, 5)         // 1 / 2s avg, burst 5
	signupLim := ratelimit.New(1.0/600.0, 3)  // ~3 per hour, burst 3
	resetLim := ratelimit.New(1.0/600.0, 3)   // ~3 per hour, burst 3
	return &Server{
		log:              d.Log,
		users:            d.Users,
		sessions:         d.Sessions,
		memberships:      d.Memberships,
		passwordResets:   d.PasswordResets,
		tenants:          d.Tenants,
		entities:         d.Entities,
		dashboards:       d.Dashboards,
		reportExec:       d.ReportExec,
		proposer:         d.Proposer,
		agent:            d.Agent,
		llm:              d.LLM,
		connectors:       d.Connectors,
		flows:            d.Flows,
		flowRunner:       d.FlowRunner,
		flowDispatcher:   d.FlowDispatcher,
		chatLimiter:      chatLim,
		loginLimiter:     loginLim,
		signupLimiter:    signupLim,
		resetLimiter:     resetLim,
		mail:             d.Mailer,
		appURL:           appURL,
		appOrigin:        appOrigin,
		appHost:          appHost,
		secureCookies:    d.SecureCookies,
		spaDir:           d.SPADir,
		externalBindings: d.ExternalBindings,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Public — rate-limited by client IP to blunt brute-force, enumeration
	// and reset-link spam. Each endpoint has its own bucket so a flooded
	// /forgot doesn't lock out /login.
	mux.Handle("POST /api/v1/auth/signup", rateLimitByIP(s.signupLimiter, http.HandlerFunc(s.signup)))
	mux.Handle("POST /api/v1/auth/login", rateLimitByIP(s.loginLimiter, http.HandlerFunc(s.login)))
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
	mux.Handle("POST /api/v1/auth/forgot", rateLimitByIP(s.resetLimiter, http.HandlerFunc(s.forgotPassword)))
	mux.Handle("POST /api/v1/auth/reset", rateLimitByIP(s.resetLimiter, http.HandlerFunc(s.resetPassword)))
	mux.HandleFunc("POST /webhooks/{tenant_slug}/{flow_id}", s.webhookReceive)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Authed (any user)
	authed := http.NewServeMux()
	authed.HandleFunc("GET /api/v1/me", s.me)
	authed.HandleFunc("POST /api/v1/orgs", s.createOrg)
	authed.HandleFunc("POST /api/v1/memberships/{id}/activate", s.activateMembership)

	// Authed + active org required.
	//
	// Role policy:
	//   member: row CRUD, reads, chat, ordinary view CRUD, execute reports/queries.
	//   admin: schema mutations (create entity, add field, create dashboard / report,
	//          create / patch flow, write connector + LLM config, trigger flows,
	//          resolve approvals).
	//   owner: destructive ops (drop field, apply template, delete entity field /
	//          flow / dashboard / report / connector, rotate webhook token, wipe LLM config).
	authed.Handle("GET /api/v1/entities", auth.RequireMembership(http.HandlerFunc(s.listEntities)))
	authed.Handle("POST /api/v1/entities", auth.RequireRole(auth.RoleAdmin, http.HandlerFunc(s.proposeEntity)))
	authed.Handle("POST /api/v1/entities/spec", auth.RequireRole(auth.RoleAdmin, http.HandlerFunc(s.createEntityFromSpec)))
	authed.Handle("GET /api/v1/entities/{name}", auth.RequireMembership(http.HandlerFunc(s.getEntity)))
	authed.Handle("GET /api/v1/entities/{name}/rows", auth.RequireMembership(http.HandlerFunc(s.listRows)))
	authed.Handle("POST /api/v1/entities/{name}/rows", auth.RequireMembership(http.HandlerFunc(s.createRow)))
	authed.Handle("DELETE /api/v1/entities/{name}/rows/{id}", auth.RequireMembership(http.HandlerFunc(s.deleteRow)))
	authed.Handle("PATCH /api/v1/entities/{name}/rows/{id}", auth.RequireMembership(http.HandlerFunc(s.updateRow)))
	authed.Handle("POST /api/v1/entities/{name}/fields", auth.RequireRole(auth.RoleAdmin, http.HandlerFunc(s.addField)))
	authed.Handle("DELETE /api/v1/entities/{name}/fields/{field}", auth.RequireRole(auth.RoleOwner, http.HandlerFunc(s.dropField)))
	authed.Handle("GET /api/v1/entities/{name}/fields/{field}/options", auth.RequireMembership(http.HandlerFunc(s.listFieldOptions)))
	authed.Handle("GET /api/v1/entities/{name}/views", auth.RequireMembership(http.HandlerFunc(s.listViews)))
	authed.Handle("POST /api/v1/entities/{name}/views", auth.RequireMembership(http.HandlerFunc(s.createView)))
	authed.Handle("PATCH /api/v1/views/{id}", auth.RequireMembership(http.HandlerFunc(s.patchView)))
	authed.Handle("DELETE /api/v1/views/{id}", auth.RequireMembership(http.HandlerFunc(s.deleteView)))
	authed.Handle("POST /api/v1/chat/messages/stream", auth.RequireMembership(http.HandlerFunc(s.chatStream)))

	authed.Handle("GET /api/v1/templates", auth.RequireAuth(http.HandlerFunc(s.listTemplates)))
	authed.Handle("POST /api/v1/templates/{id}/apply", auth.RequireRole(auth.RoleOwner, http.HandlerFunc(s.applyTemplate)))

	authed.Handle("GET /api/v1/llm/providers", auth.RequireAuth(http.HandlerFunc(s.listLLMProviders)))
	authed.Handle("GET /api/v1/llm/config", auth.RequireMembership(http.HandlerFunc(s.getLLMConfig)))
	authed.Handle("PUT /api/v1/llm/config", auth.RequireRole(auth.RoleAdmin, http.HandlerFunc(s.putLLMConfig)))
	authed.Handle("DELETE /api/v1/llm/config", auth.RequireRole(auth.RoleOwner, http.HandlerFunc(s.deleteLLMConfig)))
	authed.Handle("POST /api/v1/llm/models/list", auth.RequireAuth(http.HandlerFunc(s.listLLMModels)))
	authed.Handle("POST /api/v1/llm/test", auth.RequireAuth(http.HandlerFunc(s.testLLM)))
	authed.Handle("POST /api/v1/llm/self-test", auth.RequireMembership(http.HandlerFunc(s.selfTestLLM)))

	authed.Handle("GET /api/v1/flows", auth.RequireMembership(http.HandlerFunc(s.listFlows)))
	authed.Handle("POST /api/v1/flows", auth.RequireRole(auth.RoleAdmin, http.HandlerFunc(s.createFlow)))
	authed.Handle("GET /api/v1/flows/tools", auth.RequireMembership(http.HandlerFunc(s.listFlowTools)))
	authed.Handle("GET /api/v1/flows/{id}", auth.RequireMembership(http.HandlerFunc(s.getFlow)))
	authed.Handle("PATCH /api/v1/flows/{id}", auth.RequireRole(auth.RoleAdmin, http.HandlerFunc(s.patchFlow)))
	authed.Handle("DELETE /api/v1/flows/{id}", auth.RequireRole(auth.RoleOwner, http.HandlerFunc(s.deleteFlow)))
	authed.Handle("POST /api/v1/flows/{id}/trigger", auth.RequireRole(auth.RoleAdmin, http.HandlerFunc(s.triggerFlow)))
	authed.Handle("POST /api/v1/flows/{id}/webhook_token", auth.RequireRole(auth.RoleOwner, http.HandlerFunc(s.rotateFlowWebhookToken)))
	authed.Handle("GET /api/v1/flows/{id}/runs", auth.RequireMembership(http.HandlerFunc(s.listFlowRuns)))
	authed.Handle("GET /api/v1/flow_runs/{run_id}", auth.RequireMembership(http.HandlerFunc(s.getFlowRun)))
	authed.Handle("GET /api/v1/flow_approvals", auth.RequireMembership(http.HandlerFunc(s.listFlowApprovals)))
	authed.Handle("POST /api/v1/flow_approvals/{id}/resolve", auth.RequireRole(auth.RoleAdmin, http.HandlerFunc(s.resolveFlowApproval)))

	authed.Handle("GET /api/v1/connectors", auth.RequireAuth(http.HandlerFunc(s.listConnectors)))
	authed.Handle("GET /api/v1/connectors/configs", auth.RequireMembership(http.HandlerFunc(s.listConnectorConfigs)))
	authed.Handle("PUT /api/v1/connectors/configs/{id}", auth.RequireRole(auth.RoleAdmin, http.HandlerFunc(s.putConnectorConfig)))
	authed.Handle("POST /api/v1/connectors/configs/{id}/test", auth.RequireRole(auth.RoleAdmin, http.HandlerFunc(s.testConnectorConfig)))
	authed.Handle("DELETE /api/v1/connectors/configs/{id}", auth.RequireRole(auth.RoleOwner, http.HandlerFunc(s.deleteConnectorConfig)))

	authed.Handle("GET /api/v1/dashboards", auth.RequireMembership(http.HandlerFunc(s.listDashboards)))
	authed.Handle("POST /api/v1/dashboards", auth.RequireRole(auth.RoleAdmin, http.HandlerFunc(s.createDashboard)))
	authed.Handle("GET /api/v1/dashboards/{slug}", auth.RequireMembership(http.HandlerFunc(s.getDashboard)))
	authed.Handle("PATCH /api/v1/dashboards/{slug}", auth.RequireRole(auth.RoleAdmin, http.HandlerFunc(s.patchDashboard)))
	authed.Handle("DELETE /api/v1/dashboards/{slug}", auth.RequireRole(auth.RoleOwner, http.HandlerFunc(s.deleteDashboard)))
	authed.Handle("POST /api/v1/dashboards/{slug}/reports", auth.RequireRole(auth.RoleAdmin, http.HandlerFunc(s.addReport)))
	authed.Handle("POST /api/v1/dashboards/{slug}/reports/reorder", auth.RequireRole(auth.RoleAdmin, http.HandlerFunc(s.reorderReports)))
	authed.Handle("PATCH /api/v1/reports/{id}", auth.RequireRole(auth.RoleAdmin, http.HandlerFunc(s.patchReport)))
	authed.Handle("DELETE /api/v1/reports/{id}", auth.RequireRole(auth.RoleOwner, http.HandlerFunc(s.deleteReport)))
	authed.Handle("POST /api/v1/reports/{id}/execute", auth.RequireMembership(http.HandlerFunc(s.executeReport)))
	authed.Handle("POST /api/v1/queries/execute", auth.RequireMembership(http.HandlerFunc(s.executeQuery)))

	if s.externalBindings != nil {
		s.externalBindings.Mount(authed)
	}

	mux.Handle("/api/v1/", requireJSON(limitBody(auth.RequireAuth(authed))))

	if s.spaDir != "" {
		mux.Handle("/", spa.Handler(s.spaDir))
	}

	attach := (&auth.Middleware{
		Sessions:    s.sessions,
		Users:       s.users,
		Memberships: s.memberships,
	}).Attach

	return s.canonicalHost(s.secureHeaders(s.sameOriginCheck(attach(mux))))
}

// canonicalHost 301-redirects any request whose Host header differs from
// the configured APP_URL host to the canonical scheme+host. Keeps users
// off bare-vs-www variants where the SPA loads but POSTs trip the
// same-origin CSRF check. Skips:
//
//   - /healthz so dokku/k8s loopback health probes always succeed
//   - /.well-known/* so Let's Encrypt http-01 challenges can validate
//     non-canonical hosts and keep their certs alive
//   - requests where the parsed app URL has no host (dev mode without
//     APP_URL configured)
//
// Method handling: GET/HEAD use 301; other methods would lose the body
// on a redirect, so we serve them on the wrong host. With same-origin
// CSRF in place those POSTs will get rejected with "forbidden origin",
// which is the right answer for an attacker but the wrong answer for a
// user who just typed the bare domain. The SPA itself only ever issues
// same-origin XHRs, so once the GET lands on the canonical host the
// subsequent POSTs are fine.
func (s *Server) canonicalHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.appHost == "" || s.appOrigin == "" {
			next.ServeHTTP(w, r)
			return
		}
		host := r.Host
		// Strip any :port suffix; dokku/nginx already terminates TLS
		// so we compare on hostname only.
		if i := strings.LastIndex(host, ":"); i >= 0 {
			if _, err := strconv.Atoi(host[i+1:]); err == nil {
				host = host[:i]
			}
		}
		if host == "" || strings.EqualFold(host, s.appHost) {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/healthz" || strings.HasPrefix(r.URL.Path, "/.well-known/") {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			// Don't redirect mutating requests — the body would be lost.
			// Let same-origin CSRF take care of rejecting them.
			next.ServeHTTP(w, r)
			return
		}
		target := s.appOrigin + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})
}

func (s *Server) secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if s.secureCookies {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) sameOriginCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/webhooks/") {
			next.ServeHTTP(w, r)
			return
		}

		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" {
			if origin != s.appOrigin {
				writeErr(w, http.StatusForbidden, "forbidden origin")
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		referer := strings.TrimSpace(r.Header.Get("Referer"))
		if referer == "" {
			writeErr(w, http.StatusForbidden, "missing origin")
			return
		}
		ref, err := url.Parse(referer)
		if err != nil || ref.Host == "" {
			writeErr(w, http.StatusForbidden, "forbidden origin")
			return
		}
		refOrigin := ref.Scheme + "://" + ref.Host
		if refOrigin != s.appOrigin && ref.Host != s.appHost {
			writeErr(w, http.StatusForbidden, "forbidden origin")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			ct := r.Header.Get("Content-Type")
			if i := strings.Index(ct, ";"); i >= 0 {
				ct = ct[:i]
			}
			ct = strings.TrimSpace(strings.ToLower(ct))
			if !strings.HasPrefix(ct, "application/json") {
				writeErr(w, http.StatusUnsupportedMediaType, "unsupported media type")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// rateLimitByIP applies a Keyed limiter on the request's source IP. Used
// on the unauthenticated auth endpoints (login / signup / forgot / reset)
// to make brute-force, enumeration and email-flood attacks pay a real
// cost per IP. The key falls back to the raw RemoteAddr when there's no
// X-Forwarded-For — fine for a dokku deployment where nginx already
// terminates and rewrites client IPs into the header.
func rateLimitByIP(lim *ratelimit.Keyed, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if ok, retry := lim.Allow(ip); !ok {
			w.Header().Set("Retry-After", retrySeconds(retry))
			writeErr(w, http.StatusTooManyRequests, "too many requests")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// First entry is the original client; the rest are proxies.
		if comma := strings.Index(xff, ","); comma >= 0 {
			return strings.TrimSpace(xff[:comma])
		}
		return strings.TrimSpace(xff)
	}
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return strings.TrimSpace(xrip)
	}
	// Fallback: RemoteAddr is "host:port"; strip the port.
	addr := r.RemoteAddr
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[:i]
	}
	return addr
}

func retrySeconds(d time.Duration) string {
	secs := int(d.Round(time.Second).Seconds())
	if secs < 1 {
		secs = 1
	}
	return strconv.Itoa(secs)
}

func limitBody(next http.Handler) http.Handler {
	const (
		defaultMax = int64(1 << 20)       // 1 MiB
		chatMax    = int64(8 * (1 << 20)) // 8 MiB
		rowsMax    = int64(4 * (1 << 20)) // 4 MiB
	)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil {
			next.ServeHTTP(w, r)
			return
		}
		limit := defaultMax
		p := r.URL.Path
		switch {
		case p == "/api/v1/chat/messages/stream":
			limit = chatMax
		case strings.HasPrefix(p, "/api/v1/entities/") && strings.HasSuffix(p, "/rows"):
			limit = rowsMax
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}
