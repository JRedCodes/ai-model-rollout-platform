package writer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/JRedCodes/rollout-controller/internal/db"
)

type CommandType string

const (
	CmdHold     CommandType = "HOLD"
	CmdRollback CommandType = "ROLLBACK"
	CmdAdvance  CommandType = "ADVANCE"
	CmdComplete CommandType = "COMPLETE"
	CmdResume   CommandType = "RESUME"
)

type Command struct {
	Type   CommandType
	Reason string
	Source string // "guard" | "controller" | "manual"
}

// percentageSteps is the advancement ladder for candidate traffic.
var percentageSteps = []int{10, 25, 50, 75, 100}

type featureFlagPayload struct {
	FlagKey                 string  `json:"flagKey"`
	RolloutID               *string `json:"rolloutId"`
	RolloutPhaseID          *string `json:"rolloutPhaseId"`
	StableModelVersionID    string  `json:"stableModelVersionId"`
	CandidateModelVersionID *string `json:"candidateModelVersionId"`
	CandidatePercentage     int     `json:"candidatePercentage"`
	ConfigurationVersion    int     `json:"configurationVersion"`
}

type Writer struct {
	rdb            *redis.Client
	repo           *db.RolloutRepository
	broadcast      func(string, any)
	featureFlagKey string
	rolloutID      string
	rolloutPhaseID string
	stableID       string
	candidateID    string

	pct        atomic.Int32 // current candidate percentage; readable from any goroutine
	held       atomic.Bool
	rolledBack atomic.Bool
	version    int

	Commands chan Command
}

func New(
	rdb *redis.Client,
	repo *db.RolloutRepository,
	broadcast func(string, any),
	featureFlagKey, rolloutID, rolloutPhaseID, stableID, candidateID string,
	initialPercentage, initialVersion int,
) *Writer {
	w := &Writer{
		rdb:            rdb,
		repo:           repo,
		broadcast:      broadcast,
		featureFlagKey: featureFlagKey,
		rolloutID:      rolloutID,
		rolloutPhaseID: rolloutPhaseID,
		stableID:       stableID,
		candidateID:    candidateID,
		version:        initialVersion,
		Commands:       make(chan Command, 32),
	}
	w.pct.Store(int32(initialPercentage))
	return w
}

func (w *Writer) emit(eventType string, data any) {
	if w.broadcast != nil {
		w.broadcast(eventType, data)
	}
}

// IsHeld is safe to call from any goroutine. True for both HOLD and
// ROLLBACK — use IsRolledBack to distinguish the two.
func (w *Writer) IsHeld() bool {
	return w.held.Load()
}

// IsRolledBack is safe to call from any goroutine.
func (w *Writer) IsRolledBack() bool {
	return w.rolledBack.Load()
}

// CurrentPercentage returns the live candidate traffic percentage.
// Safe to call from any goroutine.
func (w *Writer) CurrentPercentage() int {
	return int(w.pct.Load())
}

// SeedRedis writes the current rollout state to Redis without changing anything.
// Call this on startup so the Edge Evaluator has a valid feature flag immediately.
func (w *Writer) SeedRedis(ctx context.Context) error {
	return w.writeActiveFlag(ctx)
}

func (w *Writer) Run(ctx context.Context) {
	heartbeat := time.NewTicker(60 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case cmd := <-w.Commands:
			if err := w.handle(ctx, cmd); err != nil {
				log.Printf("writer: failed to handle %s command: %v", cmd.Type, err)
			}
		case <-heartbeat.C:
			if !w.held.Load() {
				if err := w.writeActiveFlag(ctx); err != nil {
					log.Printf("writer: heartbeat re-seed failed: %v", err)
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (w *Writer) handle(ctx context.Context, cmd Command) error {
	source := cmd.Source
	if source == "" {
		source = "controller"
	}

	switch cmd.Type {
	case CmdHold:
		w.held.Store(true)
		log.Printf("writer: HOLD — %s", cmd.Reason)
		w.persistDecision(ctx, string(CmdHold), cmd.Reason, source)
		w.emit("hold", map[string]any{
			"reason": cmd.Reason, "source": source,
			"candidatePercentage": w.CurrentPercentage(),
		})
		return w.repo.UpdateStatus(ctx, w.rolloutID, "HELD")

	case CmdRollback:
		w.held.Store(true)
		w.rolledBack.Store(true)
		w.pct.Store(0)
		log.Printf("writer: ROLLBACK — %s", cmd.Reason)
		w.persistDecision(ctx, string(CmdRollback), cmd.Reason, source)
		w.emit("rollback", map[string]any{"reason": cmd.Reason, "source": source})
		if err := w.repo.UpdateStatus(ctx, w.rolloutID, "ROLLED_BACK"); err != nil {
			return err
		}
		return w.writeInactiveFlag(ctx)

	case CmdResume:
		if !w.held.Load() {
			log.Printf("writer: resume ignored — rollout is not held")
			return nil
		}
		w.held.Store(false)
		log.Printf("writer: RESUME — %s", cmd.Reason)
		w.persistDecision(ctx, string(CmdResume), cmd.Reason, source)
		w.emit("resume", map[string]any{
			"reason": cmd.Reason, "source": source,
			"candidatePercentage": w.CurrentPercentage(),
		})
		return w.repo.UpdateStatus(ctx, w.rolloutID, "RUNNING")

	case CmdAdvance:
		if w.held.Load() {
			log.Printf("writer: advance blocked — rollout is held")
			return nil
		}
		w.persistDecision(ctx, string(CmdAdvance), cmd.Reason, source)
		return w.advance(ctx, cmd.Reason, source)

	case CmdComplete:
		promoted := w.candidateID
		w.stableID = promoted
		w.pct.Store(0)
		log.Printf("writer: COMPLETE — %s, promoting %s to stable", cmd.Reason, promoted)
		w.persistDecision(ctx, string(CmdComplete), cmd.Reason, source)
		w.emit("complete", map[string]any{"reason": cmd.Reason, "source": source, "promotedModelVersionId": promoted})
		if err := w.repo.UpdateStatus(ctx, w.rolloutID, "COMPLETED"); err != nil {
			return err
		}
		return w.writeInactiveFlag(ctx)

	default:
		return fmt.Errorf("unknown command type: %s", cmd.Type)
	}
}

func (w *Writer) advance(ctx context.Context, reason, source string) error {
	current := int(w.pct.Load())
	next, done := nextPercentage(current)

	if done {
		promoted := w.candidateID
		w.stableID = promoted
		w.pct.Store(0)
		log.Printf("writer: COMPLETE — rollout fully ramped at %d%%, promoting %s to stable", current, promoted)
		w.emit("complete", map[string]any{"reason": "fully ramped at 100%", "source": source, "promotedModelVersionId": promoted})
		if err := w.repo.UpdateStatus(ctx, w.rolloutID, "COMPLETED"); err != nil {
			return err
		}
		return w.writeInactiveFlag(ctx)
	}

	w.pct.Store(int32(next))
	log.Printf("writer: ADVANCE %d%% → %d%% — %s", current, next, reason)
	w.emit("advance", map[string]any{"from": current, "to": next, "reason": reason, "source": source})

	if err := w.repo.UpdatePercentage(ctx, w.rolloutID, next); err != nil {
		return err
	}

	return w.writeActiveFlag(ctx)
}

func (w *Writer) persistDecision(ctx context.Context, action, reason, source string) {
	if err := w.repo.InsertDecision(ctx, uuid.NewString(), w.rolloutID, action, reason, source); err != nil {
		log.Printf("writer: failed to persist decision: %v", err)
	}
}

// writeActiveFlag writes the feature flag with the current candidate still in place.
func (w *Writer) writeActiveFlag(ctx context.Context) error {
	rolloutID := w.rolloutID
	rolloutPhaseID := w.rolloutPhaseID
	candidateID := w.candidateID
	return w.writeFeatureFlag(ctx, &rolloutID, &rolloutPhaseID, &candidateID, int(w.pct.Load()))
}

// writeInactiveFlag clears the candidate — used for rollback and completion.
func (w *Writer) writeInactiveFlag(ctx context.Context) error {
	return w.writeFeatureFlag(ctx, nil, nil, nil, 0)
}

func (w *Writer) writeFeatureFlag(ctx context.Context, rolloutID, rolloutPhaseID, candidateID *string, candidatePercentage int) error {
	w.version++

	payload := featureFlagPayload{
		FlagKey:                 w.featureFlagKey,
		RolloutID:               rolloutID,
		RolloutPhaseID:          rolloutPhaseID,
		StableModelVersionID:    w.stableID,
		CandidateModelVersionID: candidateID,
		CandidatePercentage:     candidatePercentage,
		ConfigurationVersion:    w.version,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal feature flag: %w", err)
	}

	return w.rdb.Set(ctx, w.featureFlagKey, data, 0).Err()
}

// nextPercentage returns the next step in the advancement ladder.
// Returns done=true when the rollout has reached 100% and should complete.
func nextPercentage(current int) (next int, done bool) {
	for i, step := range percentageSteps {
		if current < step {
			return step, false
		}
		if current == step {
			if i == len(percentageSteps)-1 {
				return 0, true
			}
			return percentageSteps[i+1], false
		}
	}
	return 0, true
}
