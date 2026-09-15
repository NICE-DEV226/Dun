# Dun Extension Contract v0 — Quick Reference

This document describes the minimal extension ABI proven end-to-end.

## Guest Exports

| Export                   | Signature | Meaning                          |
|--------------------------|-----------|----------------------------------|
| `dun_abi_version_0_1`   | `() -> i32` | ABI marker, must return 1     |
| `dun_process_request`   | `() -> i32` | Request handler (per-request) |

Decision codes: 0=CONTINUE, 1=RESPOND, 2=REJECT, 3=ERROR

## Host Imports (module "dun")

| Import name              | Signature                                    | Capability               |
|--------------------------|----------------------------------------------|--------------------------|
| `response.headers.write` | `(name_off:i32, name_len:i32, val_off:i32, val_len:i32) -> errno:i32` | `response.headers.write` |
| `log.info`               | `(msg_off:i32, msg_len:i32) -> ()`           | `log.info`               |
| `log.error`              | `(msg_off:i32, msg_len:i32) -> ()`           | `log.error`              |

## String passing

All strings: `(offset: i32, byte_length: i32)` pointing into guest linear memory.
Guest **must export memory** (minimum 1 page = 64 KiB).

## WAT: security-header plugin

```wat
(module
  (type (func (result i32)))
  (type (func (param i32 i32 i32 i32) (result i32)))
  (import "dun" "response.headers.write" (func $set (type 1)))
  (memory (export "memory") 1)
  (data (i32.const 0)  "X-Security-Policy")
  (data (i32.const 32) "default-src 'self'")
  (func $process (type 0) (result i32)
    i32.const 0  i32.const 17
    i32.const 32 i32.const 18
    call $set
    drop
    i32.const 0
  )
  (export "dun_abi_version_0_1" (func $process))
  (export "dun_process_request" (func $process))
)
```

## Configuration

```yaml
listen:
  address: ":8080"

routes:
  - name: api
    match:
      path_prefix: /api
    upstream: http://localhost:3000
    plugins:
      - security-header

plugins:
  - name: security-header
    path: plugins/examples/security-header/security-header.wasm
    version: "0.1.0"
    capabilities:
      - response.headers.write
```

## Capability enforcement (two stages)

- **Stage 1 (load-time, fast-fail):** manifest capabilities checked against deployment policy.
- **Stage 2 (runtime, authoritative):** host functions exposed only for granted capabilities; importing an absent function traps the guest instantiation.

## Resource limits (RFC-0000 §11)

Per-plugin sandbox limits are configured in YAML and enforced at three points:

```yaml
plugins:
  - name: security-header
    path: plugins/examples/security-header/security-header.wasm
    capabilities:
      - response.headers.write
    limits:
      memory: 64MiB     # max linear memory (KiB/MiB/GiB, binary units)
      timeout: 5ms      # per-request execution budget (Go duration)
      body: 1MiB        # max buffered body copy for body plugins (§11.1)
      instances: 8      # max concurrent instances (reserved)
```

| Limit      | Enforcement point      | Behavior                          |
|-----------|------------------------|-----------------------------------|
| `memory`  | COMPILE (load-time)    | rejects modules declaring more initial memory than allowed (fail fast) |
| `timeout` | INVOKE (per-request)   | terminates runaway guests via context deadline; request → 500, core stays alive |
| `body`    | §11.1 (with body caps) | not yet enforced (no body-read capability in v0) |
| `instances`| reserved              | not yet enforced                   |

Failure classification (`FailureKind`): `trap`, `timeout`, `cancelled`,
`host_call`, `instantiate`, `unknown`. The extension manager records
per-plugin telemetry for each kind (RFC-0000 §14: isolate, record, apply
policy, keep core alive).

## Hot reload (RFC-0000 §13, Milestone 6)

Plugin binaries can be swapped without restarting the data plane:

- `Manager.Reload(name, artifact, manifest)` performs **atomic
  activation**: the new binary is validated (capability vs policy,
  ABI marker) and compiled into a fresh isolated runtime while the old
  version keeps serving, then the running entry is swapped in one step.
  Any failure — bad artifact, rejected capability, compile error —
  leaves the previous version active.
- The old compiled module has nothing in flight (instances are
  per-request, §15.1; plugin state does not survive a reload, §13.1) so
  it is drained and closed right after the swap.
- `Server.WatchPlugins(ctx, interval)` polls every plugin wasm path with
  a dependency-free watcher and reloads on modification. `dun run`
  enables it with a 5s interval; logs show `plugin reloaded`.
- Lifecycle: an extension stays `SERVING` across a reload; reload is
  rejected for extensions not in `SERVING`.

```yaml
# changing security-header.wasm on disk is picked up automatically;
# new requests then run the new version's code.
```

## Writing real plugins

TinyGo: `tinygo build -o plugin.wasm -target wasm ./plugins/myplugin`
Rust: `cargo build --target wasm32-wasi --release`

A TinyGo SDK will be provided in `sdk/tinygo/` in a future milestone.
