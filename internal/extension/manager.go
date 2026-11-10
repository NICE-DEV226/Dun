package extension

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// Manager owns the lifecycle of all loaded extensions.
type Manager struct {
	host     Host
	logger   *slog.Logger
	policies map[string]Set
	limits   map[string]ResourceLimits

	mu        sync.RWMutex
	byName    map[string]*entry
	telemetry map[string]*failureCounters
	supported map[string]bool

	invocations map[string]*atomic.Uint64
	durations   map[string]*atomic.Uint64 // total invoke time, nanoseconds
	reloads     map[string]*atomic.Uint64
}

type entry struct {
	name     string
	manifest *Manifest
	module   Module
	liveness *Liveness
	policy   Set
	limits   ResourceLimits
	active   sync.RWMutex
}

type failureCounters struct {
	mu     sync.Mutex
	byKind map[FailureKind]uint64
	total  uint64
	last   string
}

// NewManager creates an extension manager bound to a runtime host.
func NewManager(host Host, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		host:      host,
		logger:    logger,
		policies:  make(map[string]Set),
		limits:    make(map[string]ResourceLimits),
		byName:    make(map[string]*entry),
		telemetry: make(map[string]*failureCounters),
		supported: map[string]bool{
			"wasm": true,
		},
		invocations: make(map[string]*atomic.Uint64),
		durations:   make(map[string]*atomic.Uint64),
		reloads:     make(map[string]*atomic.Uint64),
	}
}

// SetPolicy grants a capability policy for a plugin name.
// This is the deployment policy that load-time validation (stage 1)
// checks declared capabilities against.
func (m *Manager) SetPolicy(name string, caps Set) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.policies[name] = caps
}

// Policy returns the policy granted for a plugin name.
func (m *Manager) Policy(name string) Set {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.policies[name]
}

// SetLimits configures the sandbox resource limits for a plugin name.
func (m *Manager) SetLimits(name string, limits ResourceLimits) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.limits[name] = limits
}

// Limits returns the resource limits configured for a plugin name.
func (m *Manager) Limits(name string) ResourceLimits {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.limits[name]
}

// Load validates and loads a plugin from manifest metadata and a raw
// Wasm artifact. It moves the plugin through DISCOVER -> VALIDATE -> LOAD
// -> COMPILE and leaves it in COMPILE (not yet instantiated).
func (m *Manager) Load(name string, manifest *Manifest, artifact []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.byName[name]; exists {
		return fmt.Errorf("extension %q already loaded", name)
	}

	liveness := NewLiveness()
	if err := liveness.Validate(); err != nil {
		return err
	}

	// Stage 1: fast-fail capability validation against deployment policy.
	policy, ok := m.policies[name]
	if !ok {
		return fmt.Errorf("%w: no policy configured for %q", ErrInvalidCapability, name)
	}
	limits := m.limits[name]
	declared := manifest.All()
	if overlap := declared.Intersection(policy); len(overlap) != len(declared) {
		for c := range declared {
			if !policy.Has(c) {
				m.logger.Warn("capability not permitted by policy",
					"extension", name,
					"capability", string(c),
				)
			}
		}
		return fmt.Errorf("%w: %q requests capabilities outside its policy", ErrInvalidCapability, name)
	}

	// ABI marker check at VALIDATE.
	if m.host != nil {
		if err := m.host.CheckABI(context.Background(), artifact); err != nil {
			return fmt.Errorf("ABI validation failed: %w", err)
		}
	}

	if err := liveness.Transition(StateLoad); err != nil {
		return err
	}
	if err := liveness.Transition(StateCompile); err != nil {
		return err
	}

	var mod Module = &noopModule{name: name}
	if m.host != nil {
		loaded, err := m.host.Load(context.Background(), artifact, LoadOptions{
			Capabilities: policy,
			Limits:       limits,
		})
		if err != nil {
			return fmt.Errorf("load module: %w", err)
		}
		mod = &hostModule{inner: loaded, name: name}
	}

	m.byName[name] = &entry{
		name:     name,
		manifest: manifest,
		module:   mod,
		liveness: liveness,
		policy:   policy,
		limits:   limits,
	}
	if _, ok := m.telemetry[name]; !ok {
		m.telemetry[name] = &failureCounters{byKind: make(map[FailureKind]uint64)}
	}
	if _, ok := m.invocations[name]; !ok {
		m.invocations[name] = &atomic.Uint64{}
		m.durations[name] = &atomic.Uint64{}
		m.reloads[name] = &atomic.Uint64{}
	}

	m.logger.Info("extension loaded", "extension", name, "version", manifest.Version)
	return nil
}

// Activate moves a loaded extension into SERVING.
// It walks the full remaining lifecycle path (INSTANTIATE -> INITIALIZE
// -> READY -> SERVING) so no intermediate transition is skipped.
func (m *Manager) Activate(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.byName[name]
	if !ok {
		return ErrNotFound
	}
	for _, s := range []LifecycleState{
		StateInstantiate, StateInitialize, StateReady, StateServing,
	} {
		if err := e.liveness.Transition(s); err != nil {
			return err
		}
	}
	return nil
}

// Reload replaces a running plugin's binary with a new artifact using
// atomic activation (RFC-0000 §13): the new binary is validated (policy
// + ABI) and compiled while the old version keeps SERVING; only after the
// new module is ready is the running entry swapped in a single operation.
// Existing in-flight requests are unaffected — with per-request
// instantiation (§15.1) they already hold their own module instance, so
// the old compiled module is drained (nothing is running on it) and
// closed right after the swap. A failed reload leaves the previous
// version active.
func (m *Manager) Reload(name string, newArtifact []byte, newManifest *Manifest) error {
	m.mu.RLock()
	e, ok := m.byName[name]
	m.mu.RUnlock()
	if !ok {
		return ErrNotFound
	}
	if e.liveness.State() != StateServing {
		return fmt.Errorf("extension %q not serving (state=%s), cannot reload", name, e.liveness.State())
	}

	// Stage 1: fast-fail capability validation against deployment policy.
	declared := newManifest.All()
	if overlap := declared.Intersection(e.policy); len(overlap) != len(declared) {
		for c := range declared {
			if !e.policy.Has(c) {
				m.logger.Warn("reload capability not permitted by policy",
					"extension", name,
					"capability", string(c),
				)
			}
		}
		return fmt.Errorf("%w: %q reload requests capabilities outside its policy", ErrInvalidCapability, name)
	}

	// ABI marker check at VALIDATE (never skips the gate).
	if m.host != nil {
		if err := m.host.CheckABI(context.Background(), newArtifact); err != nil {
			return fmt.Errorf("reload ABI validation failed: %w", err)
		}
	}

	// Compile the new version while the old one keeps serving.
	var next Module = &noopModule{name: name}
	if m.host != nil {
		loaded, err := m.host.Load(context.Background(), newArtifact, LoadOptions{
			Capabilities: e.policy,
			Limits:       e.limits,
		})
		if err != nil {
			return fmt.Errorf("reload load module: %w", err)
		}
		next = &hostModule{inner: loaded, name: name}
	}

	// Atomic activation: swap pointers in one locked operation.
	m.mu.Lock()
	cur, ok := m.byName[name]
	if !ok || cur != e || cur.liveness.State() != StateServing {
		m.mu.Unlock()
		_ = next.Close(context.Background())
		if !ok {
			return ErrNotFound
		}
		return fmt.Errorf("extension %q changed while reloading", name)
	}
	old := cur.module
	cur.module = next
	cur.manifest = newManifest
	m.mu.Unlock()

	// Drain: no pooled instances exist; in-flight requests are already
	// isolated module instances (§13.1), so close the old compiled module.
	e.active.Lock()
	defer e.active.Unlock()
	if err := old.Close(context.Background()); err != nil {
		m.logger.Warn("failed to close old extension after reload",
			"extension", name,
			"error", err,
		)
	}

	m.logger.Info("extension reloaded",
		"extension", name,
		"version", newManifest.Version,
	)
	if c, ok := m.reloads[name]; ok {
		c.Add(1)
	}
	return nil
}

// Close closes and removes an extension.
func (m *Manager) Close(ctx context.Context, name string) error {
	m.mu.Lock()
	e, ok := m.byName[name]
	if !ok {
		m.mu.Unlock()
		return ErrNotFound
	}
	if e.liveness.State() == StateServing {
		if err := e.liveness.Transition(StateDraining); err != nil {
			m.mu.Unlock()
			return err
		}
	}
	delete(m.byName, name)
	m.mu.Unlock()

	e.active.Lock()
	defer e.active.Unlock()
	if err := e.module.Close(ctx); err != nil {
		return err
	}
	if err := e.liveness.Transition(StateClosed); err != nil {
		return err
	}
	m.logger.Info("extension removed", "extension", name)
	return nil
}

// CloseAll drains and closes every loaded extension.
func (m *Manager) CloseAll(ctx context.Context) error {
	for _, name := range m.List() {
		if err := m.Close(ctx, name); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	return nil
}

// Invoke runs a named extension operation with an optional per-request bridge.
// Failures are classified into telemetry counters (RFC-0000 §14) and the
// core is kept alive: the error is returned to the caller, never panicked.
func (m *Manager) Invoke(ctx context.Context, name, operation string, args []byte, bridge HTTPBridge) (Result, error) {
	m.mu.RLock()
	e, ok := m.byName[name]
	if !ok {
		m.mu.RUnlock()
		return Result{}, ErrNotFound
	}
	e.active.RLock()
	m.mu.RUnlock()
	defer e.active.RUnlock()
	if e.liveness.State() != StateServing {
		return Result{}, fmt.Errorf("extension %q not serving (state=%s)", name, e.liveness.State())
	}

	m.mu.RLock()
	inv, dur := m.invocations[name], m.durations[name]
	m.mu.RUnlock()
	if inv != nil {
		inv.Add(1)
	}
	start := time.Now()
	res, err := e.module.Invoke(ctx, Call{Operation: operation, Args: args, Bridge: bridge})
	if dur != nil {
		dur.Add(uint64(time.Since(start)))
	}
	if err == nil {
		return res, nil
	}

	m.recordFailure(name, err)
	return res, err
}

// recordFailure classifies an invocation error and increments the
// per-plugin telemetry counters for it (RFC-0000 §14: isolate, record,
// apply policy, keep core alive).
func (m *Manager) recordFailure(name string, err error) {
	kind := FailureUnknown

	var ive *InvocationError
	if errors.As(err, &ive) {
		kind = ive.Kind
	}

	m.mu.RLock()
	f := m.telemetry[name]
	m.mu.RUnlock()
	if f == nil {
		f = &failureCounters{byKind: make(map[FailureKind]uint64)}
		m.mu.Lock()
		m.telemetry[name] = f
		m.mu.Unlock()
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.byKind[kind]++
	f.total++
	f.last = err.Error()

	m.logger.Warn("extension failure recorded",
		"extension", name,
		"kind", kind.String(),
		"error", err.Error(),
	)
}

// FailureStats summarizes a plugin's recorded failure telemetry.
type FailureStats struct {
	ByName map[FailureKind]uint64
	Total  uint64
	Last   string
}

// Failures returns the recorded failure telemetry for a named extension.
func (m *Manager) Failures(name string) FailureStats {
	m.mu.RLock()
	f := m.telemetry[name]
	m.mu.RUnlock()

	out := FailureStats{ByName: map[FailureKind]uint64{}}
	if f == nil {
		return out
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out.Total = f.total
	out.Last = f.last
	for k, v := range f.byKind {
		out.ByName[k] = v
	}
	return out
}

// PluginStats summarizes a plugin's operational counters for metrics
// exposition (/metrics).
type PluginStats struct {
	Invocations uint64
	DurationNs  uint64 // cumulative invoke time
	Reloads     uint64
	Failures    FailureStats
}

// Stats returns the recorded counters for a named extension.
func (m *Manager) Stats(name string) PluginStats {
	m.mu.RLock()
	inv, hasInv := m.invocations[name]
	dur := m.durations[name]
	rel := m.reloads[name]
	m.mu.RUnlock()

	out := PluginStats{Failures: m.Failures(name)}
	if hasInv {
		out.Invocations = inv.Load()
	}
	if dur != nil {
		out.DurationNs = dur.Load()
	}
	if rel != nil {
		out.Reloads = rel.Load()
	}
	return out
}

// State returns the lifecycle state of a named extension.
func (m *Manager) State(name string) (LifecycleState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.byName[name]
	if !ok {
		return StateClosed, ErrNotFound
	}
	return e.liveness.State(), nil
}

// List returns the names of all loaded extensions.
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.byName))
	for name := range m.byName {
		out = append(out, name)
	}
	return out
}

type hostModule struct {
	inner Module
	name  string
}

func (h *hostModule) Invoke(ctx context.Context, call Call) (Result, error) {
	return h.inner.Invoke(ctx, call)
}

func (h *hostModule) Close(ctx context.Context) error {
	return h.inner.Close(ctx)
}

type noopModule struct {
	name string
}

func (n *noopModule) Invoke(_ context.Context, _ Call) (Result, error) {
	return Result{Decision: "continue"}, nil
}

func (n *noopModule) Close(_ context.Context) error { return nil }
