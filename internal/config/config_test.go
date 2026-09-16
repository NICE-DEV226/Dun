package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Listen.Address != ":8080" {
		t.Errorf("expected :8080, got %s", cfg.Listen.Address)
	}
}

func TestLoadValidConfig(t *testing.T) {
	content := `
listen:
  address: ":9090"
routes:
  - name: api
    match:
      path_prefix: /api
    upstream: http://localhost:3000
log:
  level: debug
  format: json
`
	dir := t.TempDir()
	path := filepath.Join(dir, "dun.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Listen.Address != ":9090" {
		t.Errorf("expected :9090, got %s", cfg.Listen.Address)
	}
	if len(cfg.Routes) != 1 {
		t.Errorf("expected 1 route, got %d", len(cfg.Routes))
	}
	if cfg.Routes[0].Upstream != "http://localhost:3000" {
		t.Errorf("unexpected upstream: %s", cfg.Routes[0].Upstream)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/dun.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestValidateEmptyRoutes(t *testing.T) {
	cfg := &Config{
		Listen: ListenConfig{Address: ":8080"},
		Routes: []RouteConfig{},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for empty routes")
	}
}

func TestValidateMissingUpstream(t *testing.T) {
	cfg := &Config{
		Listen: ListenConfig{Address: ":8080"},
		Routes: []RouteConfig{
			{Match: MatchConfig{PathPrefix: "/api"}},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing upstream")
	}
}

func TestValidateMissingMatch(t *testing.T) {
	cfg := &Config{
		Listen: ListenConfig{Address: ":8080"},
		Routes: []RouteConfig{
			{Upstream: "http://localhost:3000"},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing match")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte(":::not yaml"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestValidateReferencedPluginNeedsPath(t *testing.T) {
	cfg := &Config{
		Listen:  ListenConfig{Address: ":8080"},
		Routes:  []RouteConfig{{Name: "api", Match: MatchConfig{PathPrefix: "/api"}, Upstream: "http://localhost:3000", Plugins: []string{"auth"}}},
		Plugins: []PluginConfig{{Name: "auth"}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected referenced plugin without path to fail validation")
	}
}

func TestValidatePluginSHA256(t *testing.T) {
	cfg := &Config{
		Listen:  ListenConfig{Address: ":8080"},
		Routes:  []RouteConfig{{Name: "api", Match: MatchConfig{PathPrefix: "/api"}, Upstream: "http://localhost:3000"}},
		Plugins: []PluginConfig{{Name: "auth", Path: "auth.wasm", SHA256: "not-a-digest"}},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid SHA256 to fail validation")
	}
}
