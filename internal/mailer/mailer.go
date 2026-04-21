package mailer

import (
	"context"
	"log/slog"
)

type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

// Stdout implementation — good for dev; swap for a real provider in prod.
type Stdout struct {
	Log *slog.Logger
}

func (s *Stdout) Send(_ context.Context, to, subject, body string) error {
	s.Log.Info("email", "to", to, "subject", subject, "body", body)
	return nil
}
