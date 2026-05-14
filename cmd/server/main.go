package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/openrow/openrow/internal/ai"
	"github.com/openrow/openrow/internal/auth"
	"github.com/openrow/openrow/internal/config"
	"github.com/openrow/openrow/internal/connectors"
	_ "github.com/openrow/openrow/internal/connectors/catalog"
	"github.com/openrow/openrow/internal/connectors/catalog/flexi"
	"github.com/openrow/openrow/internal/connectors/discovery"
	"github.com/openrow/openrow/internal/docs"
	"github.com/openrow/openrow/internal/entities"
	"github.com/openrow/openrow/internal/flows"
	"github.com/openrow/openrow/internal/httpapi"
	"github.com/openrow/openrow/internal/llm"
	"github.com/openrow/openrow/internal/mailer"
	"github.com/openrow/openrow/internal/reports"
	"github.com/openrow/openrow/internal/secrets"
	"github.com/openrow/openrow/internal/store"
	"github.com/openrow/openrow/internal/tenant"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := store.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := store.Migrate(ctx, pool); err != nil {
		return err
	}
	log.Info("migrations applied")

	enc, err := secrets.NewFromEnv("OPENROW_SECRET_KEY")
	if err != nil {
		return err
	}

	entSvc := entities.NewService(pool)
	dashSvc := reports.NewService(pool)
	reportExec := reports.NewExecutor(pool, entSvc)
	llmSvc := llm.NewService(pool, enc, cfg.AnthropicAPIKey)
	connectorSvc := connectors.NewService(pool, enc)
	tenantSvc := tenant.NewService(pool)
	flowSvc := flows.NewService(pool, enc)
	agent := ai.NewAgent(llmSvc, entSvc, dashSvc, connectorSvc)
	agent.AddToolProvider(flows.ChatTools(flowSvc, connectorSvc))
	agent.AddToolProvider(docs.Provider(entSvc))
	flowRunner := flows.NewRunner(flowSvc, llmSvc, agent, tenantSvc)
	flowDispatcher := flows.NewInMemoryDispatcher(flowSvc, flowRunner, 4, 256, log)
	flowDispatcher.Start()
	defer flowDispatcher.Stop()

	discoveryRepo := discovery.NewRepo(pool)
	flexi.SetResolver(discoveryRepoResolver{repo: discoveryRepo})
	externalBindings := &httpapi.ExternalBindings{
		Log:      log,
		Repo:     discoveryRepo,
		Conns:    connectorSvc,
		LLM:      llmSvc,
		AllowInt: false,
	}

	// Hook entity mutations → flow triggers. Handler is synchronous but
	// fast: it only enqueues matched runs on the dispatcher.
	eventRouter := flows.NewEntityEventRouter(flowSvc, flowDispatcher, log)
	entSvc.SetChangeHandler(eventRouter.Handle)

	// Cron scheduler: one goroutine polling Postgres every 30s. Shut down
	// when the signal-aware ctx is cancelled.
	flowScheduler := flows.NewScheduler(flowSvc, flowDispatcher, log)
	flowScheduler.Start(ctx)

	secureCookies := !strings.EqualFold(strings.TrimSpace(os.Getenv("INSECURE_COOKIES_FOR_DEV")), "true")

	// Mailer selection: in dev (secure-cookies off) we use Stdout so the
	// developer can see the reset link inline; in any production-shaped
	// deployment we fall back to Noop, which logs recipient + subject only
	// and never the body. A future SMTP/SES/Resend mailer should slot in
	// here, driven by env config.
	var mail mailer.Mailer = &mailer.Noop{Log: log}
	if !secureCookies {
		mail = &mailer.Stdout{Log: log}
	} else {
		log.Warn("no mail provider configured; password resets will not be delivered. Wire a real Mailer for production.")
	}

	api := httpapi.New(httpapi.Deps{
		Log:              log,
		Users:            auth.NewUserService(pool),
		Sessions:         auth.NewSessionService(pool),
		Memberships:      auth.NewMembershipService(pool),
		PasswordResets:   auth.NewPasswordResetService(pool),
		Tenants:          tenantSvc,
		Entities:         entSvc,
		Dashboards:       dashSvc,
		ReportExec:       reportExec,
		Proposer:         ai.NewProposer(llmSvc),
		Agent:            agent,
		LLM:              llmSvc,
		Connectors:       connectorSvc,
		Flows:            flowSvc,
		FlowRunner:       flowRunner,
		FlowDispatcher:   flowDispatcher,
		Mailer:           mail,
		ExternalBindings: externalBindings,
		AppURL:           getOr("APP_URL", "http://localhost:5173"),
		SecureCookies:    secureCookies,
		SPADir:           os.Getenv("SPA_DIR"),
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-serverErr:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func getOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

type discoveryRepoResolver struct{ repo *discovery.Repo }

func (d discoveryRepoResolver) Resolve(ctx context.Context, tenantID, connectorID, role string) (string, map[string]string, error) {
	return d.repo.ResolveRole(ctx, tenantID, connectorID, role)
}
