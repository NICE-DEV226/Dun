# Architecture

This document describes the current Dun architecture and where it is
heading. It is a companion to the full
[architecture specification](../Dun_Project_Architecture_Specification_v0_2.md).

## Principles

1. **Small trusted core** — privileged or data-plane-critical code only.
2. **Programmability over feature accumulation** — application-specific
   behavior lives in extensions.
3. **Capabilities, not implicit authority** — plugins receive only the
   capabilities they need.
4. **Measurements over marketing** — claims require reproducible
   benchmarks.
5. **Evolution without architectural lock-in** — the extension API must
   not depend permanently on one Wasm runtime or ABI.

## Current Layout (Milestone 1)

```text
                  ┌──────────────┐
                  │   cmd/dun    │
                  └──────┬───────┘
                         │
                  ┌──────▼───────┐
                  │  internal/   │
                  │  http.Server │
                  └──────┬───────┘
                         │
              ┌──────────┼──────────┐
              │          │          │
        internal/   internal/   internal/
        router      pipeline     proxy
              │          │          │
              └──────────┼──────────┘
                         │
                  ┌──────▼───────┐
                  │  Upstream    │
                  └──────────────┘
```

## HTTP Server

`internal/http.Server` is the request entry point. For each request it:

1. matches a route via `internal/router`;
2. applies route-level headers;
3. runs the extension pipeline (Milestone 2+);
4. proxies to the matched upstream via `internal/proxy`.

## Router

`internal/router.Router` matches requests against configured rules:

- `path_prefix` / `path_exact`
- `host`
- `method`

The first matching route wins. No match results in `404`.

## Reverse Proxy

`internal/proxy.UpstreamPool` owns `httputil.ReverseProxy` instances per
upstream. Upstreams are registered by route name at startup. Standard hop
headers are handled by Go's reverse proxy.

## Extension Pipeline

`internal/pipeline.Pipeline` is the ordered sequence of extensions. Each
extension returns one of:

- `CONTINUE` — proceed to next extension
- `RESPOND` — extension writes the response
- `REJECT` — request denied (403)
- `ERROR` — extension failure (500)

The pipeline is currently instantiated without extensions. Milestone 2+
wires it to the extension manager.

## Extension Host

`internal/extension` provides:

- `Manager` — owns plugin lifecycle
- `Liveness` — DISCOVER → VALIDATE → LOAD → COMPILE → INSTANTIATE →
  INITIALIZE → READY → SERVING → DRAINING → CLOSED
- `Set` / capability model — two-stage enforcement scaffolding
- `Manifest` — YAML plugin metadata

## Runtime Abstraction

`internal/runtime/wazero` is the Generation 1 runtime adapter. It keeps
the rest of the project decoupled from wazero through the
`extension.Host` and `extension.Module` interfaces. Generation 2
(WebAssembly Components) will be a separate adapter.

## Capability Model

Two-stage enforcement (RFC-0000 §10.1):

1. **Load-time validation** — the manifest's declared capabilities are
   checked against deployment policy before instantiation.
2. **Runtime denial (authoritative)** — every host call is checked
   against the validated capability set, regardless of the manifest's
   claims.

## Metrics

`internal/metrics` provides lightweight counters:

- `requests_total`
- `requests_not_found`
- `requests_rejected`
- `requests_responded`
- `extension_errors`
- `request_duration_seconds`

## Decision Log Highlights

| Decision                      | Status    |
| ----------------------------- | --------- |
| Go-native core                | Accepted  |
| Pure Go / no CGO              | Accepted  |
| wazero for initial runtime    | Accepted  |
| Per-request instantiation     | v0 default|
| Two-stage capability checks   | Accepted  |
| ABI versioning via marker fn  | Accepted  |
| Instance pooling              | M7        |
| Hot reload                    | Accepted  |

See the full specification for the complete decision log.