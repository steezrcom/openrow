package auth

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID              string
	Email           string
	Name            string
	EmailVerifiedAt *time.Time
	CreatedAt       time.Time
}

type UserService struct {
	pool *pgxpool.Pool
}

func NewUserService(pool *pgxpool.Pool) *UserService {
	return &UserService{pool: pool}
}

var (
	ErrUserExists         = errors.New("a user with that email already exists")
	ErrInvalidCredentials = errors.New("invalid email or password")
)

func normalizeEmail(raw string) (string, error) {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return "", errors.New("email is required")
	}
	if _, err := mail.ParseAddress(s); err != nil {
		return "", errors.New("email is not valid")
	}
	return s, nil
}

func (s *UserService) Signup(ctx context.Context, email, name, password string) (*User, error) {
	em, err := normalizeEmail(email)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("name is required")
	}
	if len(password) < 10 {
		return nil, errors.New("password must be at least 10 characters")
	}
	hash, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil {
		return nil, fmt.Errorf("hash: %w", err)
	}

	var u User
	err = s.pool.QueryRow(ctx, `
		INSERT INTO openrow.users (email, name, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, email, name, email_verified_at, created_at`,
		em, name, hash,
	).Scan(&u.ID, &u.Email, &u.Name, &u.EmailVerifiedAt, &u.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrUserExists
		}
		return nil, err
	}
	return &u, nil
}

func (s *UserService) Authenticate(ctx context.Context, email, password string) (*User, error) {
	em, err := normalizeEmail(email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	var (
		u    User
		hash string
	)
	err = s.pool.QueryRow(ctx, `
		SELECT id, email, name, email_verified_at, created_at, password_hash
		FROM openrow.users
		WHERE email = $1`, em,
	).Scan(&u.ID, &u.Email, &u.Name, &u.EmailVerifiedAt, &u.CreatedAt, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		_, _ = argon2id.ComparePasswordAndHash("timing-attack-shield", hash)
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	ok, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInvalidCredentials
	}
	return &u, nil
}

func (s *UserService) ByID(ctx context.Context, id string) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, name, email_verified_at, created_at
		FROM openrow.users
		WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.Name, &u.EmailVerifiedAt, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}
