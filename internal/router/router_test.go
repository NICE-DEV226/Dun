package router

import (
	"net/http/httptest"
	"testing"

	"github.com/NICE-DEV226/Dun/internal/config"
)

func TestMatchPrefix(t *testing.T) {
	r := newTestRouter(t, &config.Config{
		Routes: []config.RouteConfig{
			{
				Name:     "api",
				Match:    config.MatchConfig{PathPrefix: "/api"},
				Upstream: "http://localhost:3000",
			},
		},
	})

	req := httptest.NewRequest("GET", "/api/users", nil)
	route := r.Match(req)
	if route == nil {
		t.Fatal("expected route match for /api/users")
	}
	if route.Name != "api" {
		t.Errorf("expected route name 'api', got '%s'", route.Name)
	}
}

func TestMatchExact(t *testing.T) {
	r := newTestRouter(t, &config.Config{
		Routes: []config.RouteConfig{
			{
				Name:     "health",
				Match:    config.MatchConfig{PathExact: "/health"},
				Upstream: "http://localhost:9090",
			},
		},
	})

	req := httptest.NewRequest("GET", "/health", nil)
	route := r.Match(req)
	if route == nil {
		t.Fatal("expected match for /health")
	}

	req2 := httptest.NewRequest("GET", "/health/check", nil)
	route2 := r.Match(req2)
	if route2 != nil {
		t.Fatal("expected no match for /health/check with exact rule")
	}
}

func TestMatchMethodFilter(t *testing.T) {
	r := newTestRouter(t, &config.Config{
		Routes: []config.RouteConfig{
			{
				Name:     "post-only",
				Match:    config.MatchConfig{PathPrefix: "/api", Method: "POST"},
				Upstream: "http://localhost:3000",
			},
		},
	})

	req := httptest.NewRequest("POST", "/api/data", nil)
	if r.Match(req) == nil {
		t.Fatal("expected match for POST /api/data")
	}

	req2 := httptest.NewRequest("GET", "/api/data", nil)
	if r.Match(req2) != nil {
		t.Fatal("expected no match for GET /api/data")
	}
}

func TestNoMatch(t *testing.T) {
	r := newTestRouter(t, &config.Config{
		Routes: []config.RouteConfig{
			{
				Name:     "api",
				Match:    config.MatchConfig{PathPrefix: "/api"},
				Upstream: "http://localhost:3000",
			},
		},
	})

	req := httptest.NewRequest("GET", "/other", nil)
	if r.Match(req) != nil {
		t.Fatal("expected no match for /other")
	}
}

func TestMatchPrefixDoesNotMatchSiblingPath(t *testing.T) {
	r := newTestRouter(t, &config.Config{
		Routes: []config.RouteConfig{{
			Name: "api", Match: config.MatchConfig{PathPrefix: "/api"}, Upstream: "http://localhost:3000",
		}},
	})

	if route := r.Match(httptest.NewRequest("GET", "/api-v2", nil)); route != nil {
		t.Fatalf("expected no match for sibling path, got %q", route.Name)
	}
}

func TestMultipleRoutes(t *testing.T) {
	r := newTestRouter(t, &config.Config{
		Routes: []config.RouteConfig{
			{
				Name:     "health",
				Match:    config.MatchConfig{PathExact: "/health"},
				Upstream: "http://localhost:9090",
			},
			{
				Name:     "api",
				Match:    config.MatchConfig{PathPrefix: "/api"},
				Upstream: "http://localhost:3000",
			},
		},
	})

	req := httptest.NewRequest("GET", "/api/users", nil)
	route := r.Match(req)
	if route == nil || route.Name != "api" {
		t.Fatal("expected 'api' route")
	}

	req2 := httptest.NewRequest("GET", "/health", nil)
	route2 := r.Match(req2)
	if route2 == nil || route2.Name != "health" {
		t.Fatal("expected 'health' route")
	}
}

func newTestRouter(t *testing.T, cfg *config.Config) *Router {
	t.Helper()
	return New(cfg)
}
