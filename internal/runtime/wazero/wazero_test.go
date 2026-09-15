package wazero

import (
	"context"
	"testing"

	"github.com/NICE-DEV226/Dun/internal/extension"
	"github.com/NICE-DEV226/Dun/internal/guest"
)

func TestGuestModuleValid(t *testing.T) {
	mod := guest.SecurityHeaderModule()
	if err := guest.ValidateWasm(mod); err != nil {
		t.Fatalf("generated guest module invalid: %v", err)
	}
	if len(mod) < 32 {
		t.Fatalf("module suspiciously small: %d bytes", len(mod))
	}
}

// buildWithoutMarker builds a valid wasm module that compiles but exports a
// function named "other" instead of the Dun ABI marker.
func buildWithoutMarker(t *testing.T) []byte {
	t.Helper()
	mod := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	mod = append(mod, 0x01, 0x05, 0x01, 0x60, 0x00, 0x01, 0x7f)                    // type: () -> i32
	mod = append(mod, 0x03, 0x02, 0x01, 0x00)                                      // func section: 1 fn type 0
	mod = append(mod, 0x07, 0x09, 0x01, 0x05, 'o', 't', 'h', 'e', 'r', 0x00, 0x00) // export "other"
	mod = append(mod, 0x0a, 0x06, 0x01, 0x04, 0x00, 0x41, 0x01, 0x0b)              // code: i32.const 1; end

	r, err := NewRuntime(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close(context.Background())
	if _, err := r.rt.CompileModule(context.Background(), mod); err != nil {
		t.Fatalf("test module failed to compile: %v", err)
	}
	return mod
}

type testBridge struct {
	headers map[string]string
	msgs    []string
}

func (t *testBridge) RequestHeader(name string) string { return "" }
func (t *testBridge) SetResponseHeader(name, value string) {
	if t.headers == nil {
		t.headers = make(map[string]string)
	}
	t.headers[name] = value
}
func (t *testBridge) Log(_ int, msg string) { t.msgs = append(t.msgs, msg) }

func TestCheckABIValidModule(t *testing.T) {
	r, err := NewRuntime(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close(context.Background())

	mod := guest.SecurityHeaderModule()
	if err := r.CheckABI(context.Background(), mod); err != nil {
		t.Fatalf("expected valid ABI, got error: %v", err)
	}
}

func TestCheckABIMissingMarker(t *testing.T) {
	r, err := NewRuntime(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close(context.Background())

	mod := buildWithoutMarker(t)
	err = r.CheckABI(context.Background(), mod)
	if err == nil {
		t.Fatal("expected error for missing marker")
	}
	if !IsABIError(err) {
		t.Errorf("expected ABI error, got %v", err)
	}
}

func TestLoadInvalidBytes(t *testing.T) {
	r, err := NewRuntime(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close(context.Background())

	if _, err := r.Load(context.Background(), []byte("not wasm"), extension.LoadOptions{
		Capabilities: extension.NewSet(),
	}); err == nil {
		t.Fatal("expected error for invalid wasm bytes")
	}
}

func TestInvokeSecurityHeader(t *testing.T) {
	r, err := NewRuntime(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close(context.Background())

	bridge := &testBridge{}
	m, err := r.Load(context.Background(), guest.SecurityHeaderModule(), extension.LoadOptions{
		Capabilities: extension.NewSet(string(extension.CapResponseHeadersWrite)),
	})
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	defer func() {
		if err := m.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	}()

	res, err := m.Invoke(context.Background(), extension.Call{
		Operation: "process_request",
		Bridge:    bridge,
	})
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	if res.Decision != "continue" {
		t.Errorf("expected continue decision, got %q", res.Decision)
	}
	if got := bridge.headers["X-Security-Policy"]; got != "default-src 'self'" {
		t.Errorf("expected security header, got %q", got)
	}
}

// TestInvokeCapabilityDenied proves the authoritative runtime deny (stage 2):
// a guest importing an un-granted capability fails at instantiation, even
// though the module itself is valid.
func TestInvokeCapabilityDenied(t *testing.T) {
	r, err := NewRuntime(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close(context.Background())

	m, err := r.Load(context.Background(), guest.SecurityHeaderModule(), extension.LoadOptions{
		Capabilities: extension.NewSet(string(extension.CapLogInfo)),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close(context.Background())

	bridge := &testBridge{}
	_, err = m.Invoke(context.Background(), extension.Call{Operation: "process_request", Bridge: bridge})
	if err == nil {
		t.Fatal("expected instantiation to fail: guest imports ungranted capability")
	}
	if len(bridge.headers) != 0 {
		t.Errorf("expected no headers to be set, got %v", bridge.headers)
	}
}

// TestInvokeWithoutBridge verifies that a host function invoked without a
// per-request bridge in context does not crash the process: the host fn
// returns errno 1 (which the guest silently drops and returns CONTINUE),
// proving the failure-isolate-keep-core-alive model (RFC-0000 §14).
func TestInvokeWithoutBridge(t *testing.T) {
	r, err := NewRuntime(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close(context.Background())

	m, err := r.Load(context.Background(), guest.SecurityHeaderModule(), extension.LoadOptions{
		Capabilities: extension.NewSet(string(extension.CapResponseHeadersWrite)),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close(context.Background())

	res, err := m.Invoke(context.Background(), extension.Call{Operation: "process_request"})
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	// Guest returns CONTINUE regardless — it ignored the host errno.
	// The core is still alive; the host logged the failure.
	if res.Decision != "continue" {
		t.Errorf("expected continue decision, got %q", res.Decision)
	}
}

func TestLoadIsolatedPerPlugin(t *testing.T) {
	r, err := NewRuntime(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close(context.Background())

	opts := extension.LoadOptions{Capabilities: extension.NewSet(string(extension.CapResponseHeadersWrite))}
	m1, err := r.Load(context.Background(), guest.SecurityHeaderModule(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer m1.Close(context.Background())

	m2, err := r.Load(context.Background(), guest.SecurityHeaderModule(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Close(context.Background())

	b1 := &testBridge{}
	if _, err := m1.Invoke(context.Background(), extension.Call{Operation: "process_request", Bridge: b1}); err != nil {
		t.Fatal(err)
	}
	if got := b1.headers["X-Security-Policy"]; got == "" {
		t.Fatal("expected m1 to set header")
	}

	// m2 has its own isolated runtime; fresh bridge stays clean until invoked.
	b2 := &testBridge{}
	if len(b2.headers) != 0 {
		t.Fatal("expected fresh bridge to have no headers")
	}
	if _, err := m2.Invoke(context.Background(), extension.Call{Operation: "process_request", Bridge: b2}); err != nil {
		t.Fatal(err)
	}
	if got := b2.headers["X-Security-Policy"]; got != b1.headers["X-Security-Policy"] {
		t.Fatal("expected same header value from both isolated modules")
	}
}
