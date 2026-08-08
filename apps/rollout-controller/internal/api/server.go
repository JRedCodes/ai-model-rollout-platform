package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/JRedCodes/rollout-controller/internal/config"
	"github.com/JRedCodes/rollout-controller/internal/metrics"
	"github.com/JRedCodes/rollout-controller/internal/writer"
)

type Server struct {
	rolloutCfg config.RolloutConfig
	store      *metrics.Store
	w          *writer.Writer
	httpServer *http.Server
}

func New(port int, rolloutCfg config.RolloutConfig, store *metrics.Store, w *writer.Writer) *Server {
	s := &Server{
		rolloutCfg: rolloutCfg,
		store:      store,
		w:          w,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /rollout", s.handleGetRollout)
	mux.HandleFunc("GET /rollout/metrics", s.handleGetMetrics)
	mux.HandleFunc("POST /rollout/rollback", s.handleRollback)

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
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
		"candidatePercentage":     s.rolloutCfg.CandidatePercentage,
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

func (s *Server) handleRollback(w http.ResponseWriter, r *http.Request) {
	s.w.Commands <- writer.Command{
		Type:   writer.CmdRollback,
		Reason: "manual rollback via API",
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
