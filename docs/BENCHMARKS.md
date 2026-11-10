# Dun Benchmarks — Milestone 7 Baseline

This document publishes the first Benchmark Lab results (RFC-0000 §28,
§32 Milestone 7). All numbers were produced by the harness in
[`benchmarks/`](../benchmarks) and the runtime-level benches in
`internal/runtime/wazero`.

## Environment

- CPU: Intel Core i7-8565U @ 1.80 GHz (4C/8T, laptop — noisy neighbors
  visible in run-to-run variance)
- OS: Linux (Arch), Go 1.27, wazero v1.12.0
- Workload: sequential HTTP/1.1 keep-alive GET `/api/hello` against a
  loopback `httptest` upstream, unless noted otherwise.

Reproduce:

```bash
go test ./benchmarks -bench . -benchmem -benchtime=2s
go test ./benchmarks -bench 'Pipeline' -benchmem -benchtime=2s
go test ./internal/runtime/wazero -bench BenchmarkInvoke -benchmem -benchtime=3s
```

## Runtime-level results (module.Invoke)

This is the cleanest signal for the §15.1 vs §15.2 question because it
isolates instantiation+call+close from HTTP/proxy noise.

| Model | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| Per-request instantiation (§15.1) | ~102,000–125,000 | 85,695 | 48 |
| Pooled instantiation (§15.2, max=4) | ~24,700–25,900 | 12,649 | 21 |

**Pooling is ~4–5× faster and allocates ~7× less per invocation.** The
reset cost is one 64 KiB memory-snapshot write — cheap compared to
wazero instantiation. This held across `-count=2` runs with low variance.

## End-to-end results (full HTTP path)

Representative run (`-benchtime=1s`–`2s`); absolute latencies vary
±30–50% between runs on this machine, so read them as orders of
magnitude. Allocations are stable across runs.

| Scenario | ns/op (approx.) | B/op | allocs/op |
|---|---:|---:|---:|
| Native Go middleware + reverse proxy | ~470,000–870,000 | 46,400 | 157 |
| Dun, no plugins | ~490,000–740,000 | 46,600 | 166 |
| Dun + 1 Wasm, per-request | ~920,000–1,100,000 | 133,400 | 221 |
| Dun + 1 Wasm, pooled (max=8) | ~970,000–1,020,000 | 59,600 | 193 |
| Dun + 3 Wasm plugins | ~980,000–1,430,000 | 306,700 | 327 |
| Dun + 1 Wasm, 1 MiB request body | ~2,290,000–2,780,000 | ~217,000 | ~253 |
| Dun + trapping plugin (500 path) | ~650,000–695,000 | 93,400 | 140 |
| Module compile+load (plugin startup) | ~1,197,000 | 353,400 | 461 |
| Plugin hot reload (validate+compile+swap) | ~2,510,000 | 796,100 | 995 |

### Concurrent workload (`-bench Parallel`, `b.RunParallel`)

Parallel rows measure throughput under `GOMAXPROCS` concurrent clients;
ns/op here is per-request cost under contention, not sequential latency.

| Scenario | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| Dun + 1 Wasm, per-request | ~215,000 | 144,200 | 249 |
| Dun + 1 Wasm, pooled (max=8) | ~153,000 | 68,700 | 220 |

Under concurrency the pooling win is visible end-to-end: **~29% lower
per-request cost and ~52% fewer allocated bytes** than per-request
instantiation.

## Reading the results

- **Dun core overhead vs native proxy is small** relative to total
  request cost; the dominant cost is `httputil.ReverseProxy` + HTTP
  loopback, common to both.
- **One Wasm plugin per-request adds ~300–400 µs and ~87 KB** over the
  no-plugin path end-to-end, consistent with the ~100 µs invoke-level
  cost plus pipeline/manager/GC pressure from the extra allocations.
- **Pooling removes ~28 allocs and ~74 KB per request** end-to-end
  (221→193 allocs, 133 KB→60 KB). The latency saving (~80–100 µs/op at
  invoke level) is partially masked E2E by proxy cost and by this
  machine's noise floor; on quieter hardware it should be visible in
  p50/p95.
- **Three plugins cost ~3× the per-request instantiation work** of one
  plugin when unpooled — the strongest argument for pooling once
  pipelines grow.
- **Plugin failure path is cheap**: a trapping guest still returns 500
  in roughly no-plugin time; telemetry recording adds negligible cost.
- **Reload costs ~2.5 ms** per operation (full recompile) — fine for
  file-watch hot reload, irrelevant to request latency since activation
  is atomic.

## Pipeline-level results (extension boundary only)

These benchmarks run the *same* `pipeline.Pipeline` machinery with a
single extension that performs the same work (set one response header).
They isolate the extension dispatch cost from routing, proxying and
HTTP — the closest pure-Go analogue to a Proxy-Wasm host comparison.
This is not a Proxy-Wasm benchmark (no Go Proxy-Wasm host exists); it
measures the "Wasm tax" of Dun's own extension boundary.

| Extension implementation | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| Native Go extension (no Wasm) | ~660–680 | 176 | 6 |
| Wasm guest, per-request (§15.1) | ~96,000–124,000 | 85,535 | 52 |
| Wasm guest, pooled §15.2 (max=4) | ~30,500–36,200 | 12,482 | 25 |

Reading:

- The Wasm boundary costs **~150× per-request** and **~50× pooled** vs a
  native Go extension for trivial work — instantiation, ABI marshalling
  and host-call overhead dominate at this scale.
- Pooling recovers ~3–4× of that gap, but a native extension remains
  orders of magnitude cheaper. This is the expected trade-off: Wasm
  buys isolation and language freedom, not speed.
- At E2E level this tax is diluted by proxy/network cost; it matters
  most for high-RPS, low-latency paths with many plugins.

## §15.2 decision input: is pooling worth it?

Evidence says **yes for throughput-sensitive deployments, no as a
universal default**:

- Pooling is a clear win at the invoke level (4–5×) and reduces GC
  pressure measurably E2E.
- But it is only correct for pool-safe artifacts: modules declaring
  tables, globals or element segments are excluded (the public wazero
  API cannot snapshot/reset them), and memory-`grow`ing or
  context-closed instances are discarded rather than reused.
- Per-request instantiation remains the safest default (§15.1); pooling
  is opt-in via `limits.instances: N` in plugin config.

Recommended follow-ups: re-run on quiet CI hardware for publishable
p50/p95/p99; add a concurrent (`RunParallel`) workload row; measure
RSS under sustained pooled load.

## Known limitations of this baseline

- Sequential requests only for the latency table; a concurrent
  (`RunParallel`) pair of rows covers the pooled-vs-per-request
  comparison under contention.
- Single small guest (one header write); no body-reading or streaming
  plugins exist yet (RFC-0004).
- No Proxy-Wasm host comparison — the pipeline-level rows above compare
  native Go vs Wasm *within Dun*, which is the meaningful comparison
  for this project; an actual Proxy-Wasm host (Envoy) cannot run in
  this harness.
- Latency percentiles are computed over the benchmark loop and printed
  as `p50-ns`/`p95-ns`/`p99-ns` custom metrics; treat sub-run variance
  as noise.
