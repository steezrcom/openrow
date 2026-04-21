package httpapi

import (
	"log/slog"
	"net/http"

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
	log            *slog.Logger
	users          *auth.UserService
	sessions       *auth.SessionService
	memberships    *auth.MembershipService
	passwordResets *auth.PasswordResetService
	tenants        *tenant.Service
	entities       *entities.Service
	dashboards     *reports.Service
	reportExec     *reports.Executor
	proposer       *ai.Proposer
	agent          *ai.Agent
	llm            *llm.Service
	connectors     *connectors.Service
	flows          *flows.Service
	flowRunner     *flows.Runner
	flowDispatcher flows.Dispatcher
	chatLimiter    *ratelimit.Keyed
	mail           mailer.Mailer
	appURL         string
	secureCookies  bool
	spaDir         string
}

type Deps struct {
	Log            *slog.Logger
	Users          *auth.UserService
	Sessions       *auth.SessionService
	Memberships    *auth.MembershipService
	PasswordResets *auth.PasswordResetService
	Tenants        *tenant.Service
	Entities       *entities.Service
	Dashboards     *reports.Service
	ReportExec     *reports.Executor
	Proposer       *ai.Proposer
	Agent          *ai.Agent
	LLM            *llm.Service
	Connectors     *connectors.Service
	Flows          *flows.Service
	FlowRunner     *flows.Runner
	FlowDispatcher flows.Dispatcher
	Mailer         mailer.Mailer
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
	// Chat rate limit: avg 1 message every 2s per user, burst of 5.
	// Plenty for real usage; blocks only pathological loops / abuse.
	chatLim := ratelimit.New(0.5, 5)
	return &Server{
		log:            d.Log,
		users:          d.Users,
		sessions:       d.Sessions,
		memberships:    d.Memberships,
		passwordResets: d.PasswordResets,
		tenants:        d.Tenants,
		entities:       d.Entities,
		dashboards:     d.Dashboards,
		reportExec:     d.ReportExec,
		proposer:       d.Proposer,
		agent:          d.Agent,
		llm:            d.LLM,
		connectors:     d.Connectors,
		flows:          d.Flows,
		flowRunner:     d.FlowRunner,
		flowDispatcher: d.FlowDispatcher,
		chatLimiter:    chatLim,
		mail:           d.Mailer,
		appURL:         appURL,
		secureCookies:  d.SecureCookies,
		spaDir:         d.SPADir,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Public
	mux.HandleFunc("POST /api/v1/auth/signup", s.signup)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
	mux.HandleFunc("POST /api/v1/auth/forgot", s.forgotPassword)
	mux.HandleFunc("POST /api/v1/auth/reset", s.resetPassword)
	mux.HandleFunc("POST /webhooks/{tenant_slug}/{flow_id}", s.webhookReceive)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Authed (any user)
	authed := http.NewServeMux()
	authed.HandleFunc("GET /api/v1/me", s.me)
	authed.HandleFunc("POST /api/v1/orgs", s.createOrg)
	authed.HandleFunc("POST /api/v1/memberships/{id}/activate", s.activateMembership)

	// Authed + active org required
	authed.Handle("GET /api/v1/entities", auth.RequireMembership(http.HandlerFunc(s.listEntities)))
	authed.Handle("POST /api/v1/entities", auth.RequireMembership(http.HandlerFunc(s.proposeEntity)))
	authed.Handle("POST /api/v1/entities/spec", auth.RequireMembership(http.HandlerFunc(s.createEntityFromSpec)))
	authed.Handle("GET /api/v1/entities/{name}", auth.RequireMembership(http.HandlerFunc(s.getEntity)))
	authed.Handle("GET /api/v1/entities/{name}/rows", auth.RequireMembership(http.HandlerFunc(s.listRows)))
	authed.Handle("POST /api/v1/entities/{name}/rows", auth.RequireMembership(http.HandlerFunc(s.createRow)))
	authed.Handle("DELETE /api/v1/entities/{name}/rows/{id}", auth.RequireMembership(http.HandlerFunc(s.deleteRow)))
	authed.Handle("PATCH /api/v1/entities/{name}/rows/{id}", auth.RequireMembership(http.HandlerFunc(s.updateRow)))
	authed.Handle("POST /api/v1/entities/{name}/fields", auth.RequireMembership(http.HandlerFunc(s.addField)))
	authed.Handle("DELETE /api/v1/entities/{name}/fields/{field}", auth.RequireMembership(http.HandlerFunc(s.dropField)))
	authed.Handle("GET /api/v1/entities/{name}/fields/{field}/options", auth.RequireMembership(http.HandlerFunc(s.listFieldOptions)))
	authed.Handle("POST /api/v1/chat/messages/stream", auth.RequireMembership(http.HandlerFunc(s.chatStream)))

	authed.Handle("GET /api/v1/templates", auth.RequireAuth(http.HandlerFunc(s.listTemplates)))
	authed.Handle("POST /api/v1/templates/{id}/apply", auth.RequireMembership(http.HandlerFunc(s.applyTemplate)))

	authed.Handle("GET /api/v1/llm/providers", auth.RequireAuth(http.HandlerFunc(s.listLLMProviders)))
	authed.Handle("GET /api/v1/llm/config", auth.RequireMembership(http.HandlerFunc(s.getLLMConfig)))
	authed.Handle("PUT /api/v1/llm/config", auth.RequireMembership(http.HandlerFunc(s.putLLMConfig)))
	authed.Handle("DELETE /api/v1/llm/config", auth.RequireMembership(http.HandlerFunc(s.deleteLLMConfig)))
	authed.Handle("POST /api/v1/llm/models/list", auth.RequireAuth(http.HandlerFunc(s.listLLMModels)))
	authed.Handle("POST /api/v1/llm/test", auth.RequireAuth(http.HandlerFunc(s.testLLM)))
	authed.Handle("POST /api/v1/llm/self-test", auth.RequireMembership(http.HandlerFunc(s.selfTestLLM)))

	authed.Handle("GET /api/v1/flows", auth.RequireMembership(http.HandlerFunc(s.listFlows)))
	authed.Handle("POST /api/v1/flows", auth.RequireMembership(http.HandlerFunc(s.createFlow)))
	authed.Handle("GET /api/v1/flows/tools", auth.RequireMembership(http.HandlerFunc(s.listFlowTools)))
	authed.Handle("GET /api/v1/flows/{id}", auth.RequireMembership(http.HandlerFunc(s.getFlow)))
	authed.Handle("PATCH /api/v1/flows/{id}", auth.RequireMembership(http.HandlerFunc(s.patchFlow)))
	authed.Handle("DELETE /api/v1/flows/{id}", auth.RequireMembership(http.HandlerFunc(s.deleteFlow)))
	authed.Handle("POST /api/v1/flows/{id}/trigger", auth.RequireMembership(http.HandlerFunc(s.triggerFlow)))
	authed.Handle("POST /api/v1/flows/{id}/webhook_token", auth.RequireMembership(http.HandlerFunc(s.rotateFlowWebhookToken)))
	authed.Handle("GET /api/v1/flows/{id}/runs", auth.RequireMembership(http.HandlerFunc(s.listFlowRuns)))
	authed.Handle("GET /api/v1/flow_runs/{run_id}", auth.RequireMembership(http.HandlerFunc(s.getFlowRun)))
	authed.Handle("GET /api/v1/flow_approvals", auth.RequireMembership(http.HandlerFunc(s.listFlowApprovals)))
	authed.Handle("POST /api/v1/flow_approvals/{id}/resolve", auth.RequireMembership(http.HandlerFunc(s.resolveFlowApproval)))

	authed.Handle("GET /api/v1/connectors", auth.RequireAuth(http.HandlerFunc(s.listConnectors)))
	authed.Handle("GET /api/v1/connectors/configs", auth.RequireMembership(http.HandlerFunc(s.listConnectorConfigs)))
	authed.Handle("PUT /api/v1/connectors/configs/{id}", auth.RequireMembership(http.HandlerFunc(s.putConnectorConfig)))
	authed.Handle("POST /api/v1/connectors/configs/{id}/test", auth.RequireMembership(http.HandlerFunc(s.testConnectorConfig)))
	authed.Handle("DELETE /api/v1/connectors/configs/{id}", auth.RequireMembership(http.HandlerFunc(s.deleteConnectorConfig)))

	authed.Handle("GET /api/v1/dashboards", auth.RequireMembership(http.HandlerFunc(s.listDashboards)))
	authed.Handle("POST /api/v1/dashboards", auth.RequireMembership(http.HandlerFunc(s.createDashboard)))
	authed.Handle("GET /api/v1/dashboards/{slug}", auth.RequireMembership(http.HandlerFunc(s.getDashboard)))
	authed.Handle("PATCH /api/v1/dashboards/{slug}", auth.RequireMembership(http.HandlerFunc(s.patchDashboard)))
	authed.Handle("DELETE /api/v1/dashboards/{slug}", auth.RequireMembership(http.HandlerFunc(s.deleteDashboard)))
	authed.Handle("POST /api/v1/dashboards/{slug}/reports", auth.RequireMembership(http.HandlerFunc(s.addReport)))
	authed.Handle("POST /api/v1/dashboards/{slug}/reports/reorder", auth.RequireMembership(http.HandlerFunc(s.reorderReports)))
	authed.Handle("PATCH /api/v1/reports/{id}", auth.RequireMembership(http.HandlerFunc(s.patchReport)))
	authed.Handle("DELETE /api/v1/reports/{id}", auth.RequireMembership(http.HandlerFunc(s.deleteReport)))
	authed.Handle("POST /api/v1/reports/{id}/execute", auth.RequireMembership(http.HandlerFunc(s.executeReport)))

	mux.Handle("/api/v1/", auth.RequireAuth(authed))

	if s.spaDir != "" {
		mux.Handle("/", spa.Handler(s.spaDir))
	}

	attach := (&auth.Middleware{
		Sessions:    s.sessions,
		Users:       s.users,
		Memberships: s.memberships,
	}).Attach

	return attach(mux)
}
