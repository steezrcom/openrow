package mailer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// SMTPConfig is the minimum needed to send via SMTP.
type SMTPConfig struct {
	Host         string
	Port         string // "587" (STARTTLS), "465" (implicit TLS), "25" (no TLS — dev only)
	Username     string // leave empty to disable auth
	Password     string
	From         string // either "you@example.com" or "Name <you@example.com>"
	TLSImplicit  bool   // true for port 465; false for STARTTLS/cleartext
	Log          *slog.Logger
}

// SMTPMailer sends a single message synchronously. Each call opens and
// closes its own connection — fine for password-reset volumes; a higher
// throughput sender should pool connections.
type SMTPMailer struct {
	cfg SMTPConfig
}

func NewSMTP(cfg SMTPConfig) (*SMTPMailer, error) {
	if cfg.Host == "" || cfg.Port == "" {
		return nil, errors.New("smtp: host and port are required")
	}
	if cfg.From == "" {
		return nil, errors.New("smtp: from address is required")
	}
	return &SMTPMailer{cfg: cfg}, nil
}

// Send delivers a UTF-8 plain-text e-mail. STARTTLS is required unless
// explicit TLS is on or the host resolves to localhost (dev convenience).
func (s *SMTPMailer) Send(ctx context.Context, to, subject, body string) error {
	if to == "" {
		return errors.New("smtp: empty recipient")
	}
	addr := net.JoinHostPort(s.cfg.Host, s.cfg.Port)
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(30 * time.Second)
	}
	dialer := &net.Dialer{Deadline: deadline}

	var conn net.Conn
	var err error
	if s.cfg.TLSImplicit {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
			ServerName: s.cfg.Host,
			MinVersion: tls.VersionTLS12,
		})
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	defer conn.Close()

	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer c.Close()

	if !s.cfg.TLSImplicit {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return fmt.Errorf("starttls: %w", err)
			}
		} else if !isLoopback(s.cfg.Host) {
			// In production we refuse to send credentials over cleartext.
			// Dev-only setups can reach a local relay on 127.0.0.1.
			return errors.New("smtp: server does not advertise STARTTLS and host is not loopback")
		}
	}

	if s.cfg.Username != "" {
		auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	from := extractAddress(s.cfg.From)
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	msg := buildMessage(s.cfg.From, to, subject, body)
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}
	if err := c.Quit(); err != nil {
		// QUIT failure isn't worth surfacing; the message was already accepted.
		if s.cfg.Log != nil {
			s.cfg.Log.Warn("smtp quit", "err", err)
		}
	}
	if s.cfg.Log != nil {
		s.cfg.Log.Info("email sent", "to", to, "subject", subject)
	}
	return nil
}

// buildMessage assembles an RFC 5322 message with a Date and Message-ID.
// Body is treated as UTF-8 plain text; long lines aren't folded — fine
// for transactional links.
func buildMessage(from, to, subject, body string) []byte {
	var b strings.Builder
	b.WriteString("From: ")
	b.WriteString(from)
	b.WriteString("\r\n")
	b.WriteString("To: ")
	b.WriteString(to)
	b.WriteString("\r\n")
	b.WriteString("Subject: ")
	b.WriteString(subject)
	b.WriteString("\r\n")
	b.WriteString("Date: ")
	b.WriteString(time.Now().UTC().Format(time.RFC1123Z))
	b.WriteString("\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

// extractAddress turns "Name <addr@example.com>" into "addr@example.com".
// Plain addresses pass through unchanged.
func extractAddress(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "<"); i >= 0 {
		if j := strings.LastIndex(s, ">"); j > i {
			return strings.TrimSpace(s[i+1 : j])
		}
	}
	return s
}

func isLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
