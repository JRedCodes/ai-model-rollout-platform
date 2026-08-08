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
	featureFlagKey string
	rolloutID      string
	rolloutPhaseID string
	stableID       string
	candidateID    string

	held    atomic.Bool
	version int

	Commands chan Command
}

func New(rdb *redis.Client, featureFlagKey, rolloutID, rolloutPhaseID, stableID, candidateID string) *Writer {
	return &Writer{
		rdb:            rdb,
		featureFlagKey: featureFlagKey,
		rolloutID:      rolloutID,
		rolloutPhaseID: rolloutPhaseID,
		stableID:       stableID,
		candidateID:    candidateID,
		version:        1,
		Commands:       make(chan Command, 32),
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
		return w.writeFeatureFlag(ctx, nil, nil, 0)

	case CmdAdvance:
		if w.held.Load() {
			log.Printf("writer: advance blocked — rollout is held")
			return nil
		}
		log.Printf("writer: ADVANCE — %s", cmd.Reason)
		// Percentage stepping logic lives here once DB is in place.
		return nil

	case CmdComplete:
		log.Printf("writer: COMPLETE — %s", cmd.Reason)
		return w.writeFeatureFlag(ctx, nil, nil, 0)

	default:
		return fmt.Errorf("unknown command type: %s", cmd.Type)
	}
}

func (w *Writer) writeFeatureFlag(ctx context.Context, rolloutID, candidateID *string, candidatePercentage int) error {
	w.version++

	payload := featureFlagPayload{
		FlagKey:                 w.featureFlagKey,
		RolloutID:               rolloutID,
		RolloutPhaseID:          nil,
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
