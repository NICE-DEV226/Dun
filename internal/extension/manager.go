package extension

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// Manager owns the lifecycle of all loaded extensions.
type Manager struct {
	host     Host
	logger   *slog.Logger
	policies map[string]Set

	mu        sync.RWMutex
	byName    map[string]*entry
	supported map[string]bool
}

type entry struct {
	name     string
	manifest *Manifest
	module   Module
	liveness *Liveness
	policy   Set
}

// NewManager creates an extension manager bound to a runtime host.
func NewManager(host Host, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		host:     host,
		logger:   logger,
		policies: make(map[string]Set),
		byName:   make(map[string]*entry),
		supported: map[string]bool{
			"wasm": true,
		},
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
		loaded, err := m.host.Load(context.Background(), artifact, LoadOptions{Capabilities: policy})
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

// Close closes and removes an extension.
func (m *Manager) Close(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.byName[name]
	if !ok {
		return ErrNotFound
	}
	if err := e.liveness.Close(ctx); err != nil {
		return err
	}
	delete(m.byName, name)
	m.logger.Info("extension removed", "extension", name)
	return nil
}

// Invoke runs a named extension operation with an optional per-request bridge.
func (m *Manager) Invoke(ctx context.Context, name, operation string, args []byte, bridge HTTPBridge) (Result, error) {
	m.mu.RLock()
	e, ok := m.byName[name]
	m.mu.RUnlock()
	if !ok {
		return Result{}, ErrNotFound
	}
	if e.liveness.State() != StateServing {
		return Result{}, fmt.Errorf("extension %q not serving (state=%s)", name, e.liveness.State())
	}
	return e.module.Invoke(ctx, Call{Operation: operation, Args: args, Bridge: bridge})
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
