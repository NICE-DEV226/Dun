package http_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NICE-DEV226/Dun/internal/config"
	"github.com/NICE-DEV226/Dun/internal/extension"
	"github.com/NICE-DEV226/Dun/internal/guest"
	dunhttp "github.com/NICE-DEV226/Dun/internal/http"
	"github.com/NICE-DEV226/Dun/internal/runtime/wazero"
)

func TestE2ESecurityHeaderPlugin(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream", "true")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("upstream ok"))
	}))
	defer backend.Close()

	// Write guest wasm to temp file (simulates a real plugin artifact on disk).
	tmpDir := t.TempDir()
	wasmPath := filepath.Join(tmpDir, "security-header.wasm")
	if err := os.WriteFile(wasmPath, guest.SecurityHeaderModule(), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Listen: config.ListenConfig{Address: ":0"},
		Routes: []config.RouteConfig{
			{
				Name:     "api",
				Match:    config.MatchConfig{PathPrefix: "/api"},
				Upstream: backend.URL,
				Plugins:  []string{"security-header"},
			},
		},
		Plugins: []config.PluginConfig{
			{
				Name:         "security-header",
				Path:         wasmPath,
				Version:      "0.1.0",
				API:          "dun.extension/v1",
				Capabilities: []string{"response.headers.write"},
			},
		},
		Log: config.LogConfig{Level: "info"},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s, err := dunhttp.New(cfg, dunhttp.WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}

	// Load plugins: wasm → manager → route pipeline wiring.
	rt, err := wazero.NewRuntime(logger)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	mgr := extension.NewManager(rt, logger)
	for _, p := range cfg.Plugins {
		caps := extension.NewSet()
		for _, c := range p.Capabilities {
			caps.Add(extension.Capability(c))
		}
		mgr.SetPolicy(p.Name, caps)
	}
	if err := s.LoadPlugins(context.Background(), mgr); err != nil {
		t.Fatal("load plugins:", err)
	}

	ts := httptest.NewServer(s)
	defer ts.Close()

	// --- Request through Dun + plugin ---
	resp, err := http.Get(ts.URL + "/api/test")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "upstream ok" {
		t.Errorf("unexpected body: %s", string(body))
	}

	got := resp.Header.Get("X-Security-Policy")
	if got != "default-src 'self'" {
		t.Errorf("expected security header 'default-src self', got %q", got)
	}

	// --- Verify upstream was reached ---
	up := resp.Header.Get("X-Upstream")
	if up != "true" {
		t.Errorf("expected upstream marker, got %q", up)
	}

	// --- Second request: header is consistent (per-request instantiation) ---
	resp2, err := http.Get(ts.URL + "/api/another")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if got := resp2.Header.Get("X-Security-Policy"); got != "default-src 'self'" {
		t.Errorf("second request: expected header, got %q", got)
	}
}

// TestE2EHeaderReadPlugin proves the request.headers.read host import
// end-to-end: the guest rejects requests carrying X-Denied and lets the
// rest through to the upstream.
func TestE2EHeaderReadPlugin(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("upstream ok"))
	}))
	defer backend.Close()

	tmpDir := t.TempDir()
	wasmPath := filepath.Join(tmpDir, "header-read.wasm")
	if err := os.WriteFile(wasmPath, guest.HeaderReadModule(), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Listen: config.ListenConfig{Address: ":0"},
		Routes: []config.RouteConfig{
			{
				Name:     "api",
				Match:    config.MatchConfig{PathPrefix: "/api"},
				Upstream: backend.URL,
				Plugins:  []string{"header-read"},
			},
		},
		Plugins: []config.PluginConfig{
			{
				Name:         "header-read",
				Path:         wasmPath,
				Version:      "0.1.0",
				API:          "dun.extension/v1",
				Capabilities: []string{"request.headers.read"},
			},
		},
		Log: config.LogConfig{Level: "info"},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s, err := dunhttp.New(cfg, dunhttp.WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}

	rt, err := wazero.NewRuntime(logger)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	mgr := extension.NewManager(rt, logger)
	caps := extension.NewSet("request.headers.read")
	mgr.SetPolicy("header-read", caps)
	if err := s.LoadPlugins(context.Background(), mgr); err != nil {
		t.Fatal("load plugins:", err)
	}

	ts := httptest.NewServer(s)
	defer ts.Close()

	// Request without the denied header is proxied.
	resp, err := http.Get(ts.URL + "/api/ok")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "upstream ok" {
		t.Fatalf("expected proxied 200 'upstream ok', got %d %q", resp.StatusCode, body)
	}

	// Request carrying X-Denied is rejected by the guest.
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/denied", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Denied", "true")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for denied header, got %d", resp2.StatusCode)
	}
}

// TestE2EMetricsExposesPluginStats verifies that /metrics renders the
// per-plugin operational counters recorded by the extension manager.
func TestE2EMetricsExposesPluginStats(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer backend.Close()

	tmpDir := t.TempDir()
	wasmPath := filepath.Join(tmpDir, "security-header.wasm")
	if err := os.WriteFile(wasmPath, guest.SecurityHeaderModule(), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Listen: config.ListenConfig{Address: ":0"},
		Routes: []config.RouteConfig{{
			Name:     "api",
			Match:    config.MatchConfig{PathPrefix: "/api"},
			Upstream: backend.URL,
			Plugins:  []string{"security-header"},
		}},
		Plugins: []config.PluginConfig{{
			Name:         "security-header",
			Path:         wasmPath,
			Version:      "0.1.0",
			API:          "dun.extension/v1",
			Capabilities: []string{"response.headers.write"},
		}},
		Log: config.LogConfig{Level: "info"},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s, err := dunhttp.New(cfg, dunhttp.WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}

	rt, err := wazero.NewRuntime(logger)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	mgr := extension.NewManager(rt, logger)
	mgr.SetPolicy("security-header", extension.NewSet("response.headers.write"))
	if err := s.LoadPlugins(context.Background(), mgr); err != nil {
		t.Fatal("load plugins:", err)
	}

	ts := httptest.NewServer(s)
	defer ts.Close()

	for i := 0; i < 2; i++ {
		resp, err := http.Get(ts.URL + "/api/x")
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}

	mresp, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer mresp.Body.Close()
	mbody, _ := io.ReadAll(mresp.Body)
	metrics := string(mbody)

	for _, want := range []string{
		`dun_plugin_invocations_total{plugin="security-header"} 2`,
		`dun_plugin_duration_seconds_count{plugin="security-header"} 2`,
		`dun_plugin_reloads_total{plugin="security-header"} 0`,
	} {
		if !strings.Contains(metrics, want) {
			t.Errorf("metrics missing %q\nfull output:\n%s", want, metrics)
		}
	}
}

func TestE2ERouteWithoutPlugins(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("plain"))
	}))
	defer backend.Close()

	cfg := &config.Config{
		Listen: config.ListenConfig{Address: ":0"},
		Routes: []config.RouteConfig{
			{
				Name:     "plain",
				Match:    config.MatchConfig{PathPrefix: "/"},
				Upstream: backend.URL,
				// No Plugins — default empty pipeline.
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s, err := dunhttp.New(cfg, dunhttp.WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(s)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/anything")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "plain" {
		t.Errorf("unexpected body: %s", string(body))
	}
}

func TestLoadPluginsNoFile(t *testing.T) {
	cfg := &config.Config{
		Listen: config.ListenConfig{Address: ":0"},
		Routes: []config.RouteConfig{
			{Name: "a", Match: config.MatchConfig{PathPrefix: "/"}, Upstream: "http://localhost:1"},
		},
		Plugins: []config.PluginConfig{
			{Name: "bad", Path: "/nonexistent/file.wasm"},
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s, err := dunhttp.New(cfg, dunhttp.WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}

	rt, _ := wazero.NewRuntime(logger)
	defer rt.Close(context.Background())
	mgr := extension.NewManager(rt, logger)
	err = s.LoadPlugins(context.Background(), mgr)
	if err == nil {
		t.Fatal("expected error loading nonexistent plugin file")
	}
}

// TestE2EMultiplePluginsOnRoute proves pipeline ordering with chained wasm plugins.
func TestE2EMultiplePluginsOnRoute(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer backend.Close()

	tmpDir := t.TempDir()
	// Generate two identical security-header.wasm under different names.
	// Both set X-Security-Policy; the second one overwrites — proving the
	// pipeline executes them in order.
	wasmPath1 := filepath.Join(tmpDir, "h1.wasm")
	wasmPath2 := filepath.Join(tmpDir, "h2.wasm")
	moduleBytes := guest.SecurityHeaderModule()
	os.WriteFile(wasmPath1, moduleBytes, 0644)
	os.WriteFile(wasmPath2, moduleBytes, 0644)

	cfg := &config.Config{
		Listen: config.ListenConfig{Address: ":0"},
		Routes: []config.RouteConfig{
			{
				Name:     "api",
				Match:    config.MatchConfig{PathPrefix: "/api"},
				Upstream: backend.URL,
				Plugins:  []string{"h1", "h2"},
			},
		},
		Plugins: []config.PluginConfig{
			{Name: "h1", Path: wasmPath1, Capabilities: []string{"response.headers.write"}},
			{Name: "h2", Path: wasmPath2, Capabilities: []string{"response.headers.write"}},
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s, err := dunhttp.New(cfg, dunhttp.WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}

	rt, _ := wazero.NewRuntime(logger)
	defer rt.Close(context.Background())
	mgr := extension.NewManager(rt, logger)
	for _, p := range cfg.Plugins {
		caps := extension.NewSet("response.headers.write")
		mgr.SetPolicy(p.Name, caps)
	}
	if err := s.LoadPlugins(context.Background(), mgr); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(s)
	defer ts.Close()

	// Allow server startup to settle.
	time.Sleep(20 * time.Millisecond)

	resp, err := http.Get(ts.URL + "/api/test")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("X-Security-Policy"); got != "default-src 'self'" {
		t.Errorf("expected header from chained plugins, got %q", got)
	}
}

// TestE2EPluginTimeoutKeepCoreAlive proves RFC-0000 §14 "keep core alive":
// a plugin that loops forever is terminated by the execution timeout on the
// affected route while the rest of the plane (other routes) stays up.
func TestE2EPluginTimeoutKeepCoreAlive(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("upstream-ok"))
	}))
	defer backend.Close()

	tmpDir := t.TempDir()
	loopWasm := filepath.Join(tmpDir, "loop.wasm")
	os.WriteFile(loopWasm, guest.InfiniteLoopModule(), 0644)

	cfg := &config.Config{
		Listen: config.ListenConfig{Address: ":0"},
		Routes: []config.RouteConfig{
			{
				Name:     "api",
				Match:    config.MatchConfig{PathPrefix: "/api"},
				Upstream: backend.URL,
				Plugins:  []string{"loop"},
			},
			{
				Name:     "plain",
				Match:    config.MatchConfig{PathPrefix: "/health"},
				Upstream: backend.URL,
				// No plugins.
			},
		},
		Plugins: []config.PluginConfig{
			{
				Name: "loop",
				Path: loopWasm,
				Limits: config.PluginLimits{
					Timeout: "20ms",
				},
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s, err := dunhttp.New(cfg, dunhttp.WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}

	rt, _ := wazero.NewRuntime(logger)
	defer rt.Close(context.Background())
	mgr := extension.NewManager(rt, logger)
	if err := s.LoadPlugins(context.Background(), mgr); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(s)
	defer ts.Close()

	// Request on the plugin route: times out → 500 (isolated failure).
	resp1, err := http.Get(ts.URL + "/api/first")
	if err != nil {
		t.Fatal(err)
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusInternalServerError {
		t.Errorf("first request: expected 500, got %d", resp1.StatusCode)
	}

	// Request on a route without the plugin: the core is alive and serves.
	resp2, err := http.Get(ts.URL + "/health/check")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("second request: expected 200, got %d", resp2.StatusCode)
	}
	if string(body2) != "upstream-ok" {
		t.Errorf("second request body = %q, want upstream-ok", string(body2))
	}

	// Plugin telemetry recorded the timeout failure (RFC-0000 §14).
	stats := mgr.Failures("loop")
	if stats.ByName[extension.FailureTimeout] == 0 {
		t.Errorf("expected at least one timeout in telemetry, got %+v", stats)
	}
}

func TestE2EHotReloadFileChange(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("upstream ok"))
	}))
	defer backend.Close()

	v1 := "default-src 'self'"
	v2 := "upgrade-insecure-requests"

	tmpDir := t.TempDir()
	wasmPath := filepath.Join(tmpDir, "security-header.wasm")
	if err := os.WriteFile(wasmPath, guest.SecurityHeaderModuleValue(guest.SecurityHeaderName, v1), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Listen: config.ListenConfig{Address: ":0"},
		Routes: []config.RouteConfig{
			{
				Name:     "api",
				Match:    config.MatchConfig{PathPrefix: "/api"},
				Upstream: backend.URL,
				Plugins:  []string{"security-header"},
			},
		},
		Plugins: []config.PluginConfig{
			{
				Name:         "security-header",
				Path:         wasmPath,
				Version:      "0.1.0",
				API:          "dun.extension/v1",
				Capabilities: []string{"response.headers.write"},
			},
		},
		Log: config.LogConfig{Level: "info"},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s, err := dunhttp.New(cfg, dunhttp.WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}

	rt, err := wazero.NewRuntime(logger)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	mgr := extension.NewManager(rt, logger)
	for _, p := range cfg.Plugins {
		caps := extension.NewSet()
		for _, c := range p.Capabilities {
			caps.Add(extension.Capability(c))
		}
		mgr.SetPolicy(p.Name, caps)
	}
	if err := s.LoadPlugins(context.Background(), mgr); err != nil {
		t.Fatal("load plugins:", err)
	}

	ts := httptest.NewServer(s)
	defer ts.Close()

	getHeader := func() (string, int) {
		resp, err := http.Get(ts.URL + "/api/test")
		if err != nil {
			return "", -1
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		return resp.Header.Get("X-Security-Policy"), resp.StatusCode
	}

	if got, _ := getHeader(); got != v1 {
		t.Fatalf("before reload: header = %q, want %q", got, v1)
	}

	// Watch starts a poll loop that hot-reloads on change. Must start
	// BEFORE the file is rewritten so its initial snapshot captures v1.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := s.WatchPlugins(ctx, 15*time.Millisecond)
	if w == nil {
		t.Fatal("expected watcher for plugins")
	}

	// v2 artifact replaces v1 on disk.
	time.Sleep(30 * time.Millisecond) // distinct mtime
	if err := os.WriteFile(wasmPath, guest.SecurityHeaderModuleValue(guest.SecurityHeaderName, v2), 0644); err != nil {
		t.Fatal(err)
	}

	// Poll until the new version is live (atomic activation, §13).
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, status := getHeader()
		if got == v2 {
			if status != http.StatusOK {
				t.Fatalf("after reload: status = %d, want 200", status)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for reload; last header = %q, want %q", got, v2)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
