package mailer

import (
	"context"
	"log/slog"
)

type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

// Stdout writes the full message (recipient, subject, body) to slog at
// INFO. Convenient for local dev where you want to see the reset link
// without setting up SMTP. Never wire this in production — the body
// almost always contains a single-use credential.
type Stdout struct {
	Log *slog.Logger
}

func (s *Stdout) Send(_ context.Context, to, subject, body string) error {
	s.Log.Info("email", "to", to, "subject", subject, "body", body)
	return nil
}

// Noop is the production-safe placeholder used when no real mail
// provider has been wired. It records the recipient and subject so an
// operator can grep logs to see *that* a reset email would have gone
// out, but never the body — so credentials don't end up in log
// aggregators. Returns nil so the rest of the request flow continues
// (the user is unlikely to receive the reset email, but that's the
// operator's problem to fix by wiring a real provider).
type Noop struct {
	Log *slog.Logger
}

func (n *Noop) Send(_ context.Context, to, subject, _ string) error {
	if n.Log != nil {
		n.Log.Info("email suppressed (no mail provider configured)", "to", to, "subject", subject)
	}
	return nil
}
