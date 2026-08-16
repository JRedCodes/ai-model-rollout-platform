package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/JRedCodes/rollout-controller/internal/db"
	"github.com/JRedCodes/rollout-controller/internal/metrics"
	"github.com/JRedCodes/rollout-controller/internal/modelconfig"
	"github.com/JRedCodes/rollout-controller/internal/tenant"
	"github.com/JRedCodes/rollout-controller/internal/writer"
)

type Server struct {
	pipelines            *PipelineRegistry
	repo                 *db.RolloutRepository
	hubs                 *SSEHubRegistry
	modelConfigRepo      *modelconfig.Repository
	modelConfigPub       *modelconfig.Seeder
	tenantRepo           *tenant.Repository
	tenantPub            *tenant.Seeder
	adminAPIKey          string
	featureFlagKeyPrefix string
	httpServer           *http.Server
}

func New(
	port int,
	pipelines *PipelineRegistry,
	repo *db.RolloutRepository,
	hubs *SSEHubRegistry,
	modelConfigRepo *modelconfig.Repository,
	modelConfigPub *modelconfig.Seeder,
	tenantRepo *tenant.Repository,
	tenantPub *tenant.Seeder,
	adminAPIKey string,
	featureFlagKeyPrefix string,
) *Server {
	s := &Server{
		pipelines:            pipelines,
		repo:                 repo,
		hubs:                 hubs,
		modelConfigRepo:      modelConfigRepo,
		modelConfigPub:       modelConfigPub,
		tenantRepo:           tenantRepo,
		tenantPub:            tenantPub,
		adminAPIKey:          adminAPIKey,
		featureFlagKeyPrefix: featureFlagKeyPrefix,
	}

	// Unauthenticated: health check, tenant bootstrapping (gated by
	// ADMIN_API_KEY instead), and SSE -- browsers' EventSource can't send
	// custom headers at all, so it authenticates itself via a query param
	// instead of going through authMiddleware. See handleEvents.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /tenants", s.handleCreateTenant)
	mux.HandleFunc("GET /events", s.handleEvents)

	// Every other route requires a valid tenant API key via the Authorization header.
	authed := http.NewServeMux()
	authed.HandleFunc("GET /rollout", s.handleGetRollout)
	authed.HandleFunc("GET /rollout/metrics", s.handleGetMetrics)
	authed.HandleFunc("GET /rollout/decisions", s.handleGetDecisions)
	authed.HandleFunc("POST /rollout/rollback", s.handleRollback)
	authed.HandleFunc("GET /rollouts", s.handleListRollouts)
	authed.HandleFunc("GET /rollouts/{id}", s.handleGetRolloutByID)
	authed.HandleFunc("POST /rollouts", s.handleCreateRollout)
	authed.HandleFunc("GET /models", s.handleListModels)
	authed.HandleFunc("GET /models/{id}", s.handleGetModel)
	authed.HandleFunc("PUT /models/{id}", s.handleUpdateModel)
	mux.Handle("/", s.authMiddleware(authed))

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: corsMiddleware(mux),
	}

	return s
}

type contextKey int

const tenantIDContextKey contextKey = iota

// authMiddleware resolves "Authorization: Bearer <key>" to a tenant ID and
// stores it in the request context for handlers to read via
// tenantIDFromContext.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		tenantID, err := s.resolveAPIKey(r.Context(), key)
		if err != nil {
			writeAuthError(w, err)
			return
		}

		ctx := context.WithValue(r.Context(), tenantIDContextKey, tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// resolveAPIKey looks up a tenant ID via Postgres (the Go service has
// direct DB access, unlike the Edge Evaluator, so it doesn't need the
// Redis tenant-auth cache).
func (s *Server) resolveAPIKey(ctx context.Context, key string) (string, error) {
	if key == "" {
		return "", errMissingAPIKey
	}
	return s.tenantRepo.GetIDByAPIKey(ctx, key)
}

var errMissingAPIKey = errors.New("missing api key")

func writeAuthError(w http.ResponseWriter, err error) {
	if errors.Is(err, errMissingAPIKey) {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}
	if errors.Is(err, tenant.ErrInvalidAPIKey) {
		http.Error(w, "invalid api key", http.StatusUnauthorized)
		return
	}
	log.Printf("api: auth lookup failed: %v", err)
	http.Error(w, "authentication failed", http.StatusInternalServerError)
}

func tenantIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(tenantIDContextKey).(string)
	return id
}

func (s *Server) Run(ctx context.Context) {
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("api: shutdown error: %v", err)
		}
	}()

	log.Printf("api: listening on %s", s.httpServer.Addr)

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("api: server error: %v", err)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type createTenantRequest struct {
	Name string `json:"name"`
}

// handleCreateTenant is the one bootstrapping endpoint gated by a single
// shared ADMIN_API_KEY rather than a per-tenant key -- there's no tenant
// key to present yet, since this is what issues the first one. Every other
// route is gated by authMiddleware instead.
func (s *Server) handleCreateTenant(w http.ResponseWriter, r *http.Request) {
	presented := r.Header.Get("X-Admin-Key")
	if subtle.ConstantTimeCompare([]byte(presented), []byte(s.adminAPIKey)) != 1 {
		http.Error(w, "invalid admin key", http.StatusUnauthorized)
		return
	}

	var body createTenantRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	t, plaintextKey, err := s.tenantRepo.Create(r.Context(), body.Name)
	if err != nil {
		log.Printf("api: create tenant: %v", err)
		http.Error(w, "failed to create tenant", http.StatusInternalServerError)
		return
	}

	if err := s.tenantPub.Publish(r.Context(), t.ID, plaintextKey); err != nil {
		log.Printf("api: failed to publish tenant auth to redis: %v", err)
		http.Error(w, "created but failed to propagate auth to redis", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":     t.ID,
		"name":   t.Name,
		"apiKey": plaintextKey,
	})
}

func (s *Server) handleGetRollout(w http.ResponseWriter, r *http.Request) {
	p := s.pipelines.Get(tenantIDFromContext(r.Context()))
	if p == nil {
		writeJSON(w, http.StatusOK, map[string]any{"active": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"active":                  true,
		"rolloutId":               p.Cfg.RolloutID,
		"rolloutPhaseId":          p.Cfg.RolloutPhaseID,
		"stableModelVersionId":    p.Cfg.StableModelVersionID,
		"candidateModelVersionId": p.Cfg.CandidateModelVersionID,
		"candidatePercentage":     p.Writer.CurrentPercentage(),
		"held":                    p.Writer.IsHeld(),
	})
}

func (s *Server) handleGetMetrics(w http.ResponseWriter, r *http.Request) {
	p := s.pipelines.Get(tenantIDFromContext(r.Context()))
	if p == nil {
		writeJSON(w, http.StatusOK, map[string]any{"active": false})
		return
	}

	total := p.Store.TotalCount()
	all := p.Store.LastN(total)
	window := p.Store.Since(time.Now().Add(-2 * time.Minute))

	writeJSON(w, http.StatusOK, map[string]any{
		"active":             true,
		"totalRequests":      total,
		"overallErrorRate":   metrics.ErrorRate(all),
		"windowRequestCount": len(window),
		"windowErrorRate":    metrics.ErrorRate(window),
		"windowP95LatencyMs": metrics.P95Latency(window),
	})
}

func (s *Server) handleGetDecisions(w http.ResponseWriter, r *http.Request) {
	p := s.pipelines.Get(tenantIDFromContext(r.Context()))
	if p == nil {
		writeJSON(w, http.StatusOK, []db.Decision{})
		return
	}

	decisions, err := s.repo.ListDecisions(r.Context(), p.Cfg.RolloutID, 50)
	if err != nil {
		log.Printf("api: list decisions: %v", err)
		http.Error(w, "failed to load decisions", http.StatusInternalServerError)
		return
	}
	if decisions == nil {
		decisions = []db.Decision{}
	}
	writeJSON(w, http.StatusOK, decisions)
}

func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	profiles, err := s.modelConfigRepo.List(r.Context(), tenantIDFromContext(r.Context()))
	if err != nil {
		log.Printf("api: list model configs: %v", err)
		http.Error(w, "failed to load model configurations", http.StatusInternalServerError)
		return
	}
	if profiles == nil {
		profiles = []modelconfig.Profile{}
	}
	writeJSON(w, http.StatusOK, profiles)
}

func (s *Server) handleGetModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	profile, err := s.modelConfigRepo.Get(r.Context(), tenantIDFromContext(r.Context()), id)
	if err != nil {
		s.writeModelConfigError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

type updateModelRequest struct {
	FailureRate  *float64 `json:"failureRate"`
	MinLatencyMs *int     `json:"minLatencyMs"`
	MaxLatencyMs *int     `json:"maxLatencyMs"`
}

func (s *Server) handleUpdateModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var body updateModelRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if body.FailureRate == nil || body.MinLatencyMs == nil || body.MaxLatencyMs == nil {
		http.Error(w, "failureRate, minLatencyMs, and maxLatencyMs are required", http.StatusBadRequest)
		return
	}
	if *body.FailureRate < 0 || *body.FailureRate > 1 {
		http.Error(w, "failureRate must be between 0 and 1", http.StatusBadRequest)
		return
	}
	if *body.MinLatencyMs <= 0 || *body.MaxLatencyMs <= 0 {
		http.Error(w, "minLatencyMs and maxLatencyMs must be positive", http.StatusBadRequest)
		return
	}
	if *body.MinLatencyMs > *body.MaxLatencyMs {
		http.Error(w, "minLatencyMs must be <= maxLatencyMs", http.StatusBadRequest)
		return
	}

	profile, err := s.modelConfigRepo.Update(r.Context(), tenantIDFromContext(r.Context()), id, *body.FailureRate, *body.MinLatencyMs, *body.MaxLatencyMs)
	if err != nil {
		s.writeModelConfigError(w, err)
		return
	}

	if err := s.modelConfigPub.Publish(r.Context(), profile); err != nil {
		log.Printf("api: failed to publish model config to redis: %v", err)
		http.Error(w, "saved but failed to propagate to redis", http.StatusInternalServerError)
		return
	}

	s.hubs.Get(tenantIDFromContext(r.Context())).Broadcast("model-config-updated", profile)
	writeJSON(w, http.StatusOK, profile)
}

func (s *Server) writeModelConfigError(w http.ResponseWriter, err error) {
	if notFound, ok := errors.AsType[*modelconfig.NotFoundError](err); ok {
		http.Error(w, notFound.Error(), http.StatusNotFound)
		return
	}
	log.Printf("api: model config error: %v", err)
	http.Error(w, "failed to load model configuration", http.StatusInternalServerError)
}

func (s *Server) handleRollback(w http.ResponseWriter, r *http.Request) {
	p := s.pipelines.Get(tenantIDFromContext(r.Context()))
	if p == nil {
		http.Error(w, "no active rollout", http.StatusConflict)
		return
	}
	p.Writer.Commands <- writer.Command{
		Type:   writer.CmdRollback,
		Reason: "manual rollback via API",
		Source: "manual",
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "rollback initiated"})
}

func (s *Server) handleListRollouts(w http.ResponseWriter, r *http.Request) {
	rollouts, err := s.repo.ListRollouts(r.Context(), tenantIDFromContext(r.Context()))
	if err != nil {
		log.Printf("api: list rollouts: %v", err)
		http.Error(w, "failed to load rollouts", http.StatusInternalServerError)
		return
	}
	if rollouts == nil {
		rollouts = []db.Rollout{}
	}
	writeJSON(w, http.StatusOK, rollouts)
}

func (s *Server) handleGetRolloutByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	rollout, err := s.repo.GetRollout(r.Context(), tenantIDFromContext(r.Context()), id)
	if err != nil {
		if notFound, ok := errors.AsType[*db.RolloutNotFoundError](err); ok {
			http.Error(w, notFound.Error(), http.StatusNotFound)
			return
		}
		log.Printf("api: get rollout: %v", err)
		http.Error(w, "failed to load rollout", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, rollout)
}

type createRolloutRequest struct {
	RolloutPhaseID          string  `json:"rolloutPhaseId"`
	StableModelVersionID    *string `json:"stableModelVersionId"`
	CandidateModelVersionID string  `json:"candidateModelVersionId"`
	CandidatePercentage     *int    `json:"candidatePercentage"`
}

// handleCreateRollout creates a new rollout, replacing the old manual psql
// INSERT workflow. If stableModelVersionId is omitted, it defaults to the
// most recently COMPLETED rollout's candidate — the "promote candidate to
// stable, prepare next rollout slot" behavior.
func (s *Server) handleCreateRollout(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())

	var body createRolloutRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if body.RolloutPhaseID == "" {
		http.Error(w, "rolloutPhaseId is required", http.StatusBadRequest)
		return
	}
	if body.CandidateModelVersionID == "" {
		http.Error(w, "candidateModelVersionId is required", http.StatusBadRequest)
		return
	}

	percentage := 10
	if body.CandidatePercentage != nil {
		if *body.CandidatePercentage < 0 || *body.CandidatePercentage > 100 {
			http.Error(w, "candidatePercentage must be between 0 and 100", http.StatusBadRequest)
			return
		}
		percentage = *body.CandidatePercentage
	}

	if _, err := s.modelConfigRepo.Get(r.Context(), tenantID, body.CandidateModelVersionID); err != nil {
		s.writeModelConfigError(w, err)
		return
	}

	stableID := ""
	if body.StableModelVersionID != nil && *body.StableModelVersionID != "" {
		stableID = *body.StableModelVersionID
	} else {
		promoted, err := s.repo.LatestCompletedCandidate(r.Context(), tenantID)
		if err != nil {
			if errors.Is(err, db.ErrNoCompletedRollout) {
				http.Error(w, "stableModelVersionId is required (no completed rollout to default it from)", http.StatusBadRequest)
				return
			}
			log.Printf("api: latest completed candidate: %v", err)
			http.Error(w, "failed to resolve stable model version", http.StatusInternalServerError)
			return
		}
		stableID = promoted
	}

	if _, err := s.modelConfigRepo.Get(r.Context(), tenantID, stableID); err != nil {
		s.writeModelConfigError(w, err)
		return
	}

	rollout, err := s.repo.CreateRollout(r.Context(), db.CreateRolloutInput{
		TenantID:                tenantID,
		RolloutPhaseID:          body.RolloutPhaseID,
		StableModelVersionID:    stableID,
		CandidateModelVersionID: body.CandidateModelVersionID,
		CandidatePercentage:     percentage,
		FeatureFlagKey:          s.featureFlagKeyPrefix + tenantID,
	})
	if err != nil {
		if errors.Is(err, db.ErrActiveRolloutExists) {
			http.Error(w, "an active rollout already exists", http.StatusConflict)
			return
		}
		log.Printf("api: create rollout: %v", err)
		http.Error(w, "failed to create rollout", http.StatusInternalServerError)
		return
	}

	s.hubs.Get(tenantID).Broadcast("rollout-created", rollout)
	writeJSON(w, http.StatusCreated, rollout)
}

// handleEvents serves the SSE stream for the authenticated tenant's own
// hub. Not behind authMiddleware: browsers' EventSource can't send custom
// headers, so this accepts the API key as either the usual Authorization
// header or an api_key query param, whichever is present.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if key == "" {
		key = r.URL.Query().Get("api_key")
	}

	tenantID, err := s.resolveAPIKey(r.Context(), key)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	s.hubs.Get(tenantID).ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: failed to encode response: %v", err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
