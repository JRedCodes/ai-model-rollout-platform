package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/JRedCodes/rollout-controller/internal/config"
	"github.com/JRedCodes/rollout-controller/internal/db"
	"github.com/JRedCodes/rollout-controller/internal/metrics"
	"github.com/JRedCodes/rollout-controller/internal/modelconfig"
	"github.com/JRedCodes/rollout-controller/internal/writer"
)

type Server struct {
	rolloutCfg      config.RolloutConfig
	store           *metrics.Store
	w               *writer.Writer
	repo            *db.RolloutRepository
	hub             *SSEHub
	modelConfigRepo *modelconfig.Repository
	modelConfigPub  *modelconfig.Seeder
	httpServer      *http.Server
}

func New(
	port int,
	rolloutCfg config.RolloutConfig,
	store *metrics.Store,
	w *writer.Writer,
	repo *db.RolloutRepository,
	hub *SSEHub,
	modelConfigRepo *modelconfig.Repository,
	modelConfigPub *modelconfig.Seeder,
) *Server {
	s := &Server{
		rolloutCfg:      rolloutCfg,
		store:           store,
		w:               w,
		repo:            repo,
		hub:             hub,
		modelConfigRepo: modelConfigRepo,
		modelConfigPub:  modelConfigPub,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /rollout", s.handleGetRollout)
	mux.HandleFunc("GET /rollout/metrics", s.handleGetMetrics)
	mux.HandleFunc("GET /rollout/decisions", s.handleGetDecisions)
	mux.HandleFunc("POST /rollout/rollback", s.handleRollback)
	mux.HandleFunc("GET /models", s.handleListModels)
	mux.HandleFunc("GET /models/{id}", s.handleGetModel)
	mux.HandleFunc("PUT /models/{id}", s.handleUpdateModel)
	mux.Handle("GET /events", hub)

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: corsMiddleware(mux),
	}

	return s
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

func (s *Server) handleGetRollout(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"rolloutId":               s.rolloutCfg.RolloutID,
		"rolloutPhaseId":          s.rolloutCfg.RolloutPhaseID,
		"stableModelVersionId":    s.rolloutCfg.StableModelVersionID,
		"candidateModelVersionId": s.rolloutCfg.CandidateModelVersionID,
		"candidatePercentage":     s.w.CurrentPercentage(),
		"held":                    s.w.IsHeld(),
	})
}

func (s *Server) handleGetMetrics(w http.ResponseWriter, r *http.Request) {
	total := s.store.TotalCount()
	all := s.store.LastN(total)
	window := s.store.Since(time.Now().Add(-2 * time.Minute))

	writeJSON(w, http.StatusOK, map[string]any{
		"totalRequests":      total,
		"overallErrorRate":   metrics.ErrorRate(all),
		"windowRequestCount": len(window),
		"windowErrorRate":    metrics.ErrorRate(window),
		"windowP95LatencyMs": metrics.P95Latency(window),
	})
}

func (s *Server) handleGetDecisions(w http.ResponseWriter, r *http.Request) {
	decisions, err := s.repo.ListDecisions(r.Context(), s.rolloutCfg.RolloutID, 50)
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
	profiles, err := s.modelConfigRepo.List(r.Context())
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

	profile, err := s.modelConfigRepo.Get(r.Context(), id)
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

	profile, err := s.modelConfigRepo.Update(r.Context(), id, *body.FailureRate, *body.MinLatencyMs, *body.MaxLatencyMs)
	if err != nil {
		s.writeModelConfigError(w, err)
		return
	}

	if err := s.modelConfigPub.Publish(r.Context(), profile); err != nil {
		log.Printf("api: failed to publish model config to redis: %v", err)
		http.Error(w, "saved but failed to propagate to redis", http.StatusInternalServerError)
		return
	}

	s.hub.Broadcast("model-config-updated", profile)
	writeJSON(w, http.StatusOK, profile)
}

func (s *Server) writeModelConfigError(w http.ResponseWriter, err error) {
	var notFound *modelconfig.NotFoundError
	if errors.As(err, &notFound) {
		http.Error(w, notFound.Error(), http.StatusNotFound)
		return
	}
	log.Printf("api: model config error: %v", err)
	http.Error(w, "failed to load model configuration", http.StatusInternalServerError)
}

func (s *Server) handleRollback(w http.ResponseWriter, r *http.Request) {
	s.w.Commands <- writer.Command{
		Type:   writer.CmdRollback,
		Reason: "manual rollback via API",
		Source: "manual",
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "rollback initiated"})
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
