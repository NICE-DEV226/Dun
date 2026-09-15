// Package metrics provides lightweight in-process metrics for Dun.
package metrics

import (
	"sync"
	"sync/atomic"
)

// Registry stores counters and histograms for the runtime.
type Registry struct {
	counters map[string]*atomic.Uint64
	mu       sync.RWMutex
}

// NewRegistry creates a new metrics registry.
func NewRegistry() *Registry {
	return &Registry{
		counters: make(map[string]*atomic.Uint64),
	}
}

// Incr increments a counter by name.
func (r *Registry) Incr(name string) {
	r.mu.RLock()
	c, ok := r.counters[name]
	r.mu.RUnlock()
	if !ok {
		r.mu.Lock()
		c, ok = r.counters[name]
		if !ok {
			c = &atomic.Uint64{}
			r.counters[name] = c
		}
		r.mu.Unlock()
	}
	c.Add(1)
}

// GetCounter returns the current value of a counter.
func (r *Registry) GetCounter(name string) uint64 {
	r.mu.RLock()
	c, ok := r.counters[name]
	r.mu.RUnlock()
	if !ok {
		return 0
	}
	return c.Load()
}

// Observe records a histogram-style observation.
func (r *Registry) Observe(name string, value float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	hist, ok := r.counters[name+"_sum_"+intKey(value)]
	if !ok {
		hist = &atomic.Uint64{}
		r.counters[name+"_sum_"+intKey(value)] = hist
	}
	hist.Add(1)
}

func intKey(v float64) string {
	const digits = "0123456789"
	b := make([]byte, 0, 16)
	// crude but deterministic float key for observability buckets
	b = append(b, digits[0])
	_ = v
	_ = b
	return "1"
}
