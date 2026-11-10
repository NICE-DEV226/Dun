// Package benchmarks is the Dun Benchmark Lab (Milestone 7, RFC-0000 §28).
//
// It measures the real end-to-end HTTP overhead of the data plane across
// the baseline matrix: native reverse proxy, Dun without plugins, one Wasm
// plugin per-request vs pooled (§15.1/§15.2), multiple plugins, large
// bodies, plugin failures, reload and module compilation.
//
// Run with:
//
//	go test ./benchmarks -bench . -benchmem -benchtime=2s
package benchmarks

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/NICE-DEV226/Dun/internal/config"
	"github.com/NICE-DEV226/Dun/internal/extension"
	"github.com/NICE-DEV226/Dun/internal/guest"
	dunhttp "github.com/NICE-DEV226/Dun/internal/http"
	"github.com/NICE-DEV226/Dun/internal/runtime/wazero"
)

// pluginSpec describes one plugin artifact to load through the real
// config → manager → pipeline path.
type pluginSpec struct {
	name      string
	wasm      []byte
	caps      []string
	instances int // >0 enables pooled instantiation (§15.2)
}

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

// backend returns an upstream that drains the request body and answers
// with a small fixed response, like a typical API service.
func backend(t testing.TB) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("X-Upstream", "true")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("upstream ok"))
	}))
}

// dunServer builds a Dun data plane over the given upstream and plugins,
// wired exactly like `dun run` does (config → LoadPlugins → pipelines).
func dunServer(t testing.TB, upstream string, plugins []pluginSpec, routePluginNames []string) *httptest.Server {
	t.Helper()

	dir := t.TempDir()
	cfg := &config.Config{
		Listen: config.ListenConfig{Address: ":0"},
		Routes: []config.RouteConfig{{
			Name:     "api",
			Match:    config.MatchConfig{PathPrefix: "/api"},
			Upstream: upstream,
			Plugins:  routePluginNames,
		}},
		Log: config.LogConfig{Level: "error"},
	}

	for _, p := range plugins {
		path := filepath.Join(dir, p.name+".wasm")
		if err := os.WriteFile(path, p.wasm, 0o644); err != nil {
			t.Fatal(err)
		}
		cfg.Plugins = append(cfg.Plugins, config.PluginConfig{
			Name:         p.name,
			Path:         path,
			Version:      "0.1.0",
			API:          "dun.extension/v1",
			Capabilities: p.caps,
			Limits:       config.PluginLimits{Instances: p.instances},
		})
	}

	s, err := dunhttp.New(cfg, dunhttp.WithLogger(discardLogger))
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.Plugins) > 0 {
		rt, err := wazero.NewRuntime(discardLogger)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = rt.Close(context.Background()) })

		mgr := extension.NewManager(rt, discardLogger)
		t.Cleanup(func() { _ = mgr.CloseAll(context.Background()) })
		for _, p := range cfg.Plugins {
			caps := extension.NewSet()
			for _, c := range p.Capabilities {
				caps.Add(extension.Capability(c))
			}
			mgr.SetPolicy(p.Name, caps)
		}
		if err := s.LoadPlugins(context.Background(), mgr); err != nil {
			t.Fatal(err)
		}
	}

	return httptest.NewServer(s)
}

// nativeProxy is the "Native Go middleware" baseline: a plain
// httputil reverse proxy plus the same response header the security
// plugin writes — no Dun core, no Wasm.
func nativeProxy(t testing.TB, upstream string) *httptest.Server {
	t.Helper()
	u, err := url.Parse(upstream)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Security-Policy", "default-src 'self'")
		proxy.ServeHTTP(w, r)
	}))
}

var httpClient = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:        256,
		MaxIdleConnsPerHost: 256,
	},
}

// bench drives n sequential GET requests against srv and reports latency
// percentiles as custom metrics.
func bench(b *testing.B, srv *httptest.Server, path string, body []byte) {
	b.Helper()
	lat := make([]time.Duration, 0, 1024)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		var req *http.Request
		var err error
		if body != nil {
			req, err = http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(body))
		} else {
			req, err = http.NewRequest(http.MethodGet, srv.URL+path, nil)
		}
		if err != nil {
			b.Fatal(err)
		}
		start := time.Now()
		resp, err := httpClient.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		lat = append(lat, time.Since(start))
	}
	b.StopTimer()

	if len(lat) > 1 {
		sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	}
	p := func(q float64) float64 {
		return float64(lat[int(float64(len(lat)-1)*q)])
	}
	b.ReportMetric(p(0.50), "p50-ns")
	b.ReportMetric(p(0.95), "p95-ns")
	b.ReportMetric(p(0.99), "p99-ns")
}

var headerCaps = []string{"response.headers.write"}

func BenchmarkNativeReverseProxy(b *testing.B) {
	up := backend(b)
	defer up.Close()
	srv := nativeProxy(b, up.URL)
	defer srv.Close()
	bench(b, srv, "/api/hello", nil)
}

func BenchmarkDunNoPlugin(b *testing.B) {
	up := backend(b)
	defer up.Close()
	srv := dunServer(b, up.URL, nil, nil)
	defer srv.Close()
	bench(b, srv, "/api/hello", nil)
}

func BenchmarkDunOneWasmPerRequest(b *testing.B) {
	up := backend(b)
	defer up.Close()
	plugins := []pluginSpec{{
		name: "security-header",
		wasm: guest.SecurityHeaderModule(),
		caps: headerCaps,
	}}
	srv := dunServer(b, up.URL, plugins, []string{"security-header"})
	defer srv.Close()
	bench(b, srv, "/api/hello", nil)
}

func BenchmarkDunOneWasmPooled(b *testing.B) {
	up := backend(b)
	defer up.Close()
	plugins := []pluginSpec{{
		name:      "security-header",
		wasm:      guest.SecurityHeaderModule(),
		caps:      headerCaps,
		instances: 8,
	}}
	srv := dunServer(b, up.URL, plugins, []string{"security-header"})
	defer srv.Close()
	bench(b, srv, "/api/hello", nil)
}

// BenchmarkDunThreeWasm runs three independent plugin instances per
// request (three compiled modules, three per-request instantiations).
func BenchmarkDunThreeWasm(b *testing.B) {
	up := backend(b)
	defer up.Close()
	plugins := []pluginSpec{
		{name: "sec-a", wasm: guest.SecurityHeaderModuleValue("X-Security-Policy", "default-src 'self'"), caps: headerCaps},
		{name: "sec-b", wasm: guest.SecurityHeaderModuleValue("X-Frame-Options", "DENY"), caps: headerCaps},
		{name: "sec-c", wasm: guest.SecurityHeaderModuleValue("X-Content-Type-Options", "nosniff"), caps: headerCaps},
	}
	names := []string{"sec-a", "sec-b", "sec-c"}
	srv := dunServer(b, up.URL, plugins, names)
	defer srv.Close()
	bench(b, srv, "/api/hello", nil)
}

// BenchmarkDunLargeBody pushes a 1 MiB request body through the proxy
// with one plugin on the path.
func BenchmarkDunLargeBody(b *testing.B) {
	up := backend(b)
	defer up.Close()
	plugins := []pluginSpec{{
		name: "security-header",
		wasm: guest.SecurityHeaderModule(),
		caps: headerCaps,
	}}
	srv := dunServer(b, up.URL, plugins, []string{"security-header"})
	defer srv.Close()
	bench(b, srv, "/api/upload", make([]byte, 1<<20))
}

// BenchmarkDunPluginFailure measures the failure path: every request
// traps inside the guest and returns 500 (RFC-0000 §14 telemetry path).
func BenchmarkDunPluginFailure(b *testing.B) {
	up := backend(b)
	defer up.Close()
	plugins := []pluginSpec{{
		name: "trapper",
		wasm: guest.TrapModule(),
	}}
	srv := dunServer(b, up.URL, plugins, []string{"trapper"})
	defer srv.Close()
	bench(b, srv, "/api/hello", nil)
}

// BenchmarkModuleCompile measures plugin startup: ABI-check runtime +
// compile of one artifact (the COMPILE stage cost).
func BenchmarkModuleCompile(b *testing.B) {
	rt, err := wazero.NewRuntime(discardLogger)
	if err != nil {
		b.Fatal(err)
	}
	defer rt.Close(context.Background())

	artifact := guest.SecurityHeaderModule()
	opts := extension.LoadOptions{
		Capabilities: extension.NewSet(string(extension.CapResponseHeadersWrite)),
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		mod, err := rt.Load(context.Background(), artifact, opts)
		if err != nil {
			b.Fatal(err)
		}
		if err := mod.Close(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPluginReload measures the atomic hot-reload path (§13):
// validate + compile the new artifact + swap + drain, per operation.
func BenchmarkPluginReload(b *testing.B) {
	rt, err := wazero.NewRuntime(discardLogger)
	if err != nil {
		b.Fatal(err)
	}
	defer rt.Close(context.Background())

	mgr := extension.NewManager(rt, discardLogger)

	name := "security-header"
	caps := extension.NewSet(string(extension.CapResponseHeadersWrite))
	mgr.SetPolicy(name, caps)

	manifest := &extension.Manifest{
		Name:    name,
		Version: "0.1.0",
		API:     "dun.extension/v1",
		Caps:    extension.Caps{Response: []string{"headers.write"}},
	}
	v1 := guest.SecurityHeaderModuleValue("X-Security-Policy", "default-src 'self'")
	v2 := guest.SecurityHeaderModuleValue("X-Security-Policy", "default-src 'none'")

	if err := mgr.Load(name, manifest, v1); err != nil {
		b.Fatal(err)
	}
	if err := mgr.Activate(name); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		artifact := v1
		if i%2 == 0 {
			artifact = v2
		}
		if err := mgr.Reload(name, artifact, manifest); err != nil {
			b.Fatal(err)
		}
	}
}
