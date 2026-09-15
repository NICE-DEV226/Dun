// Package wazero provides a Dun runtime adapter backed by the wazero
// WebAssembly runtime. This is the generation-1 Core-Wasm runtime.
//
// Generation 2 (WebAssembly Components) will be a separate adapter under
// runtime/component, keeping Dun's core independent from any one runtime.
//
// Runtime model per RFC-0000 §15.1: each extension is compiled once into a
// wazero.CompiledModule and instantiated fresh for every invocation (per-request
// instantiation). Also, every extension runs in its own dedicated wazero.Runtime
// so host functions are bound to exactly that plugin's capability set and module
// instances stay isolated from each other.
package wazero

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/NICE-DEV226/Dun/internal/extension"
	"github.com/NICE-DEV226/Dun/internal/guest"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// ABI marker that every Dun plugin binary must export.
// This mirrors the Proxy-Wasm pattern (proxy_abi_version_0_2_1).
const ABIVersionMarker = "dun_abi_version_0_1"

// ErrMissingABIMarker is returned when a wasm artifact does not export the
// Dun ABI version marker function.
var ErrMissingABIMarker = fmt.Errorf("missing ABI marker export %q", ABIVersionMarker)

// decision codes the guest returns from dun_process_request.
const exportedProcessFn = "dun_process_request"

type bridgeKey struct{}

// Runtime is a wazero-backed extension.Host.
type Runtime struct {
	rt     wazero.Runtime
	logger *slog.Logger
}

// NewRuntime creates a fresh wazero runtime.
// Each Runtime uses a fresh wazero.Runtime so modules are isolated.
func NewRuntime(logger *slog.Logger) (*Runtime, error) {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runtime{
		rt:     wazero.NewRuntime(context.Background()),
		logger: logger,
	}, nil
}

// CheckABI verifies the module exports the Dun ABI marker.
func (r *Runtime) CheckABI(ctx context.Context, artifact []byte) error {
	compiled, err := r.rt.CompileModule(ctx, artifact)
	if err != nil {
		return fmt.Errorf("compile module for ABI check: %w", err)
	}
	defer compiled.Close(ctx)
	if _, ok := compiled.ExportedFunctions()[ABIVersionMarker]; !ok {
		return ErrMissingABIMarker
	}
	return nil
}

// Load compiles a module and wraps it as an extension.Module.
// Each Load gets an isolated runtime so the exposed host functions are
// bound to exactly opts.Capabilities.
func (r *Runtime) Load(ctx context.Context, artifact []byte, opts extension.LoadOptions) (extension.Module, error) {
	if err := guest.ValidateWasm(artifact); err != nil {
		return nil, fmt.Errorf("load wasm: %w", err)
	}

	fresh := wazero.NewRuntime(ctx)
	cleaned := false
	cleanup := func() {
		if !cleaned {
			_ = fresh.Close(ctx)
		}
	}

	// Expose host functions for exactly the granted capabilities.
	fns := hostFuncs(opts.Capabilities, r.logger)
	if len(fns) > 0 {
		builder := fresh.NewHostModuleBuilder(guest.ABIModuleName)
		for _, f := range fns {
			builder = builder.NewFunctionBuilder().WithFunc(f.fn).Export(f.name)
		}
		if _, err := builder.Instantiate(ctx); err != nil {
			cleanup()
			return nil, fmt.Errorf("instantiate host module: %w", err)
		}
	}

	compiled, err := fresh.CompileModule(ctx, artifact)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("compile module: %w", err)
	}

	if _, ok := compiled.ExportedFunctions()[ABIVersionMarker]; !ok {
		_ = compiled.Close(ctx)
		cleanup()
		return nil, ErrMissingABIMarker
	}

	cleaned = true
	r.logger.Debug("wasm module compiled",
		"bytes", len(artifact),
		"capabilities", opts.Capabilities.String(),
	)
	return &nativeModule{runtime: fresh, compiled: compiled, logger: r.logger}, nil
}

// Close releases all resources held by the runtime's ABI-check pool.
func (r *Runtime) Close(ctx context.Context) error {
	return r.rt.Close(ctx)
}

// nativeModule is a compiled guest module plus its dedicated runtime.
type nativeModule struct {
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
	logger   *slog.Logger
}

// Invoke instantiates a fresh guest instance and runs dun_process_request.
// The bridge is attached to ctx so host functions see the current request.
func (m *nativeModule) Invoke(ctx context.Context, call extension.Call) (extension.Result, error) {
	if call.Bridge != nil {
		ctx = context.WithValue(ctx, bridgeKey{}, call.Bridge)
	}

	// Per-request instantiation from a pre-compiled module (§15.1).
	mod, err := m.runtime.InstantiateModule(ctx, m.compiled, wazero.NewModuleConfig())
	if err != nil {
		return extension.Result{}, fmt.Errorf("instantiate module: %w", err)
	}
	defer func() {
		if err := mod.Close(ctx); err != nil {
			m.logger.Warn("failed to close module instance", "error", err)
		}
	}()

	fn := mod.ExportedFunction(exportedProcessFn)
	if fn == nil {
		return extension.Result{}, fmt.Errorf("module does not export %q", exportedProcessFn)
	}

	results, err := fn.Call(ctx)
	if err != nil {
		return extension.Result{}, fmt.Errorf("invoke %q: %w", exportedProcessFn, err)
	}
	if len(results) != 1 {
		return extension.Result{}, fmt.Errorf("invoke returned %d results, want 1", len(results))
	}

	code := uint32(results[0])
	return extension.Result{Decision: extension.DecisionString(code)}, nil
}

// Close releases the compiled module and its dedicated runtime.
func (m *nativeModule) Close(ctx context.Context) error {
	err := m.compiled.Close(ctx)
	if cerr := m.runtime.Close(ctx); err == nil {
		err = cerr
	}
	return err
}

// IsABIError reports whether err is an ABI-marker error.
func IsABIError(err error) bool {
	return errors.Is(err, ErrMissingABIMarker)
}

// hostFuncName maps a capability to its exported host function name.
func hostFuncName(cap extension.Capability) string {
	return string(cap)
}

// hostFunc pairs a capability-gated host function with its export name.
type hostFunc struct {
	name string
	fn   any
}

// hostFuncs returns the host functions implied by the given capability set.
// A capability that is absent here is, by construction, unavailable to the
// guest: any import of it makes guest instantiation fail (authoritative
// runtime denial, RFC-0000 §10.1 stage 2).
func hostFuncs(caps extension.Set, logger *slog.Logger) []hostFunc {
	var fns []hostFunc

	if caps.Has(extension.CapResponseHeadersWrite) {
		fns = append(fns, hostFunc{
			name: hostFuncName(extension.CapResponseHeadersWrite),
			fn:   setResponseHeader,
		})
	}
	if caps.Has(extension.CapLogInfo) {
		fns = append(fns, hostFunc{
			name: hostFuncName(extension.CapLogInfo),
			fn:   logInfoHost(logger),
		})
	}
	if caps.Has(extension.CapLogError) {
		fns = append(fns, hostFunc{
			name: hostFuncName(extension.CapLogError),
			fn:   logErrorHost(logger),
		})
	}

	return fns
}

// setResponseHeader implements:
//
//	response.headers.write(name_off, name_len, value_off, value_len) -> i32 errno
//
// It reads two UTF-8 strings from guest linear memory and applies them to the
// request's response headers through the bridge. Returns errno 0 on success,
// non-zero on failure.
func setResponseHeader(ctx context.Context, mod api.Module, nameOff, nameLen, valueOff, valueLen uint32) uint32 {
	bridge, err := currentBridge(ctx)
	if err != nil {
		return 1
	}

	mem := mod.Memory()
	if mem == nil {
		return 1
	}

	name, ok := mem.Read(nameOff, nameLen)
	if !ok {
		return 1
	}
	value, ok := mem.Read(valueOff, valueLen)
	if !ok {
		return 1
	}

	bridge.SetResponseHeader(string(name), string(value))
	return 0
}

func logInfoHost(logger *slog.Logger) func(context.Context, api.Module, uint32, uint32) {
	return func(ctx context.Context, mod api.Module, msgOff, msgLen uint32) {
		bridge, err := currentBridge(ctx)
		if err != nil {
			logger.Warn("log.info without bridge", "error", err)
			return
		}
		if msg, ok := mod.Memory().Read(msgOff, msgLen); ok {
			bridge.Log(0, string(msg))
		}
	}
}

func logErrorHost(logger *slog.Logger) func(context.Context, api.Module, uint32, uint32) {
	return func(ctx context.Context, mod api.Module, msgOff, msgLen uint32) {
		bridge, err := currentBridge(ctx)
		if err != nil {
			logger.Warn("log.error without bridge", "error", err)
			return
		}
		if msg, ok := mod.Memory().Read(msgOff, msgLen); ok {
			bridge.Log(3, string(msg))
		}
	}
}

func currentBridge(ctx context.Context) (extension.HTTPBridge, error) {
	bridge, ok := ctx.Value(bridgeKey{}).(extension.HTTPBridge)
	if !ok || bridge == nil {
		return nil, errors.New("no HTTP bridge in invocation context")
	}
	return bridge, nil
}
