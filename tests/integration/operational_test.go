package integration_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NICE-DEV226/Dun/internal/config"
	dunhttp "github.com/NICE-DEV226/Dun/internal/http"
)

func TestOperationalEndpoints(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer backend.Close()

	server, err := dunhttp.New(&config.Config{
		Listen: config.ListenConfig{Address: ":0"},
		Routes: []config.RouteConfig{{
			Name: "default", Match: config.MatchConfig{PathPrefix: "/"}, Upstream: backend.URL,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	frontend := httptest.NewServer(server)
	defer frontend.Close()

	for _, path := range []string{"/healthz", "/readyz"} {
		response, err := http.Get(frontend.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Errorf("%s status = %d, want 200", path, response.StatusCode)
		}
	}
	response, err := http.Get(frontend.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()

	response, err = http.Get(frontend.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "requests_total") || !strings.Contains(string(body), "route_default_requests_total") {
		t.Fatalf("metrics response = %d %q", response.StatusCode, body)
	}
}
