// Package http provides the HTTP server for Dun.
package http

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/NICE-DEV226/Dun/internal/config"
	"github.com/NICE-DEV226/Dun/internal/extension"
	"github.com/NICE-DEV226/Dun/internal/metrics"
	"github.com/NICE-DEV226/Dun/internal/pipeline"
	"github.com/NICE-DEV226/Dun/internal/proxy"
	"github.com/NICE-DEV226/Dun/internal/reload"
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
	mgr            *extension.Manager
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
	s.mgr = mgr

	// Load each plugin wasm artifact.
	for _, p := range s.cfg.Plugins {
		if err := s.loadOnePlugin(ctx, mgr, p); err != nil {
			return err
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

// loadOnePlugin reads a plugin's wasm artifact from disk and loads it
// into the manager with the full lifecycle (DISCOVER → … → SERVING).
func (s *Server) loadOnePlugin(ctx context.Context, mgr *extension.Manager, p config.PluginConfig) error {
	if p.Path == "" {
		s.logger.Debug("plugin has no wasm path, skipping", "name", p.Name)
		return nil
	}
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return fmt.Errorf("read plugin %q: %w", p.Name, err)
	}
	if p.SHA256 != "" {
		digest := sha256.Sum256(data)
		if !strings.EqualFold(p.SHA256, hex.EncodeToString(digest[:])) {
			return fmt.Errorf("plugin %q: sha256 mismatch", p.Name)
		}
	}

	manifest := buildManifest(p)

	// Parse and configure sandbox resource limits (RFC-0000 §11).
	rl, err := extension.ParseResourceLimits(extension.Limits{
		Memory:    p.Limits.Memory,
		Timeout:   p.Limits.Timeout,
		BodySize:  p.Limits.BodySize,
		Instances: p.Limits.Instances,
	})
	if err != nil {
		return fmt.Errorf("plugin %q limits: %w", p.Name, err)
	}
	mgr.SetLimits(p.Name, rl)

	// Deployment policy = the capabilities declared in config
	// (stage-1 validation target, RFC-0000 §10.1).
	mgr.SetPolicy(p.Name, extension.NewSet(p.Capabilities...))

	if err := mgr.Load(p.Name, manifest, data); err != nil {
		return fmt.Errorf("load plugin %q: %w", p.Name, err)
	}
	if err := mgr.Activate(p.Name); err != nil {
		return fmt.Errorf("activate plugin %q: %w", p.Name, err)
	}
	return nil
}

// buildManifest constructs a Manifest from a PluginConfig entry.
func buildManifest(p config.PluginConfig) *extension.Manifest {
	m := &extension.Manifest{
		Name:    p.Name,
		Version: p.Version,
		API:     p.API,
	}
	if m.API == "" {
		m.API = "dun.extension/v1"
	}
	if m.Version == "" {
		m.Version = "0.1.0"
	}
	m.Caps.Request = filterCapPrefix(p.Capabilities, "request.")
	m.Caps.Response = filterCapPrefix(p.Capabilities, "response.")
	m.Caps.Context = filterCapPrefix(p.Capabilities, "context.")
	m.Caps.Log = filterCapPrefix(p.Capabilities, "log.")
	return m
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

// WatchPlugins starts a poll-based file watcher for all plugin wasm
// artifacts declared in the configuration. When a file is modified, the
// corresponding plugin is reloaded via manager.Reload (atomic activation,
// RFC-0000 §13). The watcher stops when ctx is cancelled.
func (s *Server) WatchPlugins(ctx context.Context, interval time.Duration) *reload.Watcher {
	s.mu.Lock()
	defer s.mu.Unlock()

	var paths []string
	for _, p := range s.cfg.Plugins {
		if p.Path != "" {
			paths = append(paths, p.Path)
		}
	}
	if len(paths) == 0 {
		return nil
	}

	w := reload.NewWatcher(paths, interval, s.onPluginFileChanged, s.logger)
	go w.Start(ctx)
	return w
}

// onPluginFileChanged is the callback invoked by the watcher when a wasm
// file changes on disk. It reads the new artifact and delegates to
// manager.Reload for atomic activation.
func (s *Server) onPluginFileChanged(path string) {
	mgr := s.mgr
	if mgr == nil {
		s.logger.Warn("plugin changed but manager not set", "path", path)
		return
	}

	cfg := s.pluginCfgByPath(path)
	if cfg == nil {
		s.logger.Warn("plugin changed but no config found for path", "path", path)
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		s.logger.Error("failed to read changed plugin", "path", path, "error", err)
		return
	}

	manifest := buildManifest(*cfg)
	if err := mgr.Reload(cfg.Name, data, manifest); err != nil {
		s.logger.Error("plugin reload failed",
			"extension", cfg.Name,
			"path", path,
			"error", err,
		)
		return
	}
	s.logger.Info("plugin reloaded via watcher",
		"extension", cfg.Name,
		"path", path,
	)
}

// pluginCfgByPath returns the PluginConfig with the given path, or nil.
func (s *Server) pluginCfgByPath(path string) *config.PluginConfig {
	for i := range s.cfg.Plugins {
		if s.cfg.Plugins[i].Path == path {
			return &s.cfg.Plugins[i]
		}
	}
	return nil
}

// ServeHTTP implements http.Handler. This is the main request entry point.
func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	start := time.Now()
	defer func() {
		s.metrics.Observe("request_duration_seconds", time.Since(start).Seconds())
	}()

	s.metrics.Incr("requests_total")
	if req.URL.Path == "/healthz" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if req.URL.Path == "/readyz" {
		w.WriteHeader(http.StatusOK)
		return
	}
	if req.URL.Path == "/metrics" {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(s.metrics.Prometheus()))
		return
	}

	route := s.router.Match(req)
	if route == nil {
		s.logger.Debug("no route matched", "path", req.URL.Path)
		http.NotFound(w, req)
		s.metrics.Incr("requests_not_found")
		return
	}

	pipe := s.routePipelines[route.Name]
	if pipe == nil {
		pipe = s.defaultPipe
	}
	response := &responseWriter{ResponseWriter: w}
	if pipe != nil && pipe.Len() > 0 {
		defer func() {
			pipe.ExecuteResponse(req.Context(), response, req, response.statusCode())
		}()
	}

	s.logger.Debug("route matched",
		"route", route.Name,
		"upstream", route.Upstream,
		"path", req.URL.Path,
	)
	s.metrics.Incr("route." + route.Name + ".requests_total")

	// Apply route-level response headers
	for k, v := range route.Headers {
		response.Header().Set(k, v)
	}

	// Execute extension pipeline (per-route or default empty)
	if pipe != nil && pipe.Len() > 0 {
		decision, caller := pipe.Execute(req.Context(), response, req)
		switch decision {
		case pipeline.DecisionReject:
			s.logger.Warn("request rejected by extension", "extension", caller.Name())
			s.metrics.Incr("requests_rejected")
			http.Error(response, "forbidden", http.StatusForbidden)
			return
		case pipeline.DecisionRespond:
			s.logger.Debug("extension handled response", "extension", caller.Name())
			s.metrics.Incr("requests_responded")
			return
		case pipeline.DecisionError:
			s.logger.Error("extension error", "extension", caller.Name())
			s.metrics.Incr("extension_errors")
			http.Error(response, "internal error", http.StatusInternalServerError)
			return
		}
	}

	// Proxy to upstream
	s.pool.ServeHTTP(response, req, route.Name)

	duration := time.Since(start)
	s.logger.Debug("request completed",
		"route", route.Name,
		"duration_ms", duration.Milliseconds(),
	)
}

type responseWriter struct {
	http.ResponseWriter
	status  int
	written bool
}

func (w *responseWriter) WriteHeader(status int) {
	if w.written {
		return
	}
	w.status = status
	w.written = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(body []byte) (int, error) {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *responseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		if !w.written {
			w.WriteHeader(http.StatusOK)
		}
		flusher.Flush()
	}
}

func (w *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func (w *responseWriter) Push(target string, options *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, options)
}

func (w *responseWriter) ReadFrom(src io.Reader) (int64, error) {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	if reader, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return reader.ReadFrom(src)
	}
	return io.Copy(w.ResponseWriter, src)
}

func (w *responseWriter) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
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
