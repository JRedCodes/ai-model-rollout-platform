package controller

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/JRedCodes/rollout-controller/internal/config"
	"github.com/JRedCodes/rollout-controller/internal/metrics"
	"github.com/JRedCodes/rollout-controller/internal/writer"
)

type Controller struct {
	policy config.RolloutPolicy
	store  *metrics.Store
	w      *writer.Writer
}

func New(policy config.RolloutPolicy, store *metrics.Store, w *writer.Writer) *Controller {
	return &Controller{
		policy: policy,
		store:  store,
		w:      w,
	}
}

func (c *Controller) Run(ctx context.Context) {
	interval := time.Duration(c.policy.ControllerIntervalSecs) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("controller: started (interval: %ds, advance threshold: %.0f%% error / %dms P95)",
		c.policy.ControllerIntervalSecs,
		c.policy.AdvanceMaxErrorRate*100,
		c.policy.AdvanceMaxP95LatencyMs,
	)

	for {
		select {
		case <-ticker.C:
			c.evaluate()
		case <-ctx.Done():
			return
		}
	}
}

func (c *Controller) evaluate() {
	if c.w.IsRolledBack() {
		log.Printf("controller: rollout is rolled back — skipping evaluation")
		return
	}

	window := c.store.Since(time.Now().Add(-time.Duration(c.policy.ControllerIntervalSecs) * time.Second))

	if len(window) < c.policy.AdvanceMinRequests {
		log.Printf("controller: %d requests in window, need %d — skipping", len(window), c.policy.AdvanceMinRequests)
		return
	}

	errorRate := metrics.ErrorRate(window)
	p95 := metrics.P95Latency(window)
	healthy := errorRate <= c.policy.AdvanceMaxErrorRate && p95 <= c.policy.AdvanceMaxP95LatencyMs

	log.Printf("controller: window stats — %d reqs, %.1f%% errors, P95 %dms",
		len(window), errorRate*100, p95)

	if held := c.w.IsHeld(); held {
		if !healthy {
			log.Printf("controller: still held — error rate %.1f%%, P95 %dms remain outside thresholds",
				errorRate*100, p95)
			return
		}
		log.Printf("controller: window recovered — error rate %.1f%%, P95 %dms — resuming (not advancing yet)",
			errorRate*100, p95)
		c.w.Commands <- writer.Command{
			Type: writer.CmdResume,
			Reason: fmt.Sprintf("error rate %.1f%%, P95 %dms recovered within thresholds",
				errorRate*100, p95),
		}
		return
	}

	if errorRate > c.policy.AdvanceMaxErrorRate {
		log.Printf("controller: error rate %.1f%% > %.1f%% advance threshold — holding",
			errorRate*100, c.policy.AdvanceMaxErrorRate*100)
		c.w.Commands <- writer.Command{
			Type: writer.CmdHold,
			Reason: fmt.Sprintf("error rate %.1f%% exceeded advance threshold %.1f%%",
				errorRate*100, c.policy.AdvanceMaxErrorRate*100),
		}
		return
	}

	if p95 > c.policy.AdvanceMaxP95LatencyMs {
		log.Printf("controller: P95 latency %dms > %dms threshold — holding", p95, c.policy.AdvanceMaxP95LatencyMs)
		c.w.Commands <- writer.Command{
			Type: writer.CmdHold,
			Reason: fmt.Sprintf("P95 latency %dms exceeded threshold %dms",
				p95, c.policy.AdvanceMaxP95LatencyMs),
		}
		return
	}

	log.Printf("controller: healthy — error rate %.1f%%, P95 %dms — advancing", errorRate*100, p95)
	c.w.Commands <- writer.Command{
		Type:   writer.CmdAdvance,
		Reason: fmt.Sprintf("error rate %.1f%%, P95 %dms — within thresholds", errorRate*100, p95),
	}
}
