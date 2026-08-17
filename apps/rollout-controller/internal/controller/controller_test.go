package controller

import (
	"testing"
	"time"

	"github.com/JRedCodes/rollout-controller/internal/config"
	"github.com/JRedCodes/rollout-controller/internal/metrics"
	"github.com/JRedCodes/rollout-controller/internal/writer"
)

// fakeState is a StateReader test double -- avoids needing a real
// Redis/Postgres-backed *writer.Writer just to flip held/rolled-back.
type fakeState struct {
	held       bool
	rolledBack bool
}

func (f fakeState) IsHeld() bool       { return f.held }
func (f fakeState) IsRolledBack() bool { return f.rolledBack }

// newStoreWithEvents records n events, all within the controller's
// evaluation window, the first `failures` of them failed and the rest
// with the given latency.
func newStoreWithEvents(n, failures, latencyMs int) *metrics.Store {
	store := metrics.NewStore()
	now := time.Now()
	for i := 0; i < n; i++ {
		store.Record(metrics.EventRecord{
			Timestamp: now,
			Success:   i >= failures,
			LatencyMs: latencyMs,
		})
	}
	return store
}

func recvCommand(t *testing.T, ch <-chan writer.Command) (writer.Command, bool) {
	t.Helper()
	select {
	case cmd := <-ch:
		return cmd, true
	default:
		return writer.Command{}, false
	}
}

func TestControllerEvaluate(t *testing.T) {
	basePolicy := config.RolloutPolicy{
		ControllerIntervalSecs: 120,
		AdvanceMinRequests:     10,
		AdvanceMaxErrorRate:    0.02,
		AdvanceMaxP95LatencyMs: 250,
	}

	cases := []struct {
		name        string
		state       fakeState
		totalEvents int
		failures    int
		latencyMs   int
		wantCmd     writer.CommandType
		wantNoCmd   bool
	}{
		{
			name:        "rolled back, evaluation skipped entirely",
			state:       fakeState{rolledBack: true},
			totalEvents: 100,
			failures:    100, // would otherwise clearly hold/advance either way
			latencyMs:   50,
			wantNoCmd:   true,
		},
		{
			name:        "below minimum requests in window, no command",
			totalEvents: 5,
			latencyMs:   50,
			wantNoCmd:   true,
		},
		{
			name:        "not held, healthy window, advance",
			totalEvents: 20,
			failures:    0,
			latencyMs:   50,
			wantCmd:     writer.CmdAdvance,
		},
		{
			name:        "not held, error rate exceeds threshold, hold",
			totalEvents: 20,
			failures:    2, // 10% > 2% threshold
			latencyMs:   50,
			wantCmd:     writer.CmdHold,
		},
		{
			name:        "not held, P95 latency exceeds threshold, hold",
			totalEvents: 20,
			failures:    0,
			latencyMs:   300, // > 250ms threshold
			wantCmd:     writer.CmdHold,
		},
		{
			name:        "held, window still unhealthy, no command (stays held)",
			state:       fakeState{held: true},
			totalEvents: 20,
			failures:    2,
			latencyMs:   50,
			wantNoCmd:   true,
		},
		{
			name:        "held, window recovered, resume (not advance)",
			state:       fakeState{held: true},
			totalEvents: 20,
			failures:    0,
			latencyMs:   50,
			wantCmd:     writer.CmdResume,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := newStoreWithEvents(c.totalEvents, c.failures, c.latencyMs)
			commands := make(chan writer.Command, 2)
			ctrl := &Controller{
				policy:   basePolicy,
				store:    store,
				state:    c.state,
				commands: commands,
			}

			ctrl.evaluate()

			cmd, got := recvCommand(t, commands)
			if c.wantNoCmd {
				if got {
					t.Fatalf("expected no command, got %+v", cmd)
				}
				return
			}

			if !got {
				t.Fatalf("expected a %s command, got none", c.wantCmd)
			}
			if cmd.Type != c.wantCmd {
				t.Fatalf("expected command type %s, got %s (reason: %q)", c.wantCmd, cmd.Type, cmd.Reason)
			}
			if cmd.Reason == "" {
				t.Fatal("expected a non-empty reason")
			}
			if _, gotSecond := recvCommand(t, commands); gotSecond {
				t.Fatal("expected exactly one command")
			}
		})
	}
}
