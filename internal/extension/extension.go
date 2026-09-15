// Package extension defines the extension host, lifecycle, capability model,
// manifest parsing, and extension manager for Dun.
//
// This is the boundary between the small trusted Go core and untrusted
// WebAssembly extensions. The capability model is enforced in two stages:
// load-time validation (fast-fail) and runtime denial (authoritative).
package extension

import (
	"context"
	"errors"
)

// ErrNotFound is returned when an extension cannot be found.
var ErrNotFound = errors.New("extension not found")

// ErrInvalidCapability is returned when an extension requests a capability
// that is not granted by policy.
var ErrInvalidCapability = errors.New("capability not granted")

// Extension represents a loaded extension ready to be instantiated.
type Extension struct {
	Manifest *Manifest
	Module   Module
}

// Module is the runtime-agnostic interface for a compiled Wasm module.
type Module interface {
	// Invoke runs the extension's handler for an invocation.
	Invoke(ctx context.Context, call Call) (Result, error)
	// Close releases runtime resources for the module.
	Close(ctx context.Context) error
}

// Call carries the host-visible context of a single invocation.
type Call struct {
	Operation string
	Args      []byte
}

// Result represents the outcome of an invocation.
type Result struct {
	Decision string
	Data     []byte
}

// Host is the interface an extension manager uses to interact with runtimes.
type Host interface {
	// Load compiles a Wasm artifact into a Module.
	Load(ctx context.Context, artifact []byte) (Module, error)
	// CheckABI validates that the artifact exports a Dun ABI marker.
	CheckABI(ctx context.Context, artifact []byte) error
}
