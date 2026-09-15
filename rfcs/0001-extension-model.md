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

- Exact ABI host-function signatures (Milestone 3)
- Streaming body semantics (RFC-0004)
- Proxy-Wasm compatibility scope (RFC-0006)

## References

- RFC-0000 Foundation
- Architecture specification §9, §12, §15, §18, §22