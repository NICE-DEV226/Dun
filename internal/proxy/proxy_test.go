package proxy

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestUpstreamPoolAddAndGet(t *testing.T) {
	pool := NewUpstreamPool(nil)
	err := pool.Add("backend", "http://localhost:3000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	u := pool.Get("backend")
	if u == nil {
		t.Fatal("expected upstream to exist")
	}
	if u.URL.Host != "localhost:3000" {
		t.Errorf("expected localhost:3000, got %s", u.URL.Host)
	}
}

func TestUpstreamPoolGetNotFound(t *testing.T) {
	pool := NewUpstreamPool(nil)
	if pool.Get("nonexistent") != nil {
		t.Fatal("expected nil for nonexistent upstream")
	}
}

func TestUpstreamPoolServeHTTP(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello from backend"))
	}))
	defer backend.Close()

	pool := NewUpstreamPool(slog.Default())
	if err := pool.Add("test", backend.URL); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	pool.ServeHTTP(rec, req, "test")

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "hello from backend" {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestUpstreamPoolForwardedProto(t *testing.T) {
	protocols := make(chan string, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		protocols <- r.Header.Get("X-Forwarded-Proto")
	}))
	defer backend.Close()

	pool := NewUpstreamPool(nil)
	if err := pool.Add("test", backend.URL); err != nil {
		t.Fatal(err)
	}
	pool.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil), "test")
	if got := <-protocols; got != "http" {
		t.Fatalf("expected http, got %q", got)
	}
}

func TestUpstreamPoolServeHTTPNotFound(t *testing.T) {
	pool := NewUpstreamPool(nil)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	pool.ServeHTTP(rec, req, "nonexistent")

	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", rec.Code)
	}
}

func TestInvalidURL(t *testing.T) {
	pool := NewUpstreamPool(nil)
	u, _ := url.Parse("http://invalid")
	_ = u
	// Add with invalid URL should fail
	err := pool.Add("bad", "://invalid")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}
