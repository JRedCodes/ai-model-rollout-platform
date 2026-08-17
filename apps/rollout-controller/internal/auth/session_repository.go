package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JRedCodes/rollout-controller/internal/token"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrInvalidSession means the presented session token doesn't resolve to
// any non-expired session.
var ErrInvalidSession = errors.New("invalid or expired session")

// sessionDuration is how long a session stays valid before its owner has
// to sign in again.
const sessionDuration = 30 * 24 * time.Hour

type SessionRepository struct {
	pool *pgxpool.Pool
}

func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool}
}

// Create mints a new session for a user and returns the plaintext token to
// set as the session cookie's value -- only its hash is ever persisted.
func (r *SessionRepository) Create(ctx context.Context, userID string) (string, error) {
	plaintext, err := token.Generate()
	if err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}

	if _, err := r.pool.Exec(ctx, `
		INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)
	`, token.Hash(plaintext), userID, time.Now().Add(sessionDuration)); err != nil {
		return "", fmt.Errorf("insert session: %w", err)
	}

	return plaintext, nil
}

// UserIDForToken resolves a plaintext session token to a user ID, if a
// matching, non-expired session exists.
func (r *SessionRepository) UserIDForToken(ctx context.Context, plaintext string) (string, error) {
	var userID string
	err := r.pool.QueryRow(ctx, `
		SELECT user_id FROM sessions WHERE token_hash = $1 AND expires_at > NOW()
	`, token.Hash(plaintext)).Scan(&userID)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidSession
	}
	if err != nil {
		return "", fmt.Errorf("get session: %w", err)
	}
	return userID, nil
}

// Delete invalidates a session token -- sign-out.
func (r *SessionRepository) Delete(ctx context.Context, plaintext string) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, token.Hash(plaintext)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
