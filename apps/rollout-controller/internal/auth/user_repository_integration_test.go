//go:build integration

package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/JRedCodes/rollout-controller/internal/dbtest"
	"github.com/JRedCodes/rollout-controller/internal/tenant"
)

func uniqueEmail(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("integration-test-%s@example.com", uuid.NewString())
}

func newUserRepository(t *testing.T) *UserRepository {
	t.Helper()
	pool := dbtest.Pool(t)
	return NewUserRepository(pool, tenant.NewRepository(pool))
}

func TestUserRepositorySignUpCreatesTenantAndUser(t *testing.T) {
	repo := newUserRepository(t)
	ctx := context.Background()

	email := uniqueEmail(t)
	user, plaintextKey, err := repo.SignUp(ctx, email, "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("SignUp: %v", err)
	}
	if user.Email != email {
		t.Fatalf("expected email %q, got %q", email, user.Email)
	}
	if user.TenantID == "" {
		t.Fatal("expected a non-empty tenant ID")
	}
	if plaintextKey == "" {
		t.Fatal("expected a non-empty plaintext tenant API key")
	}
}

func TestUserRepositorySignUpDuplicateEmail(t *testing.T) {
	repo := newUserRepository(t)
	ctx := context.Background()
	email := uniqueEmail(t)

	if _, _, err := repo.SignUp(ctx, email, "correct-horse-battery-staple"); err != nil {
		t.Fatalf("first SignUp: %v", err)
	}

	_, _, err := repo.SignUp(ctx, email, "a-different-password")
	if !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}

func TestUserRepositorySignUpNormalizesEmail(t *testing.T) {
	repo := newUserRepository(t)
	ctx := context.Background()

	raw := uniqueEmail(t)
	mixedCase := "  " + strings.ToUpper(raw) + "  "

	user, _, err := repo.SignUp(ctx, mixedCase, "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("SignUp: %v", err)
	}
	if user.Email != raw {
		t.Fatalf("expected normalized email %q, got %q", raw, user.Email)
	}

	// Signing in with the original casing/whitespace should still resolve
	// to the same, normalized account.
	signedIn, err := repo.SignIn(ctx, mixedCase, "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("SignIn with mixed-case email: %v", err)
	}
	if signedIn.ID != user.ID {
		t.Fatalf("expected the same user, got a different ID")
	}
}

func TestUserRepositorySignIn(t *testing.T) {
	repo := newUserRepository(t)
	ctx := context.Background()
	email := uniqueEmail(t)

	created, _, err := repo.SignUp(ctx, email, "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("SignUp: %v", err)
	}

	t.Run("correct credentials", func(t *testing.T) {
		user, err := repo.SignIn(ctx, email, "correct-horse-battery-staple")
		if err != nil {
			t.Fatalf("SignIn: %v", err)
		}
		if user.ID != created.ID {
			t.Fatalf("expected user ID %q, got %q", created.ID, user.ID)
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		_, err := repo.SignIn(ctx, email, "wrong-password")
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("expected ErrInvalidCredentials, got %v", err)
		}
	})

	t.Run("unknown email", func(t *testing.T) {
		_, err := repo.SignIn(ctx, uniqueEmail(t), "correct-horse-battery-staple")
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("expected ErrInvalidCredentials (not a distinguishable not-found), got %v", err)
		}
	})
}

func TestUserRepositoryGetByID(t *testing.T) {
	repo := newUserRepository(t)
	ctx := context.Background()

	created, _, err := repo.SignUp(ctx, uniqueEmail(t), "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("SignUp: %v", err)
	}

	t.Run("known id", func(t *testing.T) {
		user, err := repo.GetByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if user.Email != created.Email {
			t.Fatalf("expected email %q, got %q", created.Email, user.Email)
		}
	})

	t.Run("unknown id", func(t *testing.T) {
		_, err := repo.GetByID(ctx, uuid.NewString())
		if !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("expected ErrUserNotFound, got %v", err)
		}
	})
}
