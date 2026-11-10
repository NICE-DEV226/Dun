package conformance_test

import (
	"context"
	"testing"

	publicextension "github.com/NICE-DEV226/Dun/api/extension"
	"github.com/NICE-DEV226/Dun/internal/extension"
	"github.com/NICE-DEV226/Dun/internal/guest"
	"github.com/NICE-DEV226/Dun/internal/runtime/wazero"
)

func TestCoreWasmV0SecurityHeaderConformance(t *testing.T) {
	runtime, err := wazero.NewRuntime(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(context.Background())

	artifact := guest.SecurityHeaderModule()
	if err := runtime.CheckABI(context.Background(), artifact); err != nil {
		t.Fatalf("ABI check failed: %v", err)
	}

	module, err := runtime.Load(context.Background(), artifact, extension.LoadOptions{
		Capabilities: extension.NewSet(publicextension.CapabilityResponseHeadersWrite),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer module.Close(context.Background())

	bridge := &bridge{}
	result, err := module.Invoke(context.Background(), extension.Call{
		Operation: "process_request",
		Bridge:    bridge,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != publicextension.DecisionContinue.String() {
		t.Fatalf("decision = %q, want %q", result.Decision, publicextension.DecisionContinue.String())
	}
	if bridge.headers[guest.SecurityHeaderName] != guest.SecurityHeaderValue {
		t.Fatalf("security header = %q, want %q", bridge.headers[guest.SecurityHeaderName], guest.SecurityHeaderValue)
	}
}

type bridge struct {
	headers map[string]string
}

func (b *bridge) RequestHeader(string) string { return "" }
func (b *bridge) RequestHeaderValue(string) (string, bool) {
	return "", false
}
func (b *bridge) SetResponseHeader(name, value string) {
	if b.headers == nil {
		b.headers = make(map[string]string)
	}
	b.headers[name] = value
}
func (b *bridge) Log(int, string) {}
