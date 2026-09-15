# Dun

**Go-native programmable HTTP data plane**

Dun is an open-source infrastructure project exploring WebAssembly as a
sandboxed extension boundary for high-performance HTTP proxying.

Built in Go with a deliberately small trusted core, wazero-based
WebAssembly execution, capability-oriented extensions, hot reload,
reproducible benchmarks, and a long-term path toward WebAssembly
Components.

## Why Dun

Reverse proxies and API gateways accumulate complexity as routing,
authentication, policy, transformation, and platform features grow into
the core. Dun takes the opposite direction:

- keep the native Go core small and understandable;
- make HTTP routing and proxying first-class;
- move application-specific behavior into isolated WebAssembly
  extensions;
- expose only explicit host capabilities to extensions;
- make extension lifecycle, resource limits, failures, and performance
  measurable;
- preserve a single self-contained Go binary as the default deployment
  artifact.

Dun is **not** an attempt to replace Envoy, Traefik, or Kong. Its niche
is a programmable HTTP data plane for engineers who want the simplicity
and hackability of Go with a serious sandboxed extension model.

## Current Status

Milestones 1-5 complete: **native proxy + sandboxed Wasm extension model proven end-to-end**.

Native proxy foundations:

- HTTP server
- route matching (prefix, exact, host, method)
- reverse proxy with upstream pool
- YAML configuration
- CLI (`dun run`, `dun check`, `dun version`)
- structured logging
- request metrics
- extension pipeline abstraction
- capability model (two-stage enforcement)
- plugin lifecycle state machine

Wasm extension boundary (v0 ABI, frozen):

- wazero runtime adapter with per-plugin isolated sandboxes
- hand-assembled WebAssembly guest module builder (no external toolchain needed)
- **security-header plugin** (wasm) loaded from disk, wired per-route
- host function API through the `dun` module (`.capability` naming)
- per-request instantiation with fresh module instances
- two-stage capability enforcement: manifest-vs-policy at load, authoritative host-module denial at instantiate
- end-to-end: client → Dun → wasm plugin → upstream (see `internal/http/server_test.go`)
- live example artifact at `plugins/examples/security-header/security-header.wasm`

Milestone 5 (Isolation & Limits) — the sandbox is operationally credible:

- **memory limits**: load-time enforcement (a module declaring more than the
  configured max memory fails fast at COMPILE, RFC-0000 §11)
- **execution timeouts**: per-request budget via context deadline;
  runaway guests are terminated, the request gets a 500, the core keeps serving
- **failure handling**: traps/timeouts/cancellations/host-call/instantiation
  failures classified (FailureKind) and recorded in per-plugin telemetry —
  the manager isolates failures and keeps the core alive (RFC-0000 §14)

Milestones 4+ are documented in the
[architecture specification](Dun_Project_Architecture_Specification_v0_2.md).

## Quick Start

```bash
git clone https://github.com/NICE-DEV226/Dun.git
cd Dun
go run ./cmd/dun -config examples/hello/dun.yaml
```

With an upstream on `:3000`:

```bash
curl http://localhost:8080/hello
```

### With the security-header Wasm plugin

```bash
go run ./cmd/dun -config examples/security-header/dun.yaml
```

Any `/api/*` request gets a `X-Security-Policy: default-src 'self'` header
added by the WebAssembly extension — see [docs/EXTENSIONS.md](docs/EXTENSIONS.md)
for the full v0 contract.

## Configuration

```yaml
listen:
  address: ":8080"

log:
  level: info
  format: text

routes:
  - name: api
    match:
      path_prefix: /api
    upstream: http://localhost:3000
    headers:
      X-Frame-Options: DENY
```

## CLI

```text
dun run       start the data plane
dun check     validate configuration
dun version   print version
```

## Architecture

See:

- [Architecture Specification](Dun_Project_Architecture_Specification_v0_2.md)
- [ARCHITECTURE](docs/ARCHITECTURE.md)
- [RFCs](rfcs/)

## Repository Layout

```text
cmd/dun                      CLI entry point
cmd/gen-wasm                 security-header wasm artifact generator
internal/config              configuration parsing and validation
internal/http                HTTP server + E2E pipeline wiring
internal/proxy               reverse proxy upstream pool
internal/router              route matching
internal/pipeline            extension pipeline abstraction
internal/extension           extension host, lifecycle, capabilities, manager
internal/guest               hand-assembled wasm module builder
internal/runtime/wazero      wazero runtime adapter
internal/metrics             in-process metrics
docs                         documentation
rfcs                         RFC documents
examples                     example configurations
plugins                      pre-built plugin artifacts (wasm)
```

## Roadmap

- Milestone 1: Native Proxy (complete)
- Milestone 2: Wasm Runtime (wazero integration) (complete)
- Milestone 3: Extension API (first plugin) (complete)
- Milestone 4: Multiple Plugins (complete — chained pipeline ordering)
- Milestone 5: Isolation & Limits (complete — memory, timeouts, failure telemetry)
- Milestone 6: Hot Reload
- Milestone 7: Benchmark Lab
- Milestone 8: Ecosystem Research

## License

Apache License 2.0. See [LICENSE](LICENSE).