package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/steezrcom/steezr-erp/internal/ai"
	"github.com/steezrcom/steezr-erp/internal/auth"
	"github.com/steezrcom/steezr-erp/internal/entities"
	"github.com/steezrcom/steezr-erp/internal/spa"
	"github.com/steezrcom/steezr-erp/internal/tenant"
)

type Server struct {
	log           *slog.Logger
	users         *auth.UserService
	sessions      *auth.SessionService
	memberships   *auth.MembershipService
	tenants       *tenant.Service
	entities      *entities.Service
	proposer      *ai.Proposer
	secureCookies bool
	spaDir        string
}

type Deps struct {
	Log         *slog.Logger
	Users       *auth.UserService
	Sessions    *auth.SessionService
	Memberships *auth.MembershipService
	Tenants     *tenant.Service
	Entities    *entities.Service
	Proposer    *ai.Proposer
	// SecureCookies toggles the Secure flag on session cookies. Set true behind HTTPS.
	SecureCookies bool
	// SPADir is the path to the built React app. When empty the SPA route 503s,
	// which is expected in API-only dev mode where Vite serves the UI.
	SPADir string
}

func New(d Deps) *Server {
	return &Server{
		log:           d.Log,
		users:         d.Users,
		sessions:      d.Sessions,
		memberships:   d.Memberships,
		tenants:       d.Tenants,
		entities:      d.Entities,
		proposer:      d.Proposer,
		secureCookies: d.SecureCookies,
		spaDir:        d.SPADir,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Public
	mux.HandleFunc("POST /api/v1/auth/signup", s.signup)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
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
