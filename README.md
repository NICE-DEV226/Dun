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

Milestone 1 (Native Proxy) foundation:

- HTTP server
- route matching (prefix, exact, host, method)
- reverse proxy with upstream pool
- YAML configuration
- CLI (`dun run`, `dun check`, `dun version`)
- structured logging
- request metrics
- extension pipeline abstraction
- capability model (two-stage enforcement scaffolding)
- plugin lifecycle state machine
- wazero runtime adapter skeleton

Milestone 2 (Wasm runtime integration) and beyond are documented in the
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
internal/config              configuration parsing and validation
internal/http                HTTP server
internal/proxy               reverse proxy upstream pool
internal/router              route matching
internal/pipeline            extension pipeline abstraction
internal/extension           extension host, lifecycle, capabilities, manager
internal/runtime/wazero      wazero runtime adapter
internal/metrics             in-process metrics
docs                         documentation
rfcs                         RFC documents
examples                     example configurations
```

## Roadmap

- Milestone 1: Native Proxy (in progress)
- Milestone 2: Wasm Runtime (wazero integration)
- Milestone 3: Extension API (first plugin)
- Milestone 4: Multiple Plugins
- Milestone 5: Isolation & Limits
- Milestone 6: Hot Reload
- Milestone 7: Benchmark Lab
- Milestone 8: Ecosystem Research

## License

Apache License 2.0. See [LICENSE](LICENSE).