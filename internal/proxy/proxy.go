// Package proxy provides the reverse proxy implementation for Dun.
package proxy

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
)

// Upstream represents a single upstream service.
type Upstream struct {
	Name   string
	URL    *url.URL
	Proxy  *httputil.ReverseProxy
	Weight int
}

// UpstreamPool manages a set of upstream targets.
type UpstreamPool struct {
	upstreams map[string]*Upstream
	mu        sync.RWMutex
	logger    *slog.Logger
}

// NewUpstreamPool creates a new upstream pool.
func NewUpstreamPool(logger *slog.Logger) *UpstreamPool {
	if logger == nil {
		logger = slog.Default()
	}
	return &UpstreamPool{
		upstreams: make(map[string]*Upstream),
		logger:    logger,
	}
}

// Add registers an upstream by name.
func (p *UpstreamPool) Add(name, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}

	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.ErrorHandler = p.errorHandler(name)

	p.mu.Lock()
	defer p.mu.Unlock()

	p.upstreams[name] = &Upstream{
		Name:  name,
		URL:   u,
		Proxy: proxy,
	}

	p.logger.Info("upstream registered", "name", name, "url", rawURL)
	return nil
}

// Get retrieves an upstream by name.
func (p *UpstreamPool) Get(name string) *Upstream {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.upstreams[name]
}

// ServeHTTP proxies a request to the named upstream.
func (p *UpstreamPool) ServeHTTP(w http.ResponseWriter, req *http.Request, upstreamName string) {
	u := p.Get(upstreamName)
	if u == nil {
		p.logger.Error("upstream not found", "name", upstreamName)
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}

	// Remove the X-Forwarded-For header if already set, let the proxy set it
	req.Header.Del("X-Forwarded-For")
	req.Header.Set("X-Forwarded-Host", req.Host)
	protocol := "http"
	if req.TLS != nil {
		protocol = "https"
	}
	req.Header.Set("X-Forwarded-Proto", protocol)

	u.Proxy.ServeHTTP(w, req)
}

func (p *UpstreamPool) errorHandler(name string) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, _ *http.Request, err error) {
		p.logger.Error("upstream error",
			"upstream", name,
			"error", err,
		)
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}
}
