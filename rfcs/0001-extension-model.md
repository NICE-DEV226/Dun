# RFC-0001 — Extension Model

**Status:** Draft
**Date:** 2026-09-14
**Replaces:** Sections of RFC-0000 companion (architecture specification)

## Abstract

This RFC freezes the first concrete Dun extension contract. It defines
the extension lifecycle, the invocation model, the request/response
interface, capability semantics, and failure behavior for Dun v0.x.

## Context

The architecture specification resolves the open design questions:
per-request instantiation (default), scoped `max_body_size`, no
instance state across requests/reloads, ABI versioning via an exported
marker function, and two-stage capability enforcement. RFC-0001 treats
these as inherited defaults.

## Extension Contract v0

### Artifact

An extension is a WebAssembly Core Module (WASI-enabled) that exports:

```
dun_abi_version_0_1  () -> i32
```

This marker is the authoritative ABI version gate, checked at `VALIDATE`.
The manifest's `api:` field is descriptive metadata only.

### Lifecycle

```
DISCOVER -> VALIDATE -> LOAD -> COMPILE -> INSTANTIATE
        -> INITIALIZE -> READY -> SERVING -> DRAINING -> CLOSED
```

A plugin is not active until:
1. binary validated, including the ABI marker;
2. metadata valid;
3. required capabilities permitted;
4. runtime successfully instantiates it;
5. initialization succeeds.

### Invocation

Each request instantiates a fresh module instance from a pre-compiled
`CompiledModule` and closes it after the invocation. There is no
instance state between requests.

### Decisions

| Decision  | Meaning                                      |
| --------- | -------------------------------------------- |
| CONTINUE  | proceed to the next extension                |
| RESPOND   | extension writes the response                |
| REJECT    | request denied (403)                        |
| ERROR     | internal extension error (500)              |

### Capabilities

A plugin receives only the capabilities declared in its manifest and
granted by deployment policy. Runtime denial is authoritative.

## Open Items

- Streaming body semantics (RFC-0004)
- Proxy-Wasm compatibility scope (RFC-0006)
- TinyGo/Rust SDKs (`sdk/tinygo/`, `sdk/rust/`)
- Envoy-style filter set, instance pools (post v0.x)

## Milestone 3 Implementation Notes

The ABI v0 references in this RFC are now proven end-to-end and frozen:
see `docs/EXTENSIONS.md` for the exact host-function table (`dun`
module), guest exports, string-passing convention, and the committed
security-header example artifact at `plugins/examples/security-header/`.

## Milestone 5 Implementation Notes (Isolation & Limits)

Per-plugin sandbox limits are enforced three ways (details in
`docs/EXTENSIONS.md`): memory via `WithMemoryLimitPages` at COMPILE,
execution timeouts via `WithCloseOnContextDone` + per-invocation context
deadline, and failure classification via a `FailureKind`-carrying
`InvocationError` recorded as per-plugin telemetry by the manager
(§14: isolate, record, apply policy, keep core alive).

## Milestone 6 Implementation Notes (Hot Reload)

Hot reload follows §13 atomic activation: a reloaded binary is validated
(stage-1 capability vs policy, ABI marker) and compiled into a fresh
isolated runtime while the previous version keeps SERVING; only then is
the running entry swapped under a single lock (§ manager.Reload). Because
instances are per-request (§15.1) the old compiled module has nothing in
flight and is drained + closed immediately after the swap. Any failure in
the pipeline (validation, compile, swap) leaves the previous version
active. Live triggers come from a dependency-free poll-based file watcher
(`internal/reload`, no fsnotify) wired through `Server.WatchPlugins`;
tests prove a changed wasm on disk becomes visible to new requests without
restarting the core.

## References

- RFC-0000 Foundation
- Architecture specification §9, §12, §13, §15, §18, §22
- docs/EXTENSIONS.md (frozen v0 contract)