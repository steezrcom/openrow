package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	SessionCookie   = "steezr_session"
	sessionTokenLen = 32
	sessionLifetime = 30 * 24 * time.Hour
	refreshInterval = 6 * time.Hour
)

type Session struct {
	ID             string
	UserID         string
	ActiveTenantID *string
	CreatedAt      time.Time
	LastSeenAt     time.Time
	ExpiresAt      time.Time
}

type SessionService struct {
	pool *pgxpool.Pool
}

func NewSessionService(pool *pgxpool.Pool) *SessionService {
	return &SessionService{pool: pool}
}

func newToken() (string, error) {
	b := make([]byte, sessionTokenLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *SessionService) Create(ctx context.Context, userID string) (*Session, error) {
	id, err := newToken()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	expires := now.Add(sessionLifetime)
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO steezr.sessions (id, user_id, created_at, last_seen_at, expires_at)
		VALUES ($1, $2, $3, $3, $4)`,
		id, userID, now, expires); err != nil {
		return nil, err
	}
	return &Session{ID: id, UserID: userID, CreatedAt: now, LastSeenAt: now, ExpiresAt: expires}, nil
}

// Lookup fetches a valid (non-expired) session. Returns nil (no error) when missing/expired.
// If the session is older than refreshInterval it bumps last_seen_at and extends expires_at.
func (s *SessionService) Lookup(ctx context.Context, id string) (*Session, error) {
	if id == "" {
		return nil, nil
	}
	var sess Session
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, active_tenant_id, created_at, last_seen_at, expires_at
		FROM steezr.sessions
		WHERE id = $1`, id,
	).Scan(&sess.ID, &sess.UserID, &sess.ActiveTenantID, &sess.CreatedAt, &sess.LastSeenAt, &sess.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if now.After(sess.ExpiresAt) {
		_, _ = s.pool.Exec(ctx, `DELETE FROM steezr.sessions WHERE id = $1`, id)
		return nil, nil
	}
	if now.Sub(sess.LastSeenAt) > refreshInterval {
		newExpires := now.Add(sessionLifetime)
		if _, err := s.pool.Exec(ctx, `
			UPDATE steezr.sessions
			SET last_seen_at = $1, expires_at = $2
			WHERE id = $3`, now, newExpires, id); err != nil {
			return nil, err
		}
		sess.LastSeenAt = now
		sess.ExpiresAt = newExpires
	}
	return &sess, nil
}

func (s *SessionService) Delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM steezr.sessions WHERE id = $1`, id)
	return err
}

func (s *SessionService) SetActiveTenant(ctx context.Context, sessionID, tenantID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE steezr.sessions SET active_tenant_id = $1 WHERE id = $2`,
		tenantID, sessionID)
	return err
}

func (s *SessionService) Lifetime() time.Duration { return sessionLifetime }
