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
	"github.com/openrow/openrow/internal/signedstate"
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
	oauthSigner, err := signedstate.New(enc.DeriveKey("openrow-oauth-state-v1"))
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

	// Mailer selection — SMTP if configured, else Stdout in dev, else Noop.
	mail := pickMailer(log, secureCookies)

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
		OAuthSigner:      oauthSigner,
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

// pickMailer wires the mailer based on env. The decision tree:
//
//  1. SMTP_HOST is set → build an SMTP mailer. Required: SMTP_HOST,
//     SMTP_FROM. Optional: SMTP_PORT (default 587), SMTP_USERNAME,
//     SMTP_PASSWORD, SMTP_TLS_IMPLICIT (true for port 465).
//  2. Else, secureCookies on (production-shaped) → Noop, which logs
//     recipient + subject but never body, so password reset URLs don't
//     end up in log aggregators when no SMTP is configured.
//  3. Else (dev) → Stdout, which logs the whole message including body
//     so the developer can grab the reset link from terminal output.
func pickMailer(log *slog.Logger, secureCookies bool) mailer.Mailer {
	if host := strings.TrimSpace(os.Getenv("SMTP_HOST")); host != "" {
		port := strings.TrimSpace(os.Getenv("SMTP_PORT"))
		if port == "" {
			port = "587"
		}
		from := strings.TrimSpace(os.Getenv("SMTP_FROM"))
		if from == "" {
			log.Error("SMTP_HOST is set but SMTP_FROM is empty; falling back to Noop mailer")
			return &mailer.Noop{Log: log}
		}
		m, err := mailer.NewSMTP(mailer.SMTPConfig{
			Host:        host,
			Port:        port,
			Username:    os.Getenv("SMTP_USERNAME"),
			Password:    os.Getenv("SMTP_PASSWORD"),
			From:        from,
			TLSImplicit: strings.EqualFold(os.Getenv("SMTP_TLS_IMPLICIT"), "true"),
			Log:         log,
		})
		if err != nil {
			log.Error("smtp mailer init failed; falling back to Noop", "err", err)
			return &mailer.Noop{Log: log}
		}
		log.Info("smtp mailer configured", "host", host, "port", port, "from", from)
		return m
	}
	if secureCookies {
		log.Warn("no SMTP_HOST set; password reset emails will not be delivered. Set SMTP_* to wire a real provider.")
		return &mailer.Noop{Log: log}
	}
	return &mailer.Stdout{Log: log}
}

type discoveryRepoResolver struct{ repo *discovery.Repo }

func (d discoveryRepoResolver) Resolve(ctx context.Context, tenantID, connectorID, role string) (string, map[string]string, error) {
	return d.repo.ResolveRole(ctx, tenantID, connectorID, role)
}
