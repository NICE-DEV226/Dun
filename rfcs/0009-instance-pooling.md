# RFC-0009 — Instance Pooling

**Status:** Accepted (Milestone 7 outcome)
**Date:** 2026-11-10
**Amends:** RFC-0000 §15.2, RFC-0001 open items

## Abstract

This RFC records the Milestone 7 (Benchmark Lab) decision on instance
pooling: whether Dun should offer a pooled instantiation model
(checkout/return, idle eviction, reset-between-uses) alongside the v0
default of per-request instantiation (§15.1), and under which
constraints.

## Context

§15.1 fixed per-request instantiation as the v0 default and deferred
pooling to Milestone 7, where "per-request vs pooled" would become a
concrete row in the baseline matrix with real numbers. The questions
pooling raises:

1. Does pooling justify its complexity over per-request instantiation
   at Dun's target workloads? (§40 open research question)
2. Can a reused instance be reset correctly — no state leak through
   linear memory, globals or tables between requests or tenants?

## Decision

**Pooling is supported, opt-in, and restricted to pool-safe artifacts.**

- `limits.instances: N` (N > 0) in plugin config enables a bounded pool
  of N recycled instances per plugin. `0` (default) keeps per-request
  instantiation.
- Between uses, linear memory is restored from a snapshot captured
  post-instantiation (data segments applied, before any guest code).
  Instances whose memory grew or that were closed by an interrupted
  call are discarded, not reused.
- Instances are instantiated under a non-cancelable context so they
  outlive the creating request; request-scoped cancellation still
  interrupts `fn.Call` via the invoke context.
- **Pool-safety gate:** artifacts declaring table, global or element
  sections, or non-function imports, are never pooled — the public
  wazero API cannot enumerate or reset that state. Such plugins
  silently keep the per-request model.

## Evidence

From `docs/BENCHMARKS.md` (Go 1.27, wazero v1.12.0, i7-8565U):

| Level | Per-request | Pooled (max=4–8) |
|---|---:|---:|
| `module.Invoke` | ~102–125 µs, 48 allocs, 85.7 KB | ~25 µs, 21 allocs, 12.6 KB |
| E2E allocations/request | 221 allocs, 133 KB | 193 allocs, 60 KB |

Pooling is ~4–5× faster and allocates ~7× less per invocation. End-to-
end latency deltas were within this benchmark machine's noise floor;
allocation reduction was consistent across runs.

## Consequences

- Operators running many plugins per route (the 3-plugin E2E case
  showed ~3× instantiation cost unpooled) have a measured escape hatch.
- The security posture of §15.1 is preserved by default: pooling is
  never implicit, and unpoolable artifacts cannot accidentally enter
  the pool.
- Documented residual risk: a pool-safe guest can still keep state in
  ways the ABI does not forbid today (e.g. host-call side effects are
  per-request anyway). The ABI v0 contract already discourages
  cross-request state; pooling makes the memory/global/table subset of
  that guarantee structural.
- `instances` is no longer a parsed-but-inert limit.

## Alternatives considered

- **Pool everything with best-effort reset:** rejected — silent state
  leaks through globals/tables are undetectable via the public API.
- **Snapshot-and-restore all state:** not possible; wazero exposes no
  global/table enumeration on instantiated modules.
- **Keep per-request only:** rejected — the measured 4–5× invoke-level
  win is too large to leave unavailable to operators who accept the
  pool-safety constraints.

## References

- RFC-0000 §15 (Concurrency Model), §28 (Performance Research), §40
- RFC-0001 Milestone 5/6 notes
- `internal/runtime/wazero/pool.go`, `docs/BENCHMARKS.md`
- wazero v1.0.0 release notes: pooling vs per-request as the two
  sanctioned high-volume patterns
