package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/JRedCodes/rollout-controller/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrEmailTaken means a user already exists with the presented email.
	ErrEmailTaken = errors.New("email already registered")
	// ErrInvalidCredentials covers both "no such user" and "wrong
	// password" -- deliberately not distinguished, so a failed sign-in
	// doesn't reveal whether an email is registered.
	ErrInvalidCredentials = errors.New("invalid email or password")
	// ErrUserNotFound means a user ID (e.g. from a session) doesn't
	// resolve to any user.
	ErrUserNotFound = errors.New("user not found")
)

type User struct {
	ID       string
	TenantID string
	Email    string
}

type UserRepository struct {
	pool       *pgxpool.Pool
	tenantRepo *tenant.Repository
}

func NewUserRepository(pool *pgxpool.Pool, tenantRepo *tenant.Repository) *UserRepository {
	return &UserRepository{pool: pool, tenantRepo: tenantRepo}
}

// SignUp creates a tenant and the user that owns it, atomically -- mirrors
// tenant.Repository.Create's shape, but also inserts the users row in the
// same transaction. Returns the new user and the tenant's plaintext API
// key, shown here exactly once (same invariant as tenant.Repository.Create
// — only its hash is ever persisted).
func (r *UserRepository) SignUp(ctx context.Context, email, password string) (User, string, error) {
	email = normalizeEmail(email)

	passwordHash, err := HashPassword(password)
	if err != nil {
		return User{}, "", fmt.Errorf("hash password: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, "", fmt.Errorf("begin signup tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	t, plaintextKey, err := r.tenantRepo.CreateTx(ctx, tx, email)
	if err != nil {
		return User{}, "", fmt.Errorf("create tenant: %w", err)
	}

	userID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, tenant_id, email, password_hash) VALUES ($1, $2, $3, $4)
	`, userID, t.ID, email, passwordHash); err != nil {
		if isUniqueViolation(err) {
			return User{}, "", ErrEmailTaken
		}
		return User{}, "", fmt.Errorf("insert user: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return User{}, "", fmt.Errorf("commit signup tx: %w", err)
	}

	return User{ID: userID, TenantID: t.ID, Email: email}, plaintextKey, nil
}

// SignIn verifies credentials and returns the matching user.
func (r *UserRepository) SignIn(ctx context.Context, email, password string) (User, error) {
	email = normalizeEmail(email)

	var u User
	var passwordHash string
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, email, password_hash FROM users WHERE email = $1
	`, email).Scan(&u.ID, &u.TenantID, &u.Email, &passwordHash)

	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, fmt.Errorf("get user by email: %w", err)
	}

	if !VerifyPassword(passwordHash, password) {
		return User{}, ErrInvalidCredentials
	}

	return u, nil
}

// GetByID looks up a user by ID -- used to resolve a session's user_id
// into the account details GET /auth/me returns.
func (r *UserRepository) GetByID(ctx context.Context, id string) (User, error) {
	var u User
	err := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, email FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.TenantID, &u.Email)

	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("get user by id: %w", err)
	}
	return u, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
