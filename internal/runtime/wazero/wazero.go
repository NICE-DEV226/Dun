// Package wazero provides a Dun runtime adapter backed by the wazero
// WebAssembly runtime. This is the generation-1 Core-Wasm runtime.
//
// Generation 2 (WebAssembly Components) will be a separate adapter under
// runtime/component, keeping Dun's core independent from any one runtime.
package wazero

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/NICE-DEV226/Dun/internal/extension"
	"github.com/tetratelabs/wazero"
)

// ABI marker that every Dun plugin binary must export.
// This mirrors the Proxy-Wasm pattern (proxy_abi_version_0_2_1).
const ABIVersionMarker = "dun_abi_version_0_1"

// ErrMissingABIMarker is returned when a wasm artifact does not export the
// Dun ABI version marker function.
var ErrMissingABIMarker = fmt.Errorf("missing ABI marker export %q", ABIVersionMarker)

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
func (r *Runtime) Load(ctx context.Context, artifact []byte) (extension.Module, error) {
	compiled, err := r.rt.CompileModule(ctx, artifact)
	if err != nil {
		return nil, fmt.Errorf("compile module: %w", err)
	}
	r.logger.Debug("wasm module compiled", "bytes", len(artifact))
	return &nativeModule{runtime: r, compiled: compiled, logger: r.logger}, nil
}

// Close releases all resources held by the runtime.
func (r *Runtime) Close(ctx context.Context) error {
	return r.rt.Close(ctx)
}

type nativeModule struct {
	runtime  *Runtime
	compiled wazero.CompiledModule
	logger   *slog.Logger
}

func (m *nativeModule) Invoke(ctx context.Context, call extension.Call) (extension.Result, error) {
	// Instantiate a fresh instance per invocation (anonymous module / per-request).
	// This is the v0 default per RFC-0000 §15.1: every invocation is
	// stateless by construction.
	mod, err := m.runtime.rt.InstantiateModule(ctx, m.compiled, wazero.NewModuleConfig())
	if err != nil {
		return extension.Result{}, fmt.Errorf("instantiate module: %w", err)
	}
	defer func() {
		if err := mod.Close(ctx); err != nil {
			m.logger.Warn("failed to close module instance", "error", err)
		}
	}()

	fn := mod.ExportedFunction("dun_exported_instance_invoke")
	if fn == nil {
		if mod.ExportedFunction(ABIVersionMarker) == nil {
			return extension.Result{}, ErrMissingABIMarker
		}
		return extension.Result{Decision: "continue"}, nil
	}

	_, err = fn.Call(ctx, 0, 0)
	if err != nil {
		return extension.Result{}, fmt.Errorf("invoke: %w", err)
	}
	return extension.Result{Decision: "continue"}, nil
}

func (m *nativeModule) Close(ctx context.Context) error {
	return m.compiled.Close(ctx)
}

// IsABIError reports whether err is an ABI-marker error.
func IsABIError(err error) bool {
	return errors.Is(err, ErrMissingABIMarker)
}
