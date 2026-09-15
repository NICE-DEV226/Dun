package wazero

import (
	"context"
	"os"
	"testing"

	"github.com/NICE-DEV226/Dun/internal/extension"
)

func TestCommittedWasmArtifact(t *testing.T) {
	data, err := os.ReadFile("../../../plugins/examples/security-header/security-header.wasm")
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewRuntime(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close(context.Background())
	m, err := r.Load(context.Background(), data, extension.LoadOptions{
		Capabilities: extension.NewSet(string(extension.CapResponseHeadersWrite)),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close(context.Background())
	res, err := m.Invoke(context.Background(), extension.Call{Operation: "process_request"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != "continue" {
		t.Fatalf("expected continue, got %q", res.Decision)
	}
}
