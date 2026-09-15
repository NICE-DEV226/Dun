// Package http provides the HTTP server for Dun.
package http

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/NICE-DEV226/Dun/internal/config"
	"github.com/NICE-DEV226/Dun/internal/extension"
	"github.com/NICE-DEV226/Dun/internal/metrics"
	"github.com/NICE-DEV226/Dun/internal/pipeline"
	"github.com/NICE-DEV226/Dun/internal/proxy"
	"github.com/NICE-DEV226/Dun/internal/router"
)

// Server is the Dun HTTP data plane.
type Server struct {
	cfg            *config.Config
	server         *http.Server
	router         *router.Router
	pool           *proxy.UpstreamPool
	defaultPipe    *pipeline.Pipeline
	routePipelines map[string]*pipeline.Pipeline
	logger         *slog.Logger
	metrics        *metrics.Registry
	mu             sync.Mutex
}

// Option configures the server.
type Option func(*Server)

// WithLogger sets a custom logger.
func WithLogger(l *slog.Logger) Option {
	return func(s *Server) { s.logger = l }
}

// WithPipeline sets a fallback extension pipeline (used for routes with no plugins).
func WithPipeline(p *pipeline.Pipeline) Option {
	return func(s *Server) { s.defaultPipe = p }
}

// New creates a new Dun server from the given configuration.
// Plugins defined in cfg.Plugins are loaded through the manager and wired
// into per-route extension pipelines transparently.
func New(cfg *config.Config, opts ...Option) (*Server, error) {
	logger := slog.Default()
	s := &Server{
		cfg:            cfg,
		router:         router.New(cfg),
		pool:           proxy.NewUpstreamPool(logger),
		logger:         logger,
		metrics:        metrics.NewRegistry(),
		routePipelines: make(map[string]*pipeline.Pipeline),
	}

	for _, opt := range opts {
		opt(s)
	}

	if s.defaultPipe == nil {
		s.defaultPipe = pipeline.New(nil, s.logger)
	}

	for _, r := range cfg.Routes {
		if err := s.pool.Add(r.Name, r.Upstream); err != nil {
			return nil, fmt.Errorf("register upstream %q: %w", r.Name, err)
		}
	}

	s.server = &http.Server{
		Addr:              cfg.Listen.Address,
		Handler:           s,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
	}

	return s, nil
}

// LoadPlugins reads plugin wasm artifacts from disk, loads them into the
// given manager (ABI check + compilation + activation), and wires each
// referenced route with an HTTPExtension adapter in its pipeline.
// It must be called after New and before ListenAndServe.
func (s *Server) LoadPlugins(ctx context.Context, mgr *extension.Manager) error {
	// Load each plugin wasm artifact.
	for _, p := range s.cfg.Plugins {
		if p.Path == "" {
			s.logger.Debug("plugin has no wasm path, skipping", "name", p.Name)
			continue
		}
		data, err := os.ReadFile(p.Path)
		if err != nil {
			return fmt.Errorf("read plugin %q: %w", p.Name, err)
		}

		manifest := &extension.Manifest{
			Name:    p.Name,
			Version: p.Version,
			API:     p.API,
		}
		if manifest.API == "" {
			manifest.API = "dun.extension/v1"
		}
		if manifest.Version == "" {
			manifest.Version = "0.1.0"
		}
		manifest.Caps.Request = filterCapPrefix(p.Capabilities, "request.")
		manifest.Caps.Response = filterCapPrefix(p.Capabilities, "response.")
		manifest.Caps.Context = filterCapPrefix(p.Capabilities, "context.")
		manifest.Caps.Log = filterCapPrefix(p.Capabilities, "log.")

		if err := mgr.Load(p.Name, manifest, data); err != nil {
			return fmt.Errorf("load plugin %q: %w", p.Name, err)
		}
		if err := mgr.Activate(p.Name); err != nil {
			return fmt.Errorf("activate plugin %q: %w", p.Name, err)
		}
		s.logger.Info("plugin loaded", "name", p.Name, "path", p.Path)
	}

	// Build per-route pipelines from the route's plugin list.
	for _, route := range s.router.Routes() {
		if len(route.Plugins) == 0 {
			continue
		}
		var exts []pipeline.Extension
		for _, name := range route.Plugins {
			exts = append(exts, &extension.HTTPExtension{
				Name_:   name,
				Manager: mgr,
				Logger:  s.logger,
			})
		}
		s.routePipelines[route.Name] = pipeline.New(exts, s.logger)
		s.logger.Debug("route pipeline wired",
			"route", route.Name,
			"plugins", route.Plugins,
		)
	}
	return nil
}

func filterCapPrefix(caps []string, prefix string) []string {
	var out []string
	for _, c := range caps {
		if len(c) > len(prefix) && c[:len(prefix)] == prefix {
			out = append(out, c[len(prefix):])
		}
	}
	return out
}

// ServeHTTP implements http.Handler. This is the main request entry point.
func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	start := time.Now()

	s.metrics.Incr("requests_total")

	route := s.router.Match(req)
	if route == nil {
		s.logger.Debug("no route matched", "path", req.URL.Path)
		http.NotFound(w, req)
		s.metrics.Incr("requests_not_found")
		return
	}

	s.logger.Debug("route matched",
		"route", route.Name,
		"upstream", route.Upstream,
		"path", req.URL.Path,
	)

	// Apply route-level response headers
	for k, v := range route.Headers {
		w.Header().Set(k, v)
	}

	// Execute extension pipeline (per-route or default empty)
	pipe := s.routePipelines[route.Name]
	if pipe == nil {
		pipe = s.defaultPipe
	}
	if pipe != nil && pipe.Len() > 0 {
		decision, caller := pipe.Execute(req.Context(), w, req)
		switch decision {
		case pipeline.DecisionReject:
			s.logger.Warn("request rejected by extension", "extension", caller.Name())
			s.metrics.Incr("requests_rejected")
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		case pipeline.DecisionRespond:
			s.logger.Debug("extension handled response", "extension", caller.Name())
			s.metrics.Incr("requests_responded")
			return
		case pipeline.DecisionError:
			s.logger.Error("extension error", "extension", caller.Name())
			s.metrics.Incr("extension_errors")
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	// Proxy to upstream
	s.pool.ServeHTTP(w, req, route.Name)

	duration := time.Since(start)
	s.metrics.Observe("request_duration_seconds", duration.Seconds())
	s.logger.Debug("request completed",
		"route", route.Name,
		"duration_ms", duration.Milliseconds(),
	)
}

// ListenAndServe starts the HTTP server and blocks until the context is cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		s.logger.Info("dun starting", "address", s.cfg.Listen.Address)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.logger.Info("shutting down dun")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.server.Shutdown(shutdownCtx)
	}
}

// Metrics returns the server's metrics registry.
func (s *Server) Metrics() *metrics.Registry {
	return s.metrics
}

// Router returns the server's router for testing.
func (s *Server) Router() *router.Router {
	return s.router
}
