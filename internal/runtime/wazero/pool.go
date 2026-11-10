package wazero

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/NICE-DEV226/Dun/internal/extension"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// This file implements the pooled instantiation model evaluated by
// Milestone 7 (Benchmark Lab, RFC-0000 §15.2). It is opt-in: a module is
// pooled only when its resource limits request MaxInstances > 1 AND the
// artifact is pool-safe (see poolableArtifact). The default remains
// per-request instantiation (§15.1).
//
// Pool correctness model: between uses, an instance must be indistinguish-
// able from a fresh one. The public wazero API cannot enumerate or reset
// globals/tables, so pooling is restricted to artifacts that cannot carry
// mutable state outside linear memory. Linear memory is restored from a
// post-instantiation snapshot; an instance whose memory grew (or that was
// closed by an interrupted call) is discarded instead of reused.

// pooledInstance is one checked-out guest instance plus the memory image
// captured right after instantiation (data segments applied, before any
// guest code ran).
type pooledInstance struct {
	mod       api.Module
	snapshot  []byte // nil when the module has no memory
	ephemeral bool   // untracked fallback instance, always closed on release
}

// pooledModule is an extension.Module that recycles a bounded set of
// instantiated guests instead of instantiating per request.
type pooledModule struct {
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
	logger   *slog.Logger
	limits   extension.ResourceLimits

	max   int
	slots chan struct{} // bounds concurrent Invoke to max

	mu   sync.Mutex
	idle []*pooledInstance
	live int // pooled instances created and not yet discarded

	closed bool
}

// newPooledModule wraps an already-compiled module with an instance pool
// of capacity max. The caller must have verified poolableArtifact.
func newPooledModule(runtime wazero.Runtime, compiled wazero.CompiledModule, logger *slog.Logger, limits extension.ResourceLimits, max int) *pooledModule {
	if logger == nil {
		logger = slog.Default()
	}
	return &pooledModule{
		runtime:  runtime,
		compiled: compiled,
		logger:   logger,
		limits:   limits,
		max:      max,
		slots:    make(chan struct{}, max),
	}
}

// Invoke checks out an instance, runs dun_process_request and returns the
// instance to the pool. Mirrors nativeModule.Invoke semantics: per-call
// timeout, classified errors, bridge via context.
func (m *pooledModule) Invoke(ctx context.Context, call extension.Call) (extension.Result, error) {
	if call.Bridge != nil {
		ctx = context.WithValue(ctx, bridgeKey{}, call.Bridge)
	}

	if m.limits.ExecutionTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.limits.ExecutionTimeout)
		defer cancel()
	}

	inst, err := m.checkout(ctx)
	if err != nil {
		return extension.Result{}, classifyInstantiateErr(err)
	}
	defer m.release(inst)

	fn := inst.mod.ExportedFunction(exportedProcessFn)
	if fn == nil {
		return extension.Result{}, &extension.InvocationError{
			Kind: extension.FailureInstantiate,
			Err:  fmt.Errorf("module does not export %q", exportedProcessFn),
		}
	}

	results, err := fn.Call(ctx)
	if err != nil {
		return extension.Result{}, classifyCallErr(err)
	}
	if len(results) != 1 {
		return extension.Result{}, &extension.InvocationError{
			Kind: extension.FailureInstantiate,
			Err:  fmt.Errorf("invoke returned %d results, want 1", len(results)),
		}
	}

	return extension.Result{Decision: extension.DecisionString(uint32(results[0]))}, nil
}

// checkout returns a reset, ready-to-use instance. It never blocks beyond
// the slots bound: with at most max concurrent Invoke calls and live <=
// max, a free slot implies either an idle instance or live < max.
func (m *pooledModule) checkout(ctx context.Context) (*pooledInstance, error) {
	select {
	case m.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	for {
		m.mu.Lock()
		if n := len(m.idle); n > 0 {
			inst := m.idle[n-1]
			m.idle = m.idle[:n-1]
			m.mu.Unlock()

			if m.usable(inst) && m.reset(inst) {
				return inst, nil
			}
			m.discard(inst)
			continue
		}
		if !m.closed && m.live < m.max {
			m.live++
			m.mu.Unlock()

			// Instantiate under a non-cancelable context: pooled instances
			// must outlive the request that created them. Request-scoped
			// cancellation still interrupts fn.Call via the invoke ctx.
			inst, err := m.instantiate(context.Background())
			if err != nil {
				m.mu.Lock()
				m.live--
				m.mu.Unlock()
				<-m.slots
				return nil, err
			}
			return inst, nil
		}
		m.mu.Unlock()

		// Defensive fallback (invariant says unreachable while slots cap ==
		// max): run an untracked per-request instance rather than fail.
		inst, err := m.instantiateEphemeral(ctx)
		if err != nil {
			<-m.slots
			return nil, err
		}
		return inst, nil
	}
}

// release returns an instance to the pool or discards it, then frees its
// concurrency slot. Safe on nil.
func (m *pooledModule) release(inst *pooledInstance) {
	if inst != nil {
		m.recycle(inst)
	}
	<-m.slots
}

// recycle pushes a still-usable instance back to idle, or closes it.
func (m *pooledModule) recycle(inst *pooledInstance) {
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()

	if !closed && !inst.ephemeral && m.usable(inst) {
		m.mu.Lock()
		m.idle = append(m.idle, inst)
		m.mu.Unlock()
		return
	}
	m.discard(inst)
}

// usable reports whether an instance can be reset and reused: not closed
// and memory still at its post-instantiation size (no memory.grow).
func (m *pooledModule) usable(inst *pooledInstance) bool {
	if inst.mod.IsClosed() {
		return false
	}
	mem := inst.mod.Memory()
	if mem == nil {
		return inst.snapshot == nil
	}
	return uint64(mem.Size()) == uint64(len(inst.snapshot))
}

// reset restores linear memory to its post-instantiation image. Callers
// must have checked usable first.
func (m *pooledModule) reset(inst *pooledInstance) bool {
	mem := inst.mod.Memory()
	if mem == nil {
		return inst.snapshot == nil
	}
	return mem.Write(0, inst.snapshot)
}

// discard closes an instance and updates the live count for pooled ones.
func (m *pooledModule) discard(inst *pooledInstance) {
	_ = inst.mod.Close(context.Background())
	if !inst.ephemeral {
		m.mu.Lock()
		m.live--
		m.mu.Unlock()
	}
}

// instantiate creates a pooled instance and snapshots its memory.
func (m *pooledModule) instantiate(ctx context.Context) (*pooledInstance, error) {
	mod, err := m.runtime.InstantiateModule(ctx, m.compiled, wazero.NewModuleConfig())
	if err != nil {
		return nil, err
	}
	inst := &pooledInstance{mod: mod}
	if mem := mod.Memory(); mem != nil {
		// Read returns a write-through view: clone before the guest can
		// mutate it, or the "snapshot" would alias live memory.
		buf, ok := mem.Read(0, mem.Size())
		if !ok {
			_ = mod.Close(ctx)
			return nil, fmt.Errorf("snapshot guest memory")
		}
		inst.snapshot = bytes.Clone(buf)
	}
	return inst, nil
}

// instantiateEphemeral creates an untracked instance used only by the
// defensive fallback path; it is always closed on release.
func (m *pooledModule) instantiateEphemeral(ctx context.Context) (*pooledInstance, error) {
	mod, err := m.runtime.InstantiateModule(ctx, m.compiled, wazero.NewModuleConfig())
	if err != nil {
		return nil, err
	}
	return &pooledInstance{mod: mod, ephemeral: true}, nil
}

// Close drains idle instances and releases the compiled module and its
// dedicated runtime. In-flight instances are closed by runtime.Close.
func (m *pooledModule) Close(ctx context.Context) error {
	m.mu.Lock()
	m.closed = true
	idle := m.idle
	m.idle = nil
	m.mu.Unlock()

	for _, inst := range idle {
		_ = inst.mod.Close(ctx)
	}

	err := m.compiled.Close(ctx)
	if cerr := m.runtime.Close(ctx); err == nil {
		err = cerr
	}
	return err
}

// poolableArtifact reports whether artifact can be pooled safely: no
// table, global or element sections (state the public wazero API cannot
// snapshot/reset), and no non-function imports. Anything unexpected makes
// the artifact unpoolable — pooling then silently degrades to the
// per-request model, which is always correct.
func poolableArtifact(artifact []byte) bool {
	const (
		wasmMagic = "\x00asm"
		secCustom = 0
		secImport = 2
		secTable  = 4
		secGlobal = 6
		secElem   = 9
	)
	if len(artifact) < 8 || string(artifact[:4]) != wasmMagic {
		return false
	}

	off := 8
	for off < len(artifact) {
		id := artifact[off]
		off++
		size, n := readULEB(artifact[off:])
		if n <= 0 {
			return false
		}
		off += n
		end := off + int(size)
		if end > len(artifact) || end < off {
			return false
		}

		switch id {
		case secTable, secGlobal, secElem:
			return false
		case secImport:
			if !importsAreFunctionsOnly(artifact[off:end]) {
				return false
			}
		case secCustom:
			// skip payload
		}
		off = end
	}
	return true
}

// importsAreFunctionsOnly parses an import section payload and reports
// whether every import is a function (kind 0x00). Any other kind — or a
// malformed section — makes the artifact unpoolable.
func importsAreFunctionsOnly(payload []byte) bool {
	count, n := readULEB(payload)
	if n <= 0 {
		return false
	}
	off := n
	for i := uint64(0); i < count; i++ {
		// module name, field name: length-prefixed UTF-8
		for j := 0; j < 2; j++ {
			l, m := readULEB(payload[off:])
			if m <= 0 || off+m+int(l) > len(payload) {
				return false
			}
			off += m + int(l)
		}
		if off >= len(payload) {
			return false
		}
		kind := payload[off]
		off++
		if kind != 0x00 {
			return false
		}
		// function import descriptor: a single u32 type index
		_, m := readULEB(payload[off:])
		if m <= 0 {
			return false
		}
		off += m
	}
	return off == len(payload)
}

// readULEB decodes an unsigned LEB128 integer. Returns (value, bytesRead);
// bytesRead <= 0 on malformed input.
func readULEB(b []byte) (uint64, int) {
	var v uint64
	var shift uint
	for i := 0; i < len(b) && i < 10; i++ {
		c := b[i]
		v |= uint64(c&0x7f) << shift
		if c&0x80 == 0 {
			return v, i + 1
		}
		shift += 7
	}
	return 0, -1
}
