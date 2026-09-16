// Package metrics provides lightweight in-process metrics for Dun.
package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Registry stores counters and histograms for the runtime.
type Registry struct {
	counters   map[string]*atomic.Uint64
	histograms map[string]*histogram
	mu         sync.RWMutex
}

type histogram struct {
	mu    sync.RWMutex
	count uint64
	sum   float64
}

// NewRegistry creates a new metrics registry.
func NewRegistry() *Registry {
	return &Registry{
		counters:   make(map[string]*atomic.Uint64),
		histograms: make(map[string]*histogram),
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
	r.mu.RLock()
	hist, ok := r.histograms[name]
	r.mu.RUnlock()
	if !ok {
		r.mu.Lock()
		hist, ok = r.histograms[name]
		if !ok {
			hist = &histogram{}
			r.histograms[name] = hist
		}
		r.mu.Unlock()
	}
	hist.mu.Lock()
	hist.count++
	hist.sum += value
	hist.mu.Unlock()
}

// GetHistogram returns the number and sum of observations for a metric.
func (r *Registry) GetHistogram(name string) (count uint64, sum float64) {
	r.mu.RLock()
	hist := r.histograms[name]
	r.mu.RUnlock()
	if hist == nil {
		return 0, 0
	}
	hist.mu.RLock()
	defer hist.mu.RUnlock()
	return hist.count, hist.sum
}

// Prometheus returns the registry in the Prometheus text exposition format.
// Metric names are normalized and the output is deterministic for scraping
// and tests.
func (r *Registry) Prometheus() string {
	r.mu.RLock()
	counterNames := make([]string, 0, len(r.counters))
	for name := range r.counters {
		counterNames = append(counterNames, name)
	}
	histogramNames := make([]string, 0, len(r.histograms))
	for name := range r.histograms {
		histogramNames = append(histogramNames, name)
	}
	r.mu.RUnlock()
	sort.Strings(counterNames)
	sort.Strings(histogramNames)

	var out strings.Builder
	for _, name := range counterNames {
		fmt.Fprintf(&out, "%s %d\n", metricName(name), r.GetCounter(name))
	}
	for _, name := range histogramNames {
		count, sum := r.GetHistogram(name)
		base := metricName(name)
		fmt.Fprintf(&out, "%s_count %d\n%s_sum %g\n", base, count, base, sum)
	}
	return out.String()
}

func metricName(name string) string {
	name = strings.ToLower(name)
	var out strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			out.WriteRune(r)
		} else {
			out.WriteByte('_')
		}
	}
	return out.String()
}
