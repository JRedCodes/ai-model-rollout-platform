package api

import (
	"sync/atomic"

	"github.com/JRedCodes/rollout-controller/internal/config"
	"github.com/JRedCodes/rollout-controller/internal/metrics"
	"github.com/JRedCodes/rollout-controller/internal/writer"
)

// ActivePipeline is a snapshot of the currently-running rollout's config and
// the live objects (Writer, metrics Store) the API needs to read from. The
// supervisor loop in main.go rebuilds and swaps this every time a different
// rollout becomes active.
type ActivePipeline struct {
	Cfg    config.RolloutConfig
	Writer *writer.Writer
	Store  *metrics.Store
}

// PipelineHolder is a thread-safe pointer to the current ActivePipeline (or
// nil when no rollout is active). The supervisor loop is the sole writer;
// HTTP handlers are readers.
type PipelineHolder struct {
	v atomic.Pointer[ActivePipeline]
}

func NewPipelineHolder() *PipelineHolder {
	return &PipelineHolder{}
}

// Load returns the current pipeline, or nil if no rollout is active.
func (h *PipelineHolder) Load() *ActivePipeline {
	return h.v.Load()
}

// Store sets the current pipeline. Pass nil to mark the system idle.
func (h *PipelineHolder) Store(p *ActivePipeline) {
	h.v.Store(p)
}
