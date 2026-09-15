package extension

import "testing"

func TestParseManifest(t *testing.T) {
	content := []byte(`
name: dun-auth-jwt
version: 0.1.0
api: "dun.extension/v1"
runtime:
  type: wasm
  format: core-module
capabilities:
  request:
    - headers.read
  response:
    - status.write
limits:
  memory: 64MiB
  timeout: 5ms
`)
	m, err := ParseManifest(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Name != "dun-auth-jwt" {
		t.Errorf("expected dun-auth-jwt, got %s", m.Name)
	}
	if m.Version != "0.1.0" {
		t.Errorf("expected 0.1.0, got %s", m.Version)
	}
	if !m.All().Has(CapRequestHeadersRead) {
		t.Error("expected request.headers.read")
	}
	if !m.All().Has(CapResponseStatusWrite) {
		t.Error("expected response.status.write")
	}
}

func TestParseManifestMissingName(t *testing.T) {
	content := []byte(`
version: 0.1.0
`)
	_, err := ParseManifest(content)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestParseManifestBadCapability(t *testing.T) {
	content := []byte(`
name: test
version: 0.1.0
capabilities:
  request:
    - bogus.capability
`)
	_, err := ParseManifest(content)
	if err == nil {
		t.Fatal("expected error for bogus capability")
	}
}

func TestParseManifestInvalidYAML(t *testing.T) {
	_, err := ParseManifest([]byte(":::bad"))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestManifestAll(t *testing.T) {
	m := Manifest{
		Caps: Caps{
			Request:  []string{"headers.read"},
			Response: []string{"status.write"},
			Context:  []string{"metadata.write"},
			Log:      []string{"info"},
		},
	}
	all := m.All()
	if len(all) != 4 {
		t.Errorf("expected 4 capabilities, got %d", len(all))
	}
	if !all.Has(CapLogInfo) {
		t.Error("expected log.info")
	}
}
