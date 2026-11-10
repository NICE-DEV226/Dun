# Dun

<p align="center">
  <img src="assets/logo.jpeg" alt="Dun logo" width="720">
</p>

**A Go-native programmable HTTP data plane with a WebAssembly extension boundary.**

Dun is an experimental infrastructure project for building small, explicit
and extensible HTTP proxies in Go. The trusted core owns routing, proxying,
configuration and lifecycle management. Application-specific behavior runs in
capability-scoped WebAssembly extensions hosted by [wazero](https://wazero.io/).

The project is designed for engineers who want a self-contained Go binary and
a serious sandbox boundary without turning the proxy core into an all-purpose
platform.

## What Dun Does

```text
client -> Dun router -> request extensions -> upstream service
                         |
                         +-- Wasm sandbox
                         +-- explicit capabilities
                         +-- limits and timeouts
                         +-- failure telemetry
```

Dun currently provides:

- exact, prefix, host and method-based routing;
- reverse proxying to HTTP and HTTPS upstreams;
- ordered per-route extension pipelines;
- Core-Wasm v0 plugins with capability-gated host imports;
- fresh Wasm instances per request for isolation;
- memory limits, execution timeouts and classified failures;
- atomic plugin reload with invocation draining;
- optional SHA-256 artifact pinning;
- `/healthz`, `/readyz` and Prometheus-compatible `/metrics` endpoints;
- structured logging and a dependency-free file watcher;
- public ABI constants plus TinyGo and Rust SDK foundations.

## Project Status

Dun is **pre-1.0 software**. The vertical path from HTTP request to Wasm
extension to upstream is implemented and tested, but the ABI is still v0 and
the project is not positioned as a replacement for Envoy, Traefik or Kong.

Stable enough for experimentation:

- native proxy and routing;
- Core-Wasm v0 contract;
- capability checks at load and instantiation;
- per-request instance isolation;
- lifecycle, reload and shutdown behavior;
- configuration validation and operational endpoints;
- conformance, integration and race-detector tests.

Explicitly not promised yet:

- pooled Wasm instances beyond the opt-in `limits.instances` model
  (pool-safe artifacts only, see `docs/EXTENSIONS.md`);
- response body transformation and streaming extension APIs;
- cryptographic plugin signatures or a remote registry;
- a compatibility guarantee beyond Core-Wasm v0;
- production-grade control-plane, service discovery or load balancing.

## Quick Start

Requirements: Go 1.27 or newer.

```bash
git clone https://github.com/NICE-DEV226/Dun.git
cd Dun
go run ./cmd/dun run -config examples/hello/dun.yaml
```

The example expects an upstream on `localhost:3000`:

```bash
curl http://localhost:8080/hello
```

Run the Wasm security-header example:

```bash
go run ./cmd/dun run -config examples/security-header/dun.yaml
curl -i http://localhost:8080/api/example
```

The plugin adds `X-Security-Policy: default-src 'self'` before the request is
forwarded to the upstream.

Three more Wasm plugin examples ship pre-built (each needs an upstream on
`localhost:3000`, run on its own port):

```bash
# :8082 — rejects requests without "Authorization: Bearer …" (403)
go run ./cmd/dun run -config examples/auth/dun.yaml

# :8083 — rejects requests carrying an X-Denied header (403)
go run ./cmd/dun run -config examples/deny-header/dun.yaml

# :8084 — adds three CORS response headers to every response
go run ./cmd/dun run -config examples/cors/dun.yaml
```

See [docs/EXTENSIONS.md](docs/EXTENSIONS.md#example-plugins) for what each
plugin does and which capability it uses.

Validate configuration without starting the data plane:

```bash
go run ./cmd/dun check -config examples/security-header/dun.yaml
go run ./cmd/dun version
go run ./cmd/dun plugin validate -config examples/security-header/dun.yaml
go run ./cmd/dun plugin inspect -config examples/security-header/dun.yaml
```

## First Real Gateway

The complete runnable gateway scenario is in
[examples/gateway](examples/gateway): a Go backend on `:3000`, Dun on `:8080`,
the security-header Wasm plugin, route-level security headers and SHA-256
artifact verification.

```bash
go run ./examples/gateway/backend
go run ./cmd/dun run -config examples/gateway/dun.yaml
```

In another terminal:

```bash
curl -i http://localhost:8080/api/hello
```

## Project Page

The static project page lives in [web](web). Preview it from the repository
root with:

```bash
python3 -m http.server 4173
```

Then open `http://localhost:4173/web/`.

The public GitHub Pages site is available at
<https://nice-dev226.github.io/Dun/>.

## Configuration

```yaml
listen:
  address: ":8080"

log:
  level: info
  format: json

routes:
  - name: api
    match:
      path_prefix: /api
      method: GET
    upstream: http://localhost:3000
    plugins:
      - security-header
    headers:
      X-Frame-Options: DENY

plugins:
  - name: security-header
    path: plugins/examples/security-header/security-header.wasm
    version: "0.1.0"
    capabilities:
      - response.headers.write
    limits:
      memory: 64MiB
      timeout: 5ms
    # Optional integrity pin:
    # sha256: <64 hexadecimal characters>
```

Configuration is rejected at startup when routes, upstreams, plugin
references, methods, paths or artifact digests are invalid.

## Extension Contract

The public contract is documented in [api/extension](api/extension) and
[docs/EXTENSIONS.md](docs/EXTENSIONS.md).

Required guest exports:

| Export | Signature | Contract |
| --- | --- | --- |
| `dun_abi_version_0_1` | `() -> i32` | returns `1` |
| `dun_process_request` | `() -> i32` | returns a decision code |

Decision codes are `0=CONTINUE`, `1=RESPOND`, `2=REJECT` and `3=ERROR`.

The v0 host imports are:

- `response.headers.write`
- `log.info`
- `log.error`

Capabilities are checked against deployment policy before loading. At runtime,
only authorized host imports are exposed to the guest. A denied import causes
instantiation to fail rather than silently widening plugin authority.

### SDKs

Minimal SDK foundations are available for:

- [TinyGo](sdk/tinygo/README.md)
- [Rust](sdk/rust/README.md)

The SDKs are intentionally small. They expose the ABI rather than hiding the
host security model behind a large framework.

## Operational Model

Every request gets a fresh Wasm module instance in v0. This is the reference
isolation model: guest memory and transient guest state do not survive the
request or a reload.

Configured execution limits include:

- initial linear memory limit;
- per-invocation execution timeout;
- artifact SHA-256 verification;
- classified trap, timeout, cancellation, host-call and instantiation errors.

The server exposes:

- `GET /healthz` for process liveness;
- `GET /readyz` for readiness;
- `GET /metrics` using Prometheus text exposition.

## Verification

Run the complete local checks:

```bash
go test ./...
go test -race ./...
go vet ./...
```

Focused suites:

```bash
go test ./tests/conformance
go test ./tests/integration
go test -bench BenchmarkInvokeSecurityHeader -benchmem ./internal/runtime/wazero
```

Benchmark Lab (end-to-end baseline matrix, see `docs/BENCHMARKS.md`):

```bash
go test ./benchmarks -bench . -benchmem -benchtime=2s
```

The benchmark is a reproducible baseline, not a universal performance claim.
On the development machine used for the current baseline it measures roughly
`33 us/op`, `48 allocs/op` for the small security-header guest. Benchmark
results depend on Go, wazero, CPU, operating system and build configuration.

## Architecture

The trusted path is intentionally small:

```text
cmd/dun
  -> config
  -> http server
       -> router
       -> pipeline
            -> extension manager
                 -> runtime/wazero
       -> proxy
```

Important design documents:

- [Architecture specification](Dun_Project_Architecture_Specification_v0_2.md)
- [Architecture overview](docs/ARCHITECTURE.md)
- [Extension contract](docs/EXTENSIONS.md)
- [Changelog](CHANGELOG.md)
- [Runnable gateway example](examples/gateway/README.md)
- [RFCs](rfcs/)

Repository areas:

```text
api/extension       public ABI names and values
cmd/dun             user-facing CLI
cmd/gen-wasm        example artifact generator
internal/config     YAML parsing and validation
internal/http       HTTP server and pipeline wiring
internal/proxy      reverse proxy implementation
internal/router     route matching
internal/pipeline   ordered extension execution
internal/extension  lifecycle, policy and manager
internal/runtime    Wasm host adapter
sdk/                TinyGo and Rust SDK foundations
tests/conformance   ABI compatibility tests
tests/integration   cross-package operational tests
benchmarks/         Benchmark Lab harness (Milestone 7)
plugins/            example artifacts
```

## Security

Wasm isolation is a defense-in-depth boundary, not a substitute for a
complete deployment security model. Operators should pin plugin artifacts,
run Dun with least privilege, restrict filesystem access and treat upstream
configuration as sensitive.

Report vulnerabilities privately as described in [SECURITY.md](SECURITY.md).

## Contributing

Start with [CONTRIBUTING.md](CONTRIBUTING.md). Architectural changes should
be proposed as RFCs. Changes to the public ABI require compatibility tests and
an explicit contract update.

```bash
gofmt -w .
go test ./...
go test -race ./...
go vet ./...
```

## License

Dun is licensed under the Apache License 2.0. See [LICENSE](LICENSE).
