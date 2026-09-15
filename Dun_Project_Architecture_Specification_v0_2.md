# DUN --- Project Architecture & Engineering Specification

## RFC-0000 --- Foundation Document / Project Blueprint

**Status:** Draft\
**Version:** 0.2\
**Date:** 2026-09-14\
**Project:** Dun\
**Domain:** Go / HTTP infrastructure / WebAssembly / extensibility\
**Primary implementation language:** Go\
**Initial WebAssembly runtime:** wazero\
**License target:** Apache-2.0\
**Audience:** Go developers, infrastructure engineers, WebAssembly
engineers, open-source contributors

------------------------------------------------------------------------

# Changelog since v0.1

v0.2 resolves five design questions that v0.1 left open as "research
tracks," corrects one repository-structure inconsistency, and adds one
naming-collision note. Every resolution below is grounded in verifiable
current practice (wazero's own documentation, the Proxy-Wasm spec, WASI's
release notes), not invented benchmarks.

- **§15 Concurrency model** — decided: per-request instantiation is the v0
  default, not an open research track. Pooling becomes a Milestone 7
  benchmark comparison.
- **§11 Resource limits** — `max_body_size` scope clarified: it bounds only
  the plugin body-read/write path, not all proxied traffic.
- **§13 Hot Reload** — clarified: plugin-instance state does not survive
  reload (a direct consequence of per-request instantiation); durable state
  must live behind a host-provided capability.
- **§18 ABI Design Philosophy / §22 Plugin Metadata** — decided: ABI version
  is enforced via an exported marker function checked at `VALIDATE`, not
  via the manifest alone. The manifest's `api:` field is now explicitly
  scoped as UX/tooling metadata, not the enforcement mechanism.
- **§10 Capability Model / §14 Failure Model** — decided: capability
  violations are caught in two stages (load-time fail-fast, runtime
  authoritative deny), not one or the other.
- **§31 Repository Structure** — corrected: `rfcs/` was missing
  `0008-plugin-signing.md`, which §39 already defines.
- **§18 / new note** — flagged: `github.com/lavalagends/dun` already exists
  as an unrelated, inactive Go module (a renamed `gobuster` fork). Not a
  blocking collision, but worth 5 minutes of checking before the name is
  frozen in RFC-0001.

Everything else in v0.1 is unchanged.

------------------------------------------------------------------------

# 1. Executive Summary

Dun is an open-source, Go-native HTTP data plane designed around a
deliberately small trusted core and a programmable extension layer.

The project starts from a simple observation:

> Reverse proxies and API gateways become increasingly difficult to
> understand as routing, authentication, policy, transformation,
> observability, and platform-specific features accumulate in the core.

Dun takes the opposite architectural direction:

-   keep the native Go core small and understandable;
-   make HTTP routing and proxying first-class;
-   move application-specific behavior into isolated WebAssembly
    extensions;
-   expose only explicit host capabilities to extensions;
-   make extension lifecycle, resource limits, failures, and performance
    measurable;
-   preserve a single self-contained Go binary as the default deployment
    artifact;
-   design the extension contract so that the initial Core-Wasm
    implementation can evolve toward WebAssembly Components without
    forcing the core to depend on one runtime forever.

Dun is **not** intended to replace Envoy, Traefik, Kong, or service
meshes in every scenario.

Its intended niche is:

> **A programmable HTTP data plane for engineers who want the simplicity
> and hackability of Go with a serious sandboxed extension model.**

The long-term research question behind Dun is:

> **Can WebAssembly become a practical, measurable extension boundary
> for high-performance Go HTTP infrastructure?**

------------------------------------------------------------------------

# 2. Why Dun Exists

## 2.1 The problem

Modern proxies and gateways solve many problems:

-   reverse proxying;
-   TLS termination;
-   routing;
-   authentication;
-   authorization;
-   rate limiting;
-   traffic transformation;
-   observability;
-   service discovery;
-   policy;
-   retries;
-   load balancing;
-   integrations;
-   plugins.

The problem is not that these capabilities exist.

The problem is architectural growth.

A system can progressively become:

``` text
small proxy
    ↓
proxy + middleware
    ↓
proxy + plugin framework
    ↓
gateway
    ↓
control plane
    ↓
platform
```

At that point, the original networking primitive becomes difficult for a
developer to understand, modify, benchmark, or embed.

Dun deliberately keeps the core small while moving specialized behavior
across a well-defined extension boundary.

## 2.2 The opportunity

The Go ecosystem is a strong target for infrastructure tooling. The 2025
Go Developer Survey reports that more than one third of respondents
specifically build cloud infrastructure tooling, while Go continues to
be used heavily for APIs, command-line tooling, and open-source
projects.

Dun therefore targets developers who value:

-   readable systems code;
-   simple deployment;
-   static/self-contained binaries;
-   cross-compilation;
-   predictable operational behavior;
-   benchmarks;
-   explicit interfaces;
-   open-source extensibility.

------------------------------------------------------------------------

# 3. Project Thesis

Dun is built around five principles.

## Principle 1 --- Small trusted core

The core should contain only capabilities that require privileged native
access or define the HTTP data plane.

## Principle 2 --- Programmability over feature accumulation

When a feature is application-specific, it should preferably become an
extension instead of another permanent core subsystem.

## Principle 3 --- Capabilities, not implicit authority

A plugin should receive only the host capabilities it needs.

## Principle 4 --- Measurements over marketing

Claims such as "fast", "low overhead", and "secure" must be supported by
reproducible benchmarks, profiling, tests, and documented limitations.

## Principle 5 --- Evolution without architectural lock-in

The extension API must not make Dun permanently dependent on one
WebAssembly runtime or one Wasm ABI.

------------------------------------------------------------------------

# 4. Competitive Positioning

Dun must explicitly acknowledge existing solutions.

## 4.1 Envoy

Envoy is a mature, production-grade proxy/data plane with a broad
ecosystem and WebAssembly extension support.

Dun does not compete with Envoy on feature count or enterprise maturity.

Dun differentiates through:

-   smaller core;
-   Go-native implementation;
-   self-contained binary;
-   simpler mental model;
-   experimental extension architecture;
-   public performance research.

## 4.2 Traefik

Traefik provides a developer-friendly reverse proxy and supports Wasm
middleware plugins.

Dun should not claim that "Wasm plugins are new".

Instead, Dun investigates a more fundamental question:

> What should the extension boundary of a small Go-native data plane
> look like if it is designed around isolation, capabilities, streaming,
> lifecycle, and measurable overhead from the beginning?

## 4.3 Proxy-Wasm

Proxy-Wasm is a critical reference point.

Its current widely implemented specification is v0.2.1 and it already
provides a proxy-oriented Wasm ABI with SDKs for languages including Go,
Rust, C++ and AssemblyScript and hosts including Envoy, NGINX, ATS and
MOSN.

Therefore:

**Dun must not present "Wasm extensions for proxies" as its
innovation.**

Dun should instead explore:

1.  a minimal native Dun extension contract;
2.  a possible Proxy-Wasm compatibility layer;
3.  a future Component Model-based extension model;
4.  capability-oriented security;
5.  lifecycle and performance research.

Proxy-Wasm is also, as of v0.2, Dun's direct precedent for how to solve
ABI versioning (see §18) — this is treated as prior art to learn from,
not to copy wholesale.

------------------------------------------------------------------------

# 5. WebAssembly Strategy

## 5.1 Generation 1 --- Core Wasm

The first production-oriented runtime target is WebAssembly Core Modules
hosted through wazero.

Why wazero:

-   written in Go;
-   zero dependencies;
-   no CGO requirement;
-   supports WebAssembly Core;
-   supports WASI;
-   provides compiler and interpreter modes;
-   designed to be embedded into Go applications;
-   provides isolated Wasm modules.

This directly supports Dun's one-binary and cross-compilation goals.

## 5.2 Generation 2 --- Component Model

WebAssembly Components are strategically important for Dun's future.

The Component Model provides typed interfaces through WIT and is
evolving toward asynchronous primitives and streaming. WASI 0.3 shipped
on June 11, 2026 and adds native async primitives such as `async func`,
`stream<T>`, and `future<T>`, removing the `wasi:io` package entirely in
favor of native Component Model async.

However, Dun must not prematurely depend on Component Model support in
its first runtime.

The initial architecture therefore separates:

``` text
Dun Extension Contract
        │
        ├── Core-Wasm runtime adapter
        │       └── wazero
        │
        └── Component runtime adapter
                └── future implementation
```

The second adapter is a future research track, not an MVP dependency.

------------------------------------------------------------------------

# 6. High-Level Architecture

``` text
                         ┌──────────────────────┐
                         │         DUN          │
                         │      Go process      │
                         └──────────┬───────────┘
                                    │
                ┌───────────────────┼───────────────────┐
                │                   │                   │
          HTTP Server            Router             Config
                │                   │                   │
                └───────────────────┼───────────────────┘
                                    │
                              Request Pipeline
                                    │
                         ┌──────────┴──────────┐
                         │ Extension Host      │
                         │                     │
                         │ lifecycle           │
                         │ capabilities       │
                         │ resource limits    │
                         │ runtime adapters    │
                         └──────────┬──────────┘
                                    │
                     ┌──────────────┴──────────────┐
                     │                             │
               Core-Wasm                      Components
                     │                             │
                  wazero                   future runtime
                     │
          ┌──────────┼──────────┐
          │          │          │
        auth      policy     transform
        .wasm       .wasm       .wasm
                     │
                     ▼
                  Upstream
```

------------------------------------------------------------------------

# 7. Trusted Computing Base

The Trusted Computing Base (TCB) should be intentionally small.

## 7.1 Trusted components

-   Go runtime;
-   HTTP server;
-   router;
-   reverse proxy;
-   TLS handling;
-   extension manager;
-   capability enforcement;
-   Wasm runtime;
-   configuration parser;
-   plugin verification.

## 7.2 Untrusted components

Every third-party extension is considered untrusted by default.

Examples:

-   JWT middleware;
-   custom authentication;
-   rate limiting;
-   header transformation;
-   request policy;
-   response transformation;
-   tenant routing;
-   custom logging.

The core must not assume that a plugin is correct simply because it was
installed locally.

------------------------------------------------------------------------

# 8. Request Lifecycle

The canonical request path is:

``` text
Client
  │
  ▼
HTTP listener
  │
  ▼
Request parsing
  │
  ▼
Route matching
  │
  ▼
Extension pipeline
  │
  ├── plugin A
  ├── plugin B
  └── plugin C
  │
  ▼
Upstream selection
  │
  ▼
Reverse proxy
  │
  ▼
Upstream service
  │
  ▼
Response
  │
  ▼
Response extension pipeline
  │
  ▼
Client
```

Each extension must be able to return a decision.

Initial conceptual decisions:

``` text
CONTINUE
RESPOND
REJECT
ERROR
```

A plugin must never directly manipulate Go objects.

------------------------------------------------------------------------

# 9. Extension API v0

The first extension API should intentionally be small.

## 9.1 Request

Conceptual capabilities:

``` text
request.method.read
request.uri.read
request.headers.read
request.headers.write
request.body.read
request.body.write
```

## 9.2 Response

``` text
response.status.read
response.status.write
response.headers.read
response.headers.write
response.body.read
response.body.write
```

## 9.3 Context

``` text
context.metadata.read
context.metadata.write
context.state.read
context.state.write
```

`context.state` is host-owned storage, not plugin-instance memory (see
§13 and §15) — it is the mechanism a plugin uses to keep data that must
survive across requests or across a hot reload, since the Wasm instance
itself never does.

## 9.4 Logging

``` text
log.debug
log.info
log.warn
log.error
```

## 9.5 Decision

``` text
continue
respond
reject
error
```

No filesystem, process, environment, arbitrary network, or native memory
access is exposed by default.

------------------------------------------------------------------------

# 10. Why a Capability Model

A plugin should not receive "the proxy".

It should receive a constrained view of the proxy.

Example:

``` yaml
plugin:
  name: jwt-auth
  version: 0.1.0

capabilities:
  request:
    - headers.read
  response:
    - status.write
    - headers.write
  context:
    - metadata.write
```

## 10.1 Enforcement: two stages, not one

A plugin requesting an undeclared capability is denied in **two
sequential stages**, not by either mechanism alone:

1. **Load-time validation (`VALIDATE`, §12).** The manifest's declared
   capabilities are checked against deployment policy before the plugin
   is ever instantiated. This is the fast-fail layer: it gives a clear,
   actionable error before serving traffic, and it is what makes tooling
   like `dun plugin inspect` (§23) meaningful.
2. **Runtime denial (authoritative).** Every host call the instance
   actually makes is checked against the capability set that was
   validated for it, independent of what the manifest claimed. This is
   the layer that actually stops a plugin whose manifest under-declared
   or lied about its behavior — static validation only ever checked the
   manifest, never the compiled module's real behavior.

Stage 2 is authoritative; stage 1 exists purely for fast, cheap
rejection of honestly-declared-but-disallowed plugins.

Future capabilities may include:

``` text
network.connect
secrets.read
clock.read
random.read
metrics.write
cache.read
cache.write
```

These should be opt-in and policy-controlled.

------------------------------------------------------------------------

# 11. Resource Limits

Sandboxing alone is not enough.

Every extension should eventually support configurable limits:

``` text
max_memory
max_execution_time
max_fuel
max_body_size
max_header_size
max_instances
max_concurrent_invocations
```

Example:

``` yaml
limits:
  memory: 64MiB
  execution: 5ms
  body: 1MiB
  instances: 8
```

## 11.1 Scope of `max_body_size`

`max_body_size` bounds only the buffered copy made available to a
plugin's `request.body.read`/`request.body.write` or
`response.body.read`/`response.body.write` capability (the "naive model"
described in §16). It has **no effect** on a request whose matched route
has no body-reading plugin: that traffic streams through Dun's reverse
proxy natively (Go's `io.Copy` path) regardless of body size. Without
this scoping, a reader could conclude Dun caps all proxied traffic at a
few hundred KiB, which would contradict its positioning as a
general-purpose HTTP data plane.

The exact mechanism otherwise depends on runtime capabilities and must
be benchmarked.

------------------------------------------------------------------------

# 12. Plugin Lifecycle

The lifecycle must be explicit.

``` text
DISCOVER
   ↓
VALIDATE
   ↓
LOAD
   ↓
COMPILE
   ↓
INSTANTIATE
   ↓
INITIALIZE
   ↓
READY
   ↓
SERVING
   ↓
DRAINING
   ↓
CLOSED
```

A plugin is not considered active until:

1.  its binary is validated, including its exported ABI-version marker
    (see §18) and declared capabilities against policy (§10.1, stage 1);
2.  its metadata is valid;
3.  required capabilities are permitted;
4.  the runtime successfully instantiates it;
5.  initialization succeeds.

------------------------------------------------------------------------

# 13. Hot Reload

Hot reload must be designed around atomic activation rather than
replacing a running instance in-place.

``` text
                  v1
                  │
               SERVING
                  │
             file changed
                  │
                  ▼
              validate v2
                  │
                  ▼
             instantiate v2
                  │
                  ▼
              health check
                  │
                  ▼
             atomic activate
                  │
        ┌─────────┴─────────┐
        │                   │
     new reqs             old reqs
        │                   │
        ▼                   ▼
        v2                   v1
                            │
                          drain
                            │
                          close
```

Requirements:

-   no interruption of existing connections;
-   no partially initialized plugin becoming visible;
-   new requests use the new version;
-   existing requests may complete on the old version;
-   failed reload leaves the previous version active.

## 13.1 Plugin-instance state does not survive reload

Because §15 fixes per-request instantiation as the v0 default, a plugin
instance is already stateless *between two ordinary requests* — there is
no in-instance state that could survive a reload even in principle,
since no two requests ever share an instance to begin with. This
resolves what would otherwise be an open question: "does state survive
reload" and "does state survive between requests" are the same question
under this model, and the answer to both is no.

Any state a plugin needs across requests (a rate-limit counter, a cache)
must live behind a host-provided capability — `context.state` (§9.3) or
a future `cache.read`/`cache.write` capability (§10) — which is Go-side
and therefore untouched by instance replacement. The atomic-activation
diagram above only needs to preserve *that* state, not anything inside
the Wasm instance.

------------------------------------------------------------------------

# 14. Failure Model

A plugin can fail in several ways:

``` text
compile failure
initialization failure
trap
timeout
resource exhaustion
invalid host call
capability violation
malformed output
```

## 14.1 Capability violation is detected in two places

Per §10.1: (1) at `VALIDATE`, by rejecting plugins whose manifest
requests capabilities the deployment policy doesn't grant, and (2) at
runtime, by denying any host call whose capability was not in the
validated set for this instance, regardless of what the manifest
claimed. (2) is the authoritative enforcement point; (1) is the
fast-fail UX layer that makes bad configuration visible immediately
instead of as a runtime surprise.

The desired behavior for all failure modes is:

``` text
plugin failure
     │
     ▼
extension manager
     │
     ├── isolate failure
     ├── record telemetry
     ├── apply configured policy
     └── keep core alive
```

Important caveat:

**Wasm sandboxing does not automatically make the entire host
invulnerable.**

The runtime itself remains part of the trusted computing base. Dun
therefore needs:

-   fuzzing;
-   dependency updates;
-   runtime security monitoring;
-   regression tests;
-   hostile-plugin tests.

------------------------------------------------------------------------

# 15. Concurrency Model

The first implementation should avoid ambiguous shared state.

## 15.1 Decision: per-request instantiation is the v0 default

wazero's own maintainers describe exactly two production patterns for
high-volume request handling, both legitimate:

> "There are two main ways wazero is used for high-volume request
> handling. One way is pooling modules and the other is instantiating
> per-request." — wazero v1.0.0 release notes

wazero added explicit first-class support for the per-request path
("anonymous modules") specifically because instantiating a
pre-compiled module is cheap enough to make pooling optional rather than
required.

**Dun v0 uses per-request instantiation from a pre-compiled
`wazero.CompiledModule`:** compile once at `COMPILE` (§12), instantiate
fresh for each invocation, close after. This is the model for
Milestones 2–4, whose goal is "prove the extension boundary" (§32), not
throughput optimization. It also removes a security question pooling
raises for free — whether a reused instance can leak state between
requests or tenants through its linear memory or globals — by making
every invocation stateless by construction instead of requiring proof
that a pool resets cleanly.

``` text
Compiled Module
       │
       ├── Instance (request 1, closed after)
       ├── Instance (request 2, closed after)
       ├── Instance (request 3, closed after)
       └── Instance (request 4, closed after)
```

The host controls instance allocation. The extension SDK should
discourage mutable global state unless explicitly documented — per-request
instantiation makes any such state useless beyond a single request
anyway.

## 15.2 Pooling is a Milestone 7 research/benchmark item, not an open question

Instance pooling (checkout/return, idle eviction, post-trap recycling,
reset-between-uses correctness) is deferred to Milestone 7 — Benchmark
Lab (§32), where "per-request vs pooled" becomes a concrete row in the
baseline matrix (§28) with real numbers, rather than a design question
left unresolved in RFC-0001.

------------------------------------------------------------------------

# 16. Streaming Strategy

Streaming is one of the most important architectural challenges.

The naive model is:

``` text
HTTP body
   ↓
read all
   ↓
copy into Wasm
   ↓
process
   ↓
copy back
```

This is unacceptable for large payloads, which is why its cost is
bounded by `max_body_size` (§11.1) and scoped to only the plugins that
opt into body access — traffic with no body-reading plugin never enters
this path at all.

The architecture must therefore distinguish:

### Metadata path

Small, frequent:

``` text
method
URI
headers
status
metadata
```

### Body path

Potentially large:

``` text
chunk
chunk
chunk
chunk
```

Version 0 should support controlled buffering for simple plugins, per
§11.1's explicit scoping.

The architecture must leave room for a streaming interface.

Long-term:

``` text
Go stream
    ⇅
Wasm stream abstraction
    ⇅
Component Model stream<T>
```

This is a major research target because WASI 0.3 (released June 11,
2026) now provides native async/stream primitives (`async func`,
`stream<T>`, `future<T>`) at the Component Model level — see §5.2.

------------------------------------------------------------------------

# 17. Memory Model

The host must never expose raw Go pointers.

The initial Core-Wasm ABI should use:

``` text
offset
length
handle
status code
integer identifiers
```

where necessary.

For example:

``` text
host_get_header(
    request_handle,
    key_offset,
    key_length,
    output_buffer
)
```

However, the public SDK should hide this low-level representation.

The extension developer should ideally write:

``` go
value := request.Header("Authorization")
```

rather than manually managing Wasm memory.

------------------------------------------------------------------------

# 18. ABI Design Philosophy

The ABI should be:

-   small;
-   versioned;
-   deterministic;
-   explicit;
-   language-neutral;
-   testable;
-   documented independently of the implementation;
-   capable of evolving without breaking existing plugins.

The ABI should not expose Go implementation details.

Bad:

``` text
go.http.Request
```

Good:

``` text
dun.http.request
```

## 18.1 Decision: ABI versioning via an exported marker function

"Versioned" needed a concrete mechanism. Dun adopts the pattern used by
the most directly relevant prior art, Proxy-Wasm, rather than inventing
one: every guest module exports a zero-argument marker function naming
the ABI version it was built against, and the host checks for that
export immediately after loading the module, refusing to run a module
whose marker is missing or unrecognized.

> Proxy-Wasm ABI (Application Binary Interface) version 0.2.1 is
> currently recommended — Envoy documentation, describing how Envoy
> validates Proxy-Wasm modules before running them.

The Proxy-Wasm spec's own design notes explain the reasoning: a manifest
field can be wrong, stale, or absent, and Wasm toolchains have
historically had inconsistent support for exporting plain global
variables — so a callable marker function is the most portable place to
put a version the host can check unconditionally, before any capability
negotiation happens.

**Concretely:**

-   every Dun plugin binary exports `dun_abi_version_0_1` (or the
    current version string), mirroring `proxy_abi_version_0_2_1`;
-   the extension manager checks for this export at `VALIDATE` (§12),
    before `COMPILE`/`INSTANTIATE` — an unrecognized or missing marker
    is a load-time validation failure, not a runtime trap;
-   the manifest's `api: "dun.extension/v1"` field (§22) remains for
    human- and tooling-readability (`dun plugin inspect`, registry
    search) but is explicitly **not** the enforcement mechanism — the
    exported marker is.

------------------------------------------------------------------------

# 19. Runtime Abstraction

Dun must not make the rest of the project depend directly on wazero.

Conceptually:

``` go
type Runtime interface {
    Load(ctx context.Context, artifact Artifact) (Module, error)
}

type Module interface {
    Invoke(ctx context.Context, call Call) (Result, error)
    Close(ctx context.Context) error
}
```

The exact interfaces must be designed carefully after the first
prototype.

Initial adapter:

``` text
runtime/wazero
```

Future adapter:

``` text
runtime/component
```

This does not mean implementing both now.

------------------------------------------------------------------------

# 20. Proxy-Wasm Compatibility

Proxy-Wasm should be treated as an ecosystem compatibility target, not
as the foundation of Dun's identity — and, per §18.1, as a direct source
of prior art for how Dun solves ABI versioning even in its native
contract.

Possible future architecture:

``` text
                Dun Extension Host
                       │
             ┌─────────┴─────────┐
             │                   │
        Dun Extension        Proxy-Wasm
             API               Adapter
             │                   │
             └─────────┬─────────┘
                       │
                     Runtime
```

Benefits:

-   access to an existing ecosystem;
-   compatibility experiments;
-   comparison against established proxy extension semantics;
-   less pressure to reinvent every concept.

The compatibility layer must remain optional.

------------------------------------------------------------------------

# 21. Component Model Strategy

The WebAssembly Component Model is a long-term target.

The project should maintain a research branch or RFC track rather than
prematurely making it mandatory.

Future architecture:

``` text
Dun Extension Contract
          │
          ▼
        WIT
          │
          ▼
 WebAssembly Component
          │
          ▼
 Component Runtime
```

Potential benefits:

-   typed interfaces;
-   language interoperability;
-   better composition;
-   explicit resource types;
-   future async/streaming primitives.

The main constraint is runtime maturity and Go-native runtime support.

------------------------------------------------------------------------

# 22. Plugin Metadata

A plugin should eventually have machine-readable metadata.

Example:

``` yaml
name: dun-auth-jwt
version: 0.1.0
api: "dun.extension/v1"

description: JWT authentication middleware

runtime:
  type: wasm
  format: core-module

capabilities:
  request:
    - headers.read
  response:
    - status.write
  context:
    - metadata.write

limits:
  memory: 64MiB
  timeout: 5ms
```

Per §18.1, `api:` is **descriptive metadata** — useful for
`dun plugin inspect`, a future registry, and human documentation — not
the mechanism that gates whether a plugin is allowed to load. That gate
is the exported ABI-version marker function, checked at `VALIDATE`.

This metadata becomes the basis for:

-   validation;
-   permissions;
-   compatibility;
-   registry;
-   CLI;
-   signing;
-   documentation.

------------------------------------------------------------------------

# 23. Plugin Supply Chain

A future plugin ecosystem creates a security problem.

Dun should eventually support:

``` text
hash
signature
publisher
version
permissions
compatibility
provenance
```

Potential CLI:

``` bash
dun plugin inspect auth.wasm
dun plugin verify auth.wasm
dun plugin install dun-auth-jwt
```

Example output:

``` text
Plugin: dun-auth-jwt
Version: 1.2.0
Publisher: example

Signature: VALID
Checksum: VALID

Capabilities:
  request.headers.read
  response.status.write

Filesystem: denied
Network: denied
```

This is future scope, not MVP.

------------------------------------------------------------------------

# 24. Configuration Philosophy

Dun must not become a configuration language project.

Configuration should be:

-   readable;
-   concise;
-   explicit;
-   composable;
-   validated.

Potential initial format:

``` yaml
listen:
  address: ":8080"

routes:
  - match:
      path_prefix: /api
    upstream: http://localhost:3000
    plugins:
      - auth
      - rate-limit
```

CLI flags should support simple cases.

The default experience should remain extremely short:

``` bash
dun
```

or:

``` bash
dun run
```

------------------------------------------------------------------------

# 25. Developer Experience

The first successful experience should fit into minutes.

Target:

``` bash
git clone ...
cd dun
go run ./cmd/dun
```

Then:

``` bash
curl http://localhost:8080
```

The repository should include:

``` text
examples/
  hello/
  header/
  auth/
  ratelimit/
```

And plugin examples in at least:

``` text
TinyGo
Rust
```

AssemblyScript may be added after the ABI stabilizes.

------------------------------------------------------------------------

# 26. CLI

Initial commands:

``` text
dun run
dun check
dun plugin inspect
dun plugin validate
dun version
```

Future:

``` text
dun plugin install
dun plugin verify
dun benchmark
dun config
```

The CLI should remain small.

------------------------------------------------------------------------

# 27. Observability

Dun should provide useful observability without becoming a full
observability platform.

Initial:

``` text
structured logs
request counters
request duration
plugin invocation duration
plugin errors
plugin reloads
```

Potential metrics:

``` text
dun_requests_total
dun_request_duration_seconds
dun_upstream_errors_total

dun_plugin_invocations_total
dun_plugin_duration_seconds
dun_plugin_errors_total
dun_plugin_reloads_total
```

------------------------------------------------------------------------

# 28. Performance Research

Performance is a first-class project artifact.

Every meaningful release should be benchmarkable.

## Baseline matrix

``` text
Native Go middleware
Dun without plugins
Dun + one Wasm plugin (per-request instantiation)
Dun + one Wasm plugin (pooled instantiation)
Dun + multiple Wasm plugins
Proxy-Wasm host comparison
Future Component implementation
```

## Metrics

``` text
requests/sec
p50 latency
p95 latency
p99 latency
CPU
RSS memory
allocations/request
bytes allocated/request
plugin startup
module compilation
host calls/request
body copies/request
```

## Workloads

``` text
hello world
header read
header write
authentication
small body
large body
streaming
multiple plugins
plugin failure
hot reload
```

No performance claim should be published without a reproducible
benchmark.

------------------------------------------------------------------------

# 29. Security Research

Security testing must include:

``` text
malformed plugin
infinite loop
memory exhaustion
large allocation
capability violation
invalid handle
invalid memory offset
panic/trap
plugin reload race
concurrent access
malformed request
malformed response
```

The project should eventually include fuzz targets for:

``` text
ABI
metadata parser
plugin loader
request/response bridge
configuration parser
```

------------------------------------------------------------------------

# 30. Testing Strategy

## Unit tests

Each Go package should have focused tests.

## Integration tests

``` text
client → Dun → plugin → upstream
```

## Runtime tests

``` text
load
instantiate
invoke
trap
reload
close
```

## Compatibility tests

A plugin test suite should verify that the same extension behavior
remains valid across runtime versions.

## Fuzzing

Use Go fuzzing for:

-   configuration;
-   headers;
-   metadata;
-   ABI inputs;
-   plugin manifests.

------------------------------------------------------------------------

# 31. Repository Structure

Recommended initial repository:

``` text
dun/
├── cmd/
│   └── dun/
│       └── main.go
│
├── internal/
│   ├── config/
│   ├── http/
│   ├── proxy/
│   ├── router/
│   ├── pipeline/
│   ├── extension/
│   │   ├── host/
│   │   ├── lifecycle/
│   │   ├── capability/
│   │   ├── manifest/
│   │   └── manager/
│   │
│   └── runtime/
│       └── wazero/
│
├── api/
│   └── extension/
│
├── sdk/
│   ├── tinygo/
│   └── rust/
│
├── plugins/
│   └── examples/
│       ├── header/
│       ├── auth/
│       └── ratelimit/
│
├── benchmarks/
│
├── examples/
│
├── docs/
│
├── rfcs/
│   ├── 0000-foundation.md
│   ├── 0001-extension-model.md
│   ├── 0002-capabilities.md
│   ├── 0003-lifecycle.md
│   ├── 0004-streaming.md
│   ├── 0005-hot-reload.md
│   ├── 0006-proxy-wasm.md
│   ├── 0007-component-model.md
│   └── 0008-plugin-signing.md
│
├── testdata/
│
├── go.mod
├── go.sum
├── LICENSE
├── README.md
├── CONTRIBUTING.md
├── SECURITY.md
└── CHANGELOG.md
```

Do not create future directories merely to look sophisticated. Only
create packages when implementation begins.

------------------------------------------------------------------------

# 32. Initial Milestones

## Milestone 0 --- Architecture

Deliverables:

-   foundation document;
-   RFC-0001;
-   threat model;
-   request lifecycle;
-   initial ABI sketch;
-   benchmark plan.

## Milestone 1 --- Native Proxy

Deliver:

``` text
HTTP server
router
reverse proxy
config
CLI
tests
```

No Wasm yet.

Goal:

> Prove the core.

## Milestone 2 --- Wasm Runtime

Deliver:

``` text
wazero integration
module loading
module validation (including ABI marker check, §18.1)
invocation (per-request instantiation, §15.1)
basic lifecycle
```

Goal:

> Prove that Dun can execute extensions.

## Milestone 3 --- Extension API

First plugin:

``` text
security-header.wasm
```

Operations:

``` text
read request
modify response headers
continue
```

Goal:

> Prove the extension boundary.

## Milestone 4 --- Multiple Plugins

Implement:

``` text
header
auth
rate-limit
```

Goal:

> Validate that the API is general.

## Milestone 5 --- Isolation & Limits

Implement:

``` text
capabilities (two-stage enforcement, §10.1)
memory limits
timeouts
failure handling
```

Goal:

> Make the sandbox operationally credible.

## Milestone 6 --- Hot Reload

Implement:

``` text
watch
validate
instantiate
atomic activation
drain
```

Goal:

> Prove production-style lifecycle management. Plugin-instance state is
> not preserved (§13.1) — only host-side state needs to survive.

## Milestone 7 --- Benchmark Lab

Publish:

``` text
native
one wasm (per-request)
one wasm (pooled) — §15.2
three wasm
large body
plugin failures
reload
```

Goal:

> Measure the real overhead, including whether pooling (§15.2) is worth
> its added complexity.

## Milestone 8 --- Ecosystem Research

Investigate:

``` text
Proxy-Wasm
Component Model
WIT
plugin signing
registry
```

Goal:

> Decide the future direction based on evidence.

------------------------------------------------------------------------

# 33. MVP Definition

Dun v0.1 should NOT attempt to be an enterprise gateway.

The MVP is successful if it can:

1.  listen for HTTP traffic;
2.  route requests;
3.  proxy upstream services;
4.  load a Wasm extension;
5.  execute it safely;
6.  expose request/response metadata;
7.  allow a plugin to mutate permitted data;
8.  enforce capabilities;
9.  survive plugin failure;
10. hot-reload a plugin;
11. provide useful logs;
12. run reproducible benchmarks.

------------------------------------------------------------------------

# 34. Explicit Non-Goals

For v0.x:

``` text
No service mesh
No Kubernetes control plane
No distributed control plane
No web dashboard
No cloud platform
No plugin marketplace
No built-in database
No AI features
No automatic service discovery platform
No attempt to replace Envoy
No claim of universal gateway functionality
```

These restrictions protect the project's identity.

------------------------------------------------------------------------

# 35. Long-Term Roadmap

``` text
                     DUN
                      │
          ┌───────────┴───────────┐
          │                       │
        v0.x                    v1.x
          │                       │
   Core-Wasm + wazero        Stable extension API
          │                       │
          ▼                       ▼
   Security + lifecycle     ecosystem + signing
          │                       │
          ▼                       ▼
   Proxy-Wasm research      plugin distribution
          │
          ▼
   Component Model research
          │
          ▼
   WIT + async + streaming
          │
          ▼
      v2.x research
```

------------------------------------------------------------------------

# 36. What Would Make Dun Truly Original?

Dun must not rely on the sentence:

> "We use WebAssembly."

That is not enough.

The project's originality should come from the combination of:

``` text
small Go-native core
        +
explicit programmable pipeline
        +
capability-oriented extensions
        +
runtime abstraction
        +
measured host/guest overhead
        +
first-class lifecycle
        +
future Component Model path
        +
open RFC-driven development
```

The strongest research contribution may eventually be a set of
benchmarks and design findings showing where Wasm is and is not a good
extension boundary for HTTP infrastructure.

------------------------------------------------------------------------

# 37. Portfolio Positioning

Recommended project description:

> **Dun --- Go-native programmable HTTP data plane**
>
> An open-source infrastructure project exploring WebAssembly as a
> sandboxed extension boundary for high-performance HTTP proxying.
>
> Built in Go with a deliberately small trusted core, wazero-based
> WebAssembly execution, capability-oriented extensions, hot reload,
> reproducible benchmarks, and a long-term path toward WebAssembly
> Components.

Technology tags:

``` text
Go
HTTP
Reverse Proxy
WebAssembly
WASI
wazero
WIT
Systems Programming
Networking
Sandboxing
Infrastructure
Performance Engineering
Open Source
```

Before this name is used publicly, note (see §18.1 area / new note
below) that `github.com/lavalagends/dun` already exists as an unrelated,
inactive Go module — not a blocker, but worth 5 minutes of checking
before the GitHub org/repo name is frozen.

------------------------------------------------------------------------

# 38. Project Documentation Strategy

The repository should not rely only on README marketing.

Documentation should include:

``` text
README
ARCHITECTURE
GETTING_STARTED
EXTENSIONS
SECURITY
BENCHMARKS
CONTRIBUTING
RFCs
```

The README explains the project.

The RFCs explain the engineering decisions.

The benchmark reports explain the performance.

The security documentation explains the threat model.

This creates a project that can be reviewed technically by other
engineers.

------------------------------------------------------------------------

# 39. First RFC Set

The first RFC series should be:

### RFC-0001 --- Extension Model

Defines the extension lifecycle, invocation model (per-request
instantiation as default, §15.1), request/response interface, and
failure semantics.

### RFC-0002 --- Capability Model

Defines permission granularity and security boundaries, including the
two-stage enforcement decided in §10.1.

### RFC-0003 --- Plugin Lifecycle

Defines load, compile, instantiate, activation, drain, reload, and
shutdown — including the ABI-marker check at `VALIDATE` (§18.1).

### RFC-0004 --- Streaming

Defines how request and response bodies can move through extensions
without uncontrolled buffering, building on the `max_body_size` scoping
in §11.1.

### RFC-0005 --- Hot Reload

Defines atomic activation and in-flight request behavior, including
the state-non-preservation decision in §13.1.

### RFC-0006 --- Proxy-Wasm Compatibility

Evaluates whether and how Dun can consume existing Proxy-Wasm
extensions.

### RFC-0007 --- Component Model

Evaluates WIT, WASI 0.3, async, streams, and future runtime choices.

### RFC-0008 --- Plugin Signing

Defines artifact identity, checksums, signatures, publisher metadata,
and compatibility.

------------------------------------------------------------------------

# 40. Open Research Questions

These questions should remain explicitly open.

## Runtime

-   What is the real cost of a host/guest call?
-   How much compilation cost matters during reload?
-   Does pooling (§15.2) justify its complexity over per-request
    instantiation (§15.1) at Dun's target workloads?
-   How much memory does each instance require?

## HTTP

-   How should streaming bodies cross the sandbox?
-   Can zero-copy techniques be safely introduced?
-   How should HTTP/2 and HTTP/3 metadata be represented?

## Security

-   What is the minimum useful capability set?
-   How should secrets be exposed?
-   How should network access be restricted?
-   How should plugin signatures be verified?

## Compatibility

-   How much Proxy-Wasm compatibility is useful?
-   Does Dun need a native ABI at all long term?
-   Can a Dun contract map cleanly to WIT?

## Performance

-   When does Wasm become too expensive?
-   Which middleware workloads benefit from Wasm?
-   What is the break-even point versus native Go?

------------------------------------------------------------------------

# 41. Definition of Success

Dun is successful even if it never becomes a massive gateway.

Success means:

### Engineering

-   clean Go architecture;
-   robust HTTP behavior;
-   strong tests;
-   measurable performance;
-   clear security model.

### Ecosystem

-   external developers can write plugins;
-   documentation is sufficient to contribute;
-   extension API is stable enough to build against;
-   the project has useful examples.

### Research

-   benchmarks answer real questions;
-   architectural decisions are documented;
-   failures and limitations are public;
-   Dun contributes useful knowledge to the Go/Wasm ecosystem.

### Portfolio

The project demonstrates real competence in:

``` text
Go
network programming
HTTP
systems architecture
WebAssembly
sandboxing
security
performance engineering
open-source engineering
technical writing
```

------------------------------------------------------------------------

# 42. Immediate Build Order

Do not start with the entire architecture.

Start with these exact steps:

``` text
STEP 1
Create repository

STEP 2
Implement minimal HTTP reverse proxy

STEP 3
Add route abstraction

STEP 4
Create pipeline abstraction

STEP 5
Integrate wazero

STEP 6
Load one Wasm module

STEP 7
Define the smallest possible host API

STEP 8
Implement header mutation plugin

STEP 9
Add capability validation

STEP 10
Add plugin lifecycle

STEP 11
Add failure isolation

STEP 12
Add hot reload

STEP 13
Build benchmark suite

STEP 14
Write RFCs from what the implementation actually taught us

STEP 15
Only then expand the ABI
```

This order is deliberate.

**We should not design 100 host functions before proving that 10 are
necessary.**

------------------------------------------------------------------------

# 43. Final Architectural Position

Dun should be built as:

``` text
                 SMALL CORE
                     │
              GO HTTP ENGINE
                     │
              EXTENSION HOST
                     │
             CAPABILITY MODEL
                     │
             WASM RUNTIME API
                     │
                  wazero
                     │
             Core-Wasm v0
                     │
       ┌─────────────┼─────────────┐
       │             │             │
      Auth        Policy       Transform
       │             │             │
       └─────────────┼─────────────┘
                     │
                 UPSTREAM
```

And eventually:

``` text
                    DUN
                     │
             Extension Contract
                     │
          ┌──────────┴──────────┐
          │                     │
       Core-Wasm          Component Model
          │                     │
        wazero             future runtime
```

The key architectural commitment is:

> **Dun's core should remain small even if the ecosystem around it
> becomes large.**

That is the project's identity.

------------------------------------------------------------------------

# 44. Current Decision Log

  Decision                                    Status
  -------------------------------------------- -----------------------
  Go-native core                               Accepted
  Pure Go / no CGO target                      Accepted
  wazero for initial Wasm runtime               Accepted
  Single self-contained binary                  Accepted
  Wasm extensions                               Accepted
  Capability model                              Accepted
  Two-stage capability enforcement (§10.1)      Accepted
  Explicit plugin lifecycle                     Accepted
  Per-request instantiation as v0 default (§15) Accepted
  Instance pooling                              Deferred to Milestone 7
  ABI versioning via exported marker (§18.1)    Accepted
  `max_body_size` scoped to plugin path (§11.1) Accepted
  Plugin state not preserved across reload      Accepted
  Hot reload                                    Accepted
  Benchmark-driven performance claims           Accepted
  Proxy-Wasm compatibility                      Research
  Component Model                               Research / future
  WIT                                           Future research
  Plugin registry                               Future
  Plugin signing                                Future
  Kubernetes/control plane                      Out of scope for v0.x
  Dashboard                                     Out of scope for v0.x

------------------------------------------------------------------------

# 45. Current Status

**Dun is now ready to move from concept to implementation.**

v0.1's open design questions (concurrency model, body-size scope,
hot-reload state, ABI versioning mechanism, capability-violation timing)
are resolved in this revision (§10.1, §11.1, §13.1, §15.1, §18.1) and no
longer need to be re-litigated in RFC-0001 — RFC-0001 should treat them
as inherited defaults and focus on freezing the concrete extension
contract around them.

The next artifact should be:

``` text
rfcs/0001-extension-model.md
```

That RFC must freeze the first concrete extension contract before SDKs
or a plugin ecosystem are built.

The implementation should then begin with the smallest vertical slice:

``` text
HTTP request
   ↓
Dun
   ↓
one Wasm extension
   ↓
header mutation
   ↓
upstream
   ↓
response
```

If that vertical slice is clean, measurable, secure, and understandable,
Dun has a foundation worth expanding.

------------------------------------------------------------------------

# References

1.  Go Developer Survey 2025 --- Go Blog.
2.  wazero documentation, repository, and v1.0.0 release notes
    (tetratelabs/wazero) --- source for the per-request vs. pooled
    instantiation distinction (§15.1).
3.  WebAssembly Component Model specification and documentation.
4.  WASI roadmap and WASI 0.3.0 release notes (WebAssembly/WASI,
    released June 11, 2026) --- source for the async/stream primitives
    confirmed in §5.2 and §16.
5.  Proxy-Wasm specification and ecosystem --- source for the ABI
    marker-export versioning pattern adopted in §18.1.
6.  Envoy WebAssembly documentation --- describes Envoy's Proxy-Wasm
    ABI validation behavior, cited in §18.1.
7.  Traefik WebAssembly plugin documentation.

Official references should be consulted continuously as the project
evolves because WebAssembly runtime and Component Model support are
changing rapidly.
