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
	agent := ai.NewAgent(llmSvc, entSvc, dashSvc)
	flowSvc := flows.NewService(pool)
	flowRunner := flows.NewRunner(flowSvc, llmSvc, agent, tenantSvc)

	api := httpapi.New(httpapi.Deps{
		Log:            log,
		Users:          auth.NewUserService(pool),
		Sessions:       auth.NewSessionService(pool),
		Memberships:    auth.NewMembershipService(pool),
		PasswordResets: auth.NewPasswordResetService(pool),
		Tenants:        tenantSvc,
		Entities:       entSvc,
		Dashboards:     dashSvc,
		ReportExec:     reportExec,
		Proposer:       ai.NewProposer(llmSvc),
		Agent:          agent,
		LLM:            llmSvc,
		Connectors:     connectorSvc,
		Flows:          flowSvc,
		FlowRunner:     flowRunner,
		Mailer:         &mailer.Stdout{Log: log},
		AppURL:         getOr("APP_URL", "http://localhost:5173"),
		SecureCookies:  strings.EqualFold(os.Getenv("SECURE_COOKIES"), "true"),
		SPADir:         os.Getenv("SPA_DIR"),
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
