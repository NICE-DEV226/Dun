// Package router implements HTTP request routing for Dun.
package router

import (
	"net/http"
	"strings"

	"github.com/NICE-DEV226/Dun/internal/config"
)

// Route represents a compiled routing rule.
type Route struct {
	Name     string
	Prefix   string
	Exact    string
	Host     string
	Method   string
	Upstream string
	Plugins  []string
	Headers  map[string]string
}

// Router matches incoming requests to configured routes.
type Router struct {
	routes []Route
}

// New creates a Router from the given configuration.
func New(cfg *config.Config) *Router {
	routes := make([]Route, 0, len(cfg.Routes))
	for _, r := range cfg.Routes {
		routes = append(routes, Route{
			Name:     r.Name,
			Prefix:   r.Match.PathPrefix,
			Exact:    r.Match.PathExact,
			Host:     r.Match.Host,
			Method:   r.Match.Method,
			Upstream: r.Upstream,
			Plugins:  r.Plugins,
			Headers:  r.Headers,
		})
	}
	return &Router{routes: routes}
}

// Match finds the first route matching the given request.
// Returns nil if no route matches.
func (r *Router) Match(req *http.Request) *Route {
	for i := range r.routes {
		route := &r.routes[i]
		if r.matches(route, req) {
			return route
		}
	}
	return nil
}

// Routes returns all configured routes.
func (r *Router) Routes() []Route {
	return r.routes
}

func (r *Router) matches(route *Route, req *http.Request) bool {
	path := req.URL.Path

	if route.Exact != "" && path != route.Exact {
		return false
	}
	if route.Prefix != "" && !strings.HasPrefix(path, route.Prefix) {
		return false
	}
	if route.Host != "" && req.Host != route.Host {
		return false
	}
	if route.Method != "" && req.Method != route.Method {
		return false
	}
	return true
}
