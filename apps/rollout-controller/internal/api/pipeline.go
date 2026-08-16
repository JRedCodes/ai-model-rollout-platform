package api

import (
	"sync"

	"github.com/JRedCodes/rollout-controller/internal/config"
	"github.com/JRedCodes/rollout-controller/internal/metrics"
	"github.com/JRedCodes/rollout-controller/internal/writer"
)

// ActivePipeline is a snapshot of one tenant's currently-running rollout
// config and the live objects (Writer, metrics Store) the API needs to
// read from. The supervisor loop in main.go rebuilds and swaps a tenant's
// entry every time a different rollout becomes active for it.
type ActivePipeline struct {
	Cfg    config.RolloutConfig
	Writer *writer.Writer
	Store  *metrics.Store
}

// PipelineRegistry is a thread-safe map of tenantID -> that tenant's
// current ActivePipeline (absent if the tenant has no active rollout). The
// supervisor loop is the sole writer; HTTP handlers are readers.
type PipelineRegistry struct {
	mu       sync.RWMutex
	byTenant map[string]*ActivePipeline
}

func NewPipelineRegistry() *PipelineRegistry {
	return &PipelineRegistry{byTenant: make(map[string]*ActivePipeline)}
}

// Get returns tenantID's current pipeline, or nil if it has no active rollout.
func (r *PipelineRegistry) Get(tenantID string) *ActivePipeline {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byTenant[tenantID]
}

// Set stores tenantID's current pipeline. Pass nil to mark it idle.
func (r *PipelineRegistry) Set(tenantID string, p *ActivePipeline) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p == nil {
		delete(r.byTenant, tenantID)
		return
	}
	r.byTenant[tenantID] = p
}

// SSEHubRegistry is a thread-safe map of tenantID -> that tenant's SSE hub.
// A shared global hub would leak one tenant's decisions/events to another
// tenant's dashboard, so each tenant gets its own, created lazily on first
// use and kept for the tenant's lifetime (it outlives individual pipeline
// rebuilds, same as the single hub did before multi-tenancy).
type SSEHubRegistry struct {
	mu   sync.Mutex
	hubs map[string]*SSEHub
}

func NewSSEHubRegistry() *SSEHubRegistry {
	return &SSEHubRegistry{hubs: make(map[string]*SSEHub)}
}

// Get returns tenantID's hub, creating one on first use.
func (r *SSEHubRegistry) Get(tenantID string) *SSEHub {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.hubs[tenantID]
	if !ok {
		h = NewSSEHub()
		r.hubs[tenantID] = h
	}
	return h
}
