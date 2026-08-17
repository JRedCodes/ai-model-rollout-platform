package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/JRedCodes/rollout-controller/internal/auth"
)

// sessionCookieName is the httpOnly cookie the dashboard authenticates
// with. Bearer tokens (stress-tester, curl, scripted callers) are
// unaffected -- see authMiddleware's dual-path resolution.
const sessionCookieName = "session_token"

const userIDContextKey contextKey = iota + 1 // tenantIDContextKey is iota 0

func userIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(userIDContextKey).(string)
	return id
}

type signInUpRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// handleSignUp creates a tenant and the user that owns it, starts a
// session, and returns the tenant's plaintext API key -- shown here
// exactly once, same invariant as handleCreateTenant, so the dashboard can
// display it for the visitor to copy into the stress-tester CLI.
func (s *Server) handleSignUp(w http.ResponseWriter, r *http.Request) {
	var body signInUpRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Email == "" || body.Password == "" {
		http.Error(w, "email and password are required", http.StatusBadRequest)
		return
	}
	if len(body.Password) < 8 {
		http.Error(w, "password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	u, plaintextKey, err := s.userRepo.SignUp(r.Context(), body.Email, body.Password)
	if err != nil {
		if errors.Is(err, auth.ErrEmailTaken) {
			http.Error(w, "email already registered", http.StatusConflict)
			return
		}
		log.Printf("api: signup: %v", err)
		http.Error(w, "failed to sign up", http.StatusInternalServerError)
		return
	}

	if err := s.tenantPub.Publish(r.Context(), u.TenantID, plaintextKey); err != nil {
		log.Printf("api: failed to publish tenant auth to redis: %v", err)
		http.Error(w, "created but failed to propagate auth to redis", http.StatusInternalServerError)
		return
	}

	if err := s.startSession(w, r.Context(), u.ID); err != nil {
		log.Printf("api: signup: create session: %v", err)
		http.Error(w, "signed up but failed to start session", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":     u.ID,
		"email":  u.Email,
		"apiKey": plaintextKey,
	})
}

func (s *Server) handleSignIn(w http.ResponseWriter, r *http.Request) {
	var body signInUpRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	u, err := s.userRepo.SignIn(r.Context(), body.Email, body.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			http.Error(w, "invalid email or password", http.StatusUnauthorized)
			return
		}
		log.Printf("api: signin: %v", err)
		http.Error(w, "failed to sign in", http.StatusInternalServerError)
		return
	}

	if err := s.startSession(w, r.Context(), u.ID); err != nil {
		log.Printf("api: signin: create session: %v", err)
		http.Error(w, "signed in but failed to start session", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"id": u.ID, "email": u.Email})
}

func (s *Server) handleSignOut(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		if err := s.sessionRepo.Delete(r.Context(), c.Value); err != nil {
			log.Printf("api: signout: %v", err)
		}
	}
	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "signed out"})
}

// handleMe backs the dashboard's "am I signed in" check. httpOnly cookies
// aren't readable from JS, so this round-trip is how the dashboard learns
// its session is (still) valid, instead of trusting client-side state.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	if userID == "" {
		http.Error(w, "not signed in", http.StatusUnauthorized)
		return
	}

	u, err := s.userRepo.GetByID(r.Context(), userID)
	if err != nil {
		log.Printf("api: me: %v", err)
		http.Error(w, "failed to load account", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"id": u.ID, "email": u.Email})
}

// handleRegenerateKey invalidates the signed-in user's tenant's current API
// key and mints a new one, shown once -- the supported way to get a fresh
// key for the stress-tester CLI without weakening the "hash-only storage"
// invariant by making the original key retrievable later.
func (s *Server) handleRegenerateKey(w http.ResponseWriter, r *http.Request) {
	if userIDFromContext(r.Context()) == "" {
		http.Error(w, "not signed in", http.StatusUnauthorized)
		return
	}
	tenantID := tenantIDFromContext(r.Context())

	plaintextKey, err := s.tenantRepo.RegenerateAPIKey(r.Context(), tenantID)
	if err != nil {
		log.Printf("api: regenerate key: %v", err)
		http.Error(w, "failed to regenerate api key", http.StatusInternalServerError)
		return
	}

	if err := s.tenantPub.Publish(r.Context(), tenantID, plaintextKey); err != nil {
		log.Printf("api: failed to publish tenant auth to redis: %v", err)
		http.Error(w, "regenerated but failed to propagate auth to redis", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"apiKey": plaintextKey})
}

func (s *Server) startSession(w http.ResponseWriter, ctx context.Context, userID string) error {
	plaintext, err := s.sessionRepo.Create(ctx, userID)
	if err != nil {
		return err
	}

	cookie := s.sessionCookie(plaintext, int(auth.SessionDuration.Seconds()))
	http.SetCookie(w, &cookie)
	return nil
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	cookie := s.sessionCookie("", -1)
	http.SetCookie(w, &cookie)
}

// sessionCookie builds the session cookie, with Secure/SameSite driven by
// cookieSecure (COOKIE_SECURE env). Locally (false) that's Lax without
// Secure -- fetch()/XHR from another origin never attaches a Lax cookie,
// which is the CSRF mitigation for the mutating endpoints, no separate
// token needed for a same-site SPA. In a cross-origin deployment (true,
// e.g. CloudFront dashboard + ALB API per DEPLOYMENT.md) SameSite=None is
// required for the cookie to be sent cross-site at all, and browsers only
// honor SameSite=None when Secure is also set.
func (s *Server) sessionCookie(value string, maxAgeSeconds int) http.Cookie {
	sameSite := http.SameSiteLaxMode
	if s.cookieSecure {
		sameSite = http.SameSiteNoneMode
	}

	return http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAgeSeconds,
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: sameSite,
	}
}
