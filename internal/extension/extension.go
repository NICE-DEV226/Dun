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
	"fmt"
	"time"
)

// ErrNotFound is returned when an extension cannot be found.
var ErrNotFound = errors.New("extension not found")

// ErrInvalidCapability is returned when an extension requests a capability
// that is not granted by policy.
var ErrInvalidCapability = errors.New("capability not granted")

// Wasm decision codes returned by a guest's dun_exported_instance_invoke or
// dun_process_request function. These are the raw ABI values; the pipeline
// maps them to pipeline.Decision.
const (
	WasmDecisionContinue uint32 = 0
	WasmDecisionRespond  uint32 = 1
	WasmDecisionReject   uint32 = 2
	WasmDecisionError    uint32 = 3
)

// DecisionString converts an ABI decision code to its stable name.
func DecisionString(code uint32) string {
	switch code {
	case WasmDecisionContinue:
		return "continue"
	case WasmDecisionRespond:
		return "respond"
	case WasmDecisionReject:
		return "reject"
	case WasmDecisionError:
		return "error"
	default:
		return "error"
	}
}

// HTTPBridge is the per-request, capability-scoped view of the HTTP
// transaction exposed to extensions. A plugin never touches Go objects
// directly — it operates through this bridge behind host functions.
type HTTPBridge interface {
	// RequestHeader reads a request header value.
	RequestHeader(name string) string
	// RequestHeaderValue returns the first value of a request header and
	// whether the header is present at all (an empty value still counts
	// as present).
	RequestHeaderValue(name string) (string, bool)
	// SetResponseHeader sets a header on the eventual response.
	SetResponseHeader(name, value string)
	// Log emits a structured log line from the extension.
	Log(level int, msg string)
}

// LoadOptions configures how a module is loaded.
type LoadOptions struct {
	// Capabilities is the validated policy granted to this extension.
	// Host functions are exposed only for capabilities present here.
	Capabilities Set
	// Limits are the resource limits enforced at compile/instantiation/
	// invocation time (RFC-0000 §11).
	Limits ResourceLimits
}

// ResourceLimits describes the sandbox resource limits for a plugin
// (RFC-0000 §11). A zero value means "no explicit limit" and leaves the
// runtime's default in effect.
type ResourceLimits struct {
	// MaxMemoryBytes bounds the guest linear memory in bytes.
	MaxMemoryBytes uint64
	// ExecutionTimeout bounds a single invocation (trap on deadline).
	ExecutionTimeout time.Duration
	// MaxBodyBytes bounds the buffered body copy exposed to body-reading
	// plugins only (RFC-0000 §11.1). Not yet enforced by host functions.
	MaxBodyBytes uint64
	// MaxInstances bounds concurrently running instances of this plugin.
	MaxInstances int
}

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
	// Bridge is the per-request capability-scoped host view (only used
	// when the runtime supports request-visible HTTP state).
	Bridge HTTPBridge
}

// Result represents the outcome of an invocation.
type Result struct {
	Decision string
	Data     []byte
}

// Host is the interface an extension manager uses to interact with runtimes.
type Host interface {
	// Load compiles a Wasm artifact into a Module, exposing exactly the
	// host functions implied by opts.Capabilities.
	Load(ctx context.Context, artifact []byte, opts LoadOptions) (Module, error)
	// CheckABI validates that the artifact exports a Dun ABI marker.
	CheckABI(ctx context.Context, artifact []byte) error
}

// FailureKind classifies an extension failure (RFC-0000 §14). The
// extension manager records telemetry keyed by these kinds so operators
// can distinguish operational problems (timeout, instantiation failure)
// from plugin bugs (trap) or policy violations (capability, host call).
type FailureKind uint8

const (
	// FailureUnknown is a failure that does not match any other kind.
	FailureUnknown FailureKind = iota
	// FailureInstantiate is a failure to create a fresh module instance.
	FailureInstantiate
	// FailureTrap is a guest runtime trap (unreachable, OOB access, ...).
	FailureTrap
	// FailureTimeout is an invocation that exceeded execution limits.
	FailureTimeout
	// FailureCancelled is an invocation cancelled by the caller.
	FailureCancelled
	// FailureHostCall is a host-call failure (bad bridge, bad memory read).
	FailureHostCall
)

// String returns the stable name of a failure kind.
func (k FailureKind) String() string {
	switch k {
	case FailureInstantiate:
		return "instantiate"
	case FailureTrap:
		return "trap"
	case FailureTimeout:
		return "timeout"
	case FailureCancelled:
		return "cancelled"
	case FailureHostCall:
		return "host_call"
	default:
		return "unknown"
	}
}

// InvocationError is an invocation failure carrying a FailureKind so the
// manager can record per-kind telemetry (RFC-0000 §14).
type InvocationError struct {
	Kind FailureKind
	Err  error
}

func (e *InvocationError) Error() string {
	return fmt.Sprintf("extension %s: %v", e.Kind.String(), e.Err)
}

func (e *InvocationError) Unwrap() error { return e.Err }
