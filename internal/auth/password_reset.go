package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const passwordResetLifetime = 1 * time.Hour

type PasswordResetService struct {
	pool *pgxpool.Pool
}

func NewPasswordResetService(pool *pgxpool.Pool) *PasswordResetService {
	return &PasswordResetService{pool: pool}
}

// Create generates a reset token for the user if one exists. Returns ("", nil) when the
// email is unknown so callers can respond identically to avoid account enumeration.
func (s *PasswordResetService) Create(ctx context.Context, email string) (token, userEmail, userName string, err error) {
	em, err := normalizeEmail(email)
	if err != nil {
		return "", "", "", nil
	}
	var userID string
	err = s.pool.QueryRow(ctx, `
		SELECT id, email, name FROM steezr.users WHERE email = $1`, em,
	).Scan(&userID, &userEmail, &userName)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", nil
	}
	if err != nil {
		return "", "", "", err
	}
	tok, err := newToken()
	if err != nil {
		return "", "", "", err
	}
	expires := time.Now().UTC().Add(passwordResetLifetime)
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO steezr.password_resets (token, user_id, expires_at) VALUES ($1, $2, $3)`,
		tok, userID, expires,
	); err != nil {
		return "", "", "", err
	}
	return tok, userEmail, userName, nil
}

// Consume validates a token and updates the user's password. Token is single-use.
func (s *PasswordResetService) Consume(ctx context.Context, token, newPassword string) error {
	if len(newPassword) < 10 {
		return fmt.Errorf("password must be at least 10 characters")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var (
		userID    string
		expiresAt time.Time
		usedAt    *time.Time
	)
	err = tx.QueryRow(ctx, `
		SELECT user_id, expires_at, used_at
		FROM steezr.password_resets
		WHERE token = $1`, token,
	).Scan(&userID, &expiresAt, &usedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("invalid or expired token")
	}
	if err != nil {
		return err
	}
	if usedAt != nil {
		return fmt.Errorf("token already used")
	}
	if time.Now().After(expiresAt) {
		return fmt.Errorf("invalid or expired token")
	}

	hash, err := argon2id.CreateHash(newPassword, argon2id.DefaultParams)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE steezr.users SET password_hash = $1, updated_at = now() WHERE id = $2`,
		hash, userID,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE steezr.password_resets SET used_at = now() WHERE token = $1`, token,
	); err != nil {
		return err
	}
	// Revoke every existing session for this user — reset always implies they lost control.
	if _, err := tx.Exec(ctx,
		`DELETE FROM steezr.sessions WHERE user_id = $1`, userID,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
