package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/steezrcom/steezr-erp/internal/ai"
	"github.com/steezrcom/steezr-erp/internal/auth"
	"github.com/steezrcom/steezr-erp/internal/entities"
	"github.com/steezrcom/steezr-erp/internal/mailer"
	"github.com/steezrcom/steezr-erp/internal/reports"
	"github.com/steezrcom/steezr-erp/internal/spa"
	"github.com/steezrcom/steezr-erp/internal/tenant"
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
	authed.Handle("POST /api/v1/chat/messages", auth.RequireMembership(http.HandlerFunc(s.chat)))

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
