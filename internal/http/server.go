// Package http provides the HTTP server for Dun.
package http

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/NICE-DEV226/Dun/internal/config"
	"github.com/NICE-DEV226/Dun/internal/metrics"
	"github.com/NICE-DEV226/Dun/internal/pipeline"
	"github.com/NICE-DEV226/Dun/internal/proxy"
	"github.com/NICE-DEV226/Dun/internal/router"
)

// Server is the Dun HTTP data plane.
type Server struct {
	cfg      *config.Config
	server   *http.Server
	router   *router.Router
	pool     *proxy.UpstreamPool
	pipeline *pipeline.Pipeline
	logger   *slog.Logger
	metrics  *metrics.Registry
	mu       sync.Mutex
}

// Option configures the server.
type Option func(*Server)

// WithLogger sets a custom logger.
func WithLogger(l *slog.Logger) Option {
	return func(s *Server) { s.logger = l }
}

// WithPipeline sets the extension pipeline.
func WithPipeline(p *pipeline.Pipeline) Option {
	return func(s *Server) { s.pipeline = p }
}

// New creates a new Dun server from the given configuration.
func New(cfg *config.Config, opts ...Option) (*Server, error) {
	logger := slog.Default()
	s := &Server{
		cfg:     cfg,
		router:  router.New(cfg),
		pool:    proxy.NewUpstreamPool(logger),
		logger:  logger,
		metrics: metrics.NewRegistry(),
	}

	for _, opt := range opts {
		opt(s)
	}

	if s.pipeline == nil {
		s.pipeline = pipeline.New(nil, s.logger)
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

	// Execute extension pipeline
	if s.pipeline.Len() > 0 {
		decision, caller := s.pipeline.Execute(req.Context(), w, req)
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
