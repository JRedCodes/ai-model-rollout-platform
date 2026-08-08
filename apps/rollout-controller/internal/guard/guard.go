package guard

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/JRedCodes/rollout-controller/internal/config"
	"github.com/JRedCodes/rollout-controller/internal/metrics"
	"github.com/JRedCodes/rollout-controller/internal/writer"
)

type Guard struct {
	policy   config.RolloutPolicy
	store    *metrics.Store
	commands chan<- writer.Command
}

func New(policy config.RolloutPolicy, store *metrics.Store, commands chan<- writer.Command) *Guard {
	return &Guard{
		policy:   policy,
		store:    store,
		commands: commands,
	}
}

func (g *Guard) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	log.Printf("guard: started (fresh window: %d reqs / %.0f%%, absolute: %.0f%%)",
		g.policy.FreshWindowSize,
		g.policy.FreshWindowMaxErrorRate*100,
		g.policy.AbsoluteMaxErrorRate*100,
	)

	for {
		select {
		case <-ticker.C:
			g.evaluate()
		case <-ctx.Done():
			return
		}
	}
}

func (g *Guard) evaluate() {
	total := g.store.TotalCount()
	if total < g.policy.MinRequestsBeforeGuard {
		return
	}

	// Fresh window: last N requests — catches sudden error spikes.
	fresh := g.store.LastN(g.policy.FreshWindowSize)
	freshRate := metrics.ErrorRate(fresh)
	if freshRate > g.policy.FreshWindowMaxErrorRate {
		log.Printf("guard: fresh window error rate %.1f%% > %.1f%% threshold — rollback",
			freshRate*100, g.policy.FreshWindowMaxErrorRate*100)
		g.commands <- writer.Command{
			Type: writer.CmdRollback,
			Reason: fmt.Sprintf("fresh window error rate %.1f%% exceeded %.1f%% threshold",
				freshRate*100, g.policy.FreshWindowMaxErrorRate*100),
		}
		return
	}

	// Absolute window: all retained events — catches slow cumulative degradation.
	all := g.store.LastN(total)
	absoluteRate := metrics.ErrorRate(all)
	if absoluteRate > g.policy.AbsoluteMaxErrorRate {
		log.Printf("guard: absolute error rate %.1f%% > %.1f%% threshold — hold",
			absoluteRate*100, g.policy.AbsoluteMaxErrorRate*100)
		g.commands <- writer.Command{
			Type: writer.CmdHold,
			Reason: fmt.Sprintf("absolute error rate %.1f%% exceeded %.1f%% threshold",
				absoluteRate*100, g.policy.AbsoluteMaxErrorRate*100),
		}
	}
}
