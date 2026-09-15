package version

import "testing"

func TestVersionString(t *testing.T) {
	s := String()
	if s == "" {
		t.Fatal("expected non-empty version string")
	}
	if len(s) < len("dun 0.1.0") {
		t.Errorf("unexpected version string: %s", s)
	}
}

func TestVersionFields(t *testing.T) {
	if Version == "" {
		t.Error("expected Version to be set")
	}
	if Commit == "" {
		t.Error("expected Commit to be set")
	}
}
