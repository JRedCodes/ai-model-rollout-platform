package writer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync/atomic"

	"github.com/redis/go-redis/v9"
)

type CommandType string

const (
	CmdHold     CommandType = "HOLD"
	CmdRollback CommandType = "ROLLBACK"
	CmdAdvance  CommandType = "ADVANCE"
	CmdComplete CommandType = "COMPLETE"
)

type Command struct {
	Type   CommandType
	Reason string
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
	rdb               *redis.Client
	featureFlagKey    string
	rolloutID         string
	rolloutPhaseID    string
	stableID          string
	candidateID       string
	currentPercentage int

	held    atomic.Bool
	version int

	Commands chan Command
}

func New(
	rdb *redis.Client,
	featureFlagKey, rolloutID, rolloutPhaseID, stableID, candidateID string,
	initialPercentage int,
) *Writer {
	return &Writer{
		rdb:               rdb,
		featureFlagKey:    featureFlagKey,
		rolloutID:         rolloutID,
		rolloutPhaseID:    rolloutPhaseID,
		stableID:          stableID,
		candidateID:       candidateID,
		currentPercentage: initialPercentage,
		version:           1,
		Commands:          make(chan Command, 32),
	}
}

// IsHeld is safe to call from any goroutine.
func (w *Writer) IsHeld() bool {
	return w.held.Load()
}

func (w *Writer) Run(ctx context.Context) {
	for {
		select {
		case cmd := <-w.Commands:
			if err := w.handle(ctx, cmd); err != nil {
				log.Printf("writer: failed to handle %s command: %v", cmd.Type, err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (w *Writer) handle(ctx context.Context, cmd Command) error {
	switch cmd.Type {
	case CmdHold:
		w.held.Store(true)
		log.Printf("writer: HOLD — %s", cmd.Reason)
		return nil

	case CmdRollback:
		w.held.Store(true)
		log.Printf("writer: ROLLBACK — %s", cmd.Reason)
		return w.writeInactiveFlag(ctx)

	case CmdAdvance:
		if w.held.Load() {
			log.Printf("writer: advance blocked — rollout is held")
			return nil
		}
		return w.advance(ctx, cmd.Reason)

	case CmdComplete:
		log.Printf("writer: COMPLETE — %s", cmd.Reason)
		return w.writeInactiveFlag(ctx)

	default:
		return fmt.Errorf("unknown command type: %s", cmd.Type)
	}
}

func (w *Writer) advance(ctx context.Context, reason string) error {
	next, done := nextPercentage(w.currentPercentage)

	if done {
		log.Printf("writer: COMPLETE — rollout fully ramped at %d%%", w.currentPercentage)
		return w.writeInactiveFlag(ctx)
	}

	prev := w.currentPercentage
	w.currentPercentage = next

	log.Printf("writer: ADVANCE %d%% → %d%% — %s", prev, next, reason)
	return w.writeActiveFlag(ctx)
}

// writeActiveFlag writes the feature flag with the current candidate still in place.
func (w *Writer) writeActiveFlag(ctx context.Context) error {
	rolloutID := w.rolloutID
	rolloutPhaseID := w.rolloutPhaseID
	candidateID := w.candidateID

	return w.writeFeatureFlag(ctx, &rolloutID, &rolloutPhaseID, &candidateID, w.currentPercentage)
}

// writeInactiveFlag writes the feature flag with no candidate — used for rollback and completion.
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
