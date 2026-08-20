package guard

import (
	"testing"
	"time"

	"github.com/JRedCodes/rollout-controller/internal/config"
	"github.com/JRedCodes/rollout-controller/internal/metrics"
	"github.com/JRedCodes/rollout-controller/internal/writer"
)

// newStoreWithEvents records n events, the first `failures` of them marked
// unsuccessful and the rest successful, in order -- so a caller can control
// exactly which requests (fresh-window-relative) failed.
func newStoreWithEvents(n, failures int) *metrics.Store {
	store := metrics.NewStore()
	now := time.Now()
	for i := 0; i < n; i++ {
		store.Record(metrics.EventRecord{
			Timestamp: now.Add(time.Duration(i) * time.Millisecond),
			Success:   i >= failures,
			LatencyMs: 100,
		})
	}
	return store
}

// recvCommand does a non-blocking receive -- evaluate() runs synchronously,
// so by the time it returns, any send it made is already sitting in the
// buffered channel.
func recvCommand(t *testing.T, ch <-chan writer.Command) (writer.Command, bool) {
	t.Helper()
	select {
	case cmd := <-ch:
		return cmd, true
	default:
		return writer.Command{}, false
	}
}

func TestGuardEvaluate(t *testing.T) {
	basePolicy := config.RolloutPolicy{
		MinRequestsBeforeGuard:  10,
		FreshWindowSize:         10,
		FreshWindowMaxErrorRate: 0.30,
		AbsoluteMaxErrorRate:    0.05,
	}

	cases := []struct {
		name        string
		totalEvents int
		failures    int // first N events fail, rest succeed
		wantCmd     writer.CommandType
		wantNoCmd   bool
	}{
		{
			name:        "below minimum requests before guard activates, no command",
			totalEvents: 5,
			failures:    5, // 100% failure, but too few requests to evaluate at all
			wantNoCmd:   true,
		},
		{
			name:        "both windows healthy, no command",
			totalEvents: 20,
			failures:    0,
			wantNoCmd:   true,
		},
		{
			name:        "fresh window error rate exceeds threshold, rollback",
			totalEvents: 10,
			failures:    5, // 50% > 30% fresh threshold
			wantCmd:     writer.CmdRollback,
		},
		{
			name: "fresh window healthy but absolute window exceeds threshold, hold",
			// First 10 events (outside the last-10 fresh window) are all
			// failures; last 10 (the fresh window) are all successes.
			// Fresh: 0/10 = 0%. Absolute: 10/20 = 50% > 5% threshold.
			totalEvents: 20,
			failures:    10,
			wantCmd:     writer.CmdHold,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := newStoreWithEvents(c.totalEvents, c.failures)
			commands := make(chan writer.Command, 2)
			g := New(basePolicy, store, commands)

			g.evaluate()

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

			// Only one command should ever fire per evaluate() call -- the
			// fresh-window check returns early before the absolute check.
			if _, gotSecond := recvCommand(t, commands); gotSecond {
				t.Fatal("expected exactly one command, got a second")
			}
		})
	}
}

func TestGuardEvaluateFreshBreachTakesPriorityOverAbsolute(t *testing.T) {
	// Construct a case where both the fresh and absolute windows would
	// independently breach their thresholds -- only the rollback (fresh)
	// should fire, not a hold too.
	policy := config.RolloutPolicy{
		MinRequestsBeforeGuard:  10,
		FreshWindowSize:         10,
		FreshWindowMaxErrorRate: 0.30,
		AbsoluteMaxErrorRate:    0.05,
	}
	store := newStoreWithEvents(20, 20) // 100% failure everywhere
	commands := make(chan writer.Command, 2)
	g := New(policy, store, commands)

	g.evaluate()

	cmd, got := recvCommand(t, commands)
	if !got {
		t.Fatal("expected a command, got none")
	}
	if cmd.Type != writer.CmdRollback {
		t.Fatalf("expected CmdRollback to take priority, got %s", cmd.Type)
	}
	if _, gotSecond := recvCommand(t, commands); gotSecond {
		t.Fatal("expected exactly one command when both windows breach")
	}
}
