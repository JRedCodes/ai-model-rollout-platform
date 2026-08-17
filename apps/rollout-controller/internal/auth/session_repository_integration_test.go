//go:build integration

package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JRedCodes/rollout-controller/internal/dbtest"
	"github.com/JRedCodes/rollout-controller/internal/tenant"
	"github.com/JRedCodes/rollout-controller/internal/token"
)

func TestSessionRepositoryCreateAndResolve(t *testing.T) {
	pool := dbtest.Pool(t)
	userRepo := NewUserRepository(pool, tenant.NewRepository(pool))
	sessionRepo := NewSessionRepository(pool)
	ctx := context.Background()

	user, _, err := userRepo.SignUp(ctx, uniqueEmail(t), "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("SignUp: %v", err)
	}

	plaintext, err := sessionRepo.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if plaintext == "" {
		t.Fatal("expected a non-empty session token")
	}

	gotUserID, err := sessionRepo.UserIDForToken(ctx, plaintext)
	if err != nil {
		t.Fatalf("UserIDForToken: %v", err)
	}
	if gotUserID != user.ID {
		t.Fatalf("expected user ID %q, got %q", user.ID, gotUserID)
	}
}

func TestSessionRepositoryUserIDForTokenInvalidToken(t *testing.T) {
	pool := dbtest.Pool(t)
	sessionRepo := NewSessionRepository(pool)

	_, err := sessionRepo.UserIDForToken(context.Background(), "not-a-real-token")
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("expected ErrInvalidSession, got %v", err)
	}
}

func TestSessionRepositoryDeleteInvalidatesSession(t *testing.T) {
	pool := dbtest.Pool(t)
	userRepo := NewUserRepository(pool, tenant.NewRepository(pool))
	sessionRepo := NewSessionRepository(pool)
	ctx := context.Background()

	user, _, err := userRepo.SignUp(ctx, uniqueEmail(t), "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("SignUp: %v", err)
	}

	plaintext, err := sessionRepo.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := sessionRepo.Delete(ctx, plaintext); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := sessionRepo.UserIDForToken(ctx, plaintext); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("expected the deleted session to be invalid, got %v", err)
	}
}

func TestSessionRepositoryExpiredSessionIsRejected(t *testing.T) {
	pool := dbtest.Pool(t)
	userRepo := NewUserRepository(pool, tenant.NewRepository(pool))
	sessionRepo := NewSessionRepository(pool)
	ctx := context.Background()

	user, _, err := userRepo.SignUp(ctx, uniqueEmail(t), "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("SignUp: %v", err)
	}

	// Insert an already-expired session directly -- SessionRepository has
	// no API for backdating one, and that's exactly the point: this
	// exercises the "expires_at > NOW()" check in UserIDForToken's query.
	plaintext, err := token.Generate()
	if err != nil {
		t.Fatalf("token.Generate: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)
	`, token.Hash(plaintext), user.ID, time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("insert expired session: %v", err)
	}

	if _, err := sessionRepo.UserIDForToken(ctx, plaintext); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("expected an expired session to be rejected, got %v", err)
	}
}
