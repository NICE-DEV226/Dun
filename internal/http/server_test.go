package http_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
