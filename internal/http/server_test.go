package http_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NICE-DEV226/Dun/internal/config"
	dunhttp "github.com/NICE-DEV226/Dun/internal/http"
	"github.com/NICE-DEV226/Dun/internal/metrics"
)

func TestServerRoutesToUpstream(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream", "true")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("upstream response"))
	}))
	defer backend.Close()

	cfg := &config.Config{
		Listen: config.ListenConfig{Address: ":0"},
		Routes: []config.RouteConfig{
			{
				Name:     "api",
				Match:    config.MatchConfig{PathPrefix: "/api"},
				Upstream: backend.URL,
			},
		},
		Log: config.LogConfig{Level: "info"},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s, err := dunhttp.New(cfg, dunhttp.WithLogger(logger))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ts := httptest.NewServer(s)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "upstream response" {
		t.Errorf("unexpected body: %s", string(body))
	}
}

func TestServerReturns404ForNoRoute(t *testing.T) {
	cfg := &config.Config{
		Listen: config.ListenConfig{Address: ":0"},
		Routes: []config.RouteConfig{
			{
				Name:     "api",
				Match:    config.MatchConfig{PathPrefix: "/api"},
				Upstream: "http://localhost:9999",
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

	resp, err := http.Get(ts.URL + "/other")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestServerSetsHeaders(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer backend.Close()

	cfg := &config.Config{
		Listen: config.ListenConfig{Address: ":0"},
		Routes: []config.RouteConfig{
			{
				Name:     "api",
				Match:    config.MatchConfig{PathPrefix: "/api"},
				Upstream: backend.URL,
				Headers:  map[string]string{"X-Custom": "hello"},
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

	resp, err := http.Get(ts.URL + "/api/test")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("X-Custom") != "hello" {
		t.Errorf("expected X-Custom header, got %s", resp.Header.Get("X-Custom"))
	}
}

func TestServerMetrics(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	cfg := &config.Config{
		Listen: config.ListenConfig{Address: ":0"},
		Routes: []config.RouteConfig{
			{
				Name:     "api",
				Match:    config.MatchConfig{PathPrefix: "/api"},
				Upstream: backend.URL,
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

	for i := 0; i < 5; i++ {
		resp, _ := http.Get(ts.URL + "/api/test")
		if resp != nil {
			resp.Body.Close()
		}
	}

	m := s.Metrics()
	total := m.GetCounter("requests_total")
	if total != 5 {
		t.Errorf("expected 5 requests_total, got %d", total)
	}
}

func TestServerShutdown(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	cfg := &config.Config{
		Listen: config.ListenConfig{Address: ":0"},
		Routes: []config.RouteConfig{
			{
				Name:     "api",
				Match:    config.MatchConfig{PathPrefix: "/"},
				Upstream: backend.URL,
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s, err := dunhttp.New(cfg, dunhttp.WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- s.ListenAndServe(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil && !strings.Contains(err.Error(), "closed") {
			t.Errorf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown timed out")
	}
}

func TestNewMetricsRegistry(t *testing.T) {
	r := metrics.NewRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
}
