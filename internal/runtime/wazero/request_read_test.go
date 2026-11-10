package wazero

import (
	"context"
	"testing"

	"github.com/NICE-DEV226/Dun/internal/extension"
	"github.com/NICE-DEV226/Dun/internal/guest"
)

func loadHeaderReadModule(t *testing.T, rt *Runtime) extension.Module {
	t.Helper()
	mod, err := rt.Load(context.Background(), guest.HeaderReadModule(), extension.LoadOptions{
		Capabilities: extension.NewSet(string(extension.CapRequestHeadersRead)),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mod.Close(context.Background()) })
	return mod
}

func TestRequestHeadersReadRejectsDeniedHeader(t *testing.T) {
	rt, err := NewRuntime(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	mod := loadHeaderReadModule(t, rt)

	bridge := &testBridge{reqHeaders: map[string]string{"X-Denied": "1"}}
	res, err := mod.Invoke(context.Background(), extension.Call{
		Operation: "process_request",
		Bridge:    bridge,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != "reject" {
		t.Fatalf("expected reject decision, got %q", res.Decision)
	}
}

func TestRequestHeadersReadContinuesWhenAbsent(t *testing.T) {
	rt, err := NewRuntime(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	mod := loadHeaderReadModule(t, rt)

	bridge := &testBridge{reqHeaders: map[string]string{}}
	res, err := mod.Invoke(context.Background(), extension.Call{
		Operation: "process_request",
		Bridge:    bridge,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != "continue" {
		t.Fatalf("expected continue decision, got %q", res.Decision)
	}
}

// A plugin importing request.headers.read without the capability must
// fail at instantiation — runtime denial is authoritative (§10.1 stage 2).
// Compilation succeeds (imports resolve at instantiation), so the error
// surfaces on the first Invoke.
func TestRequestHeadersReadDeniedWithoutCapability(t *testing.T) {
	rt, err := NewRuntime(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	mod, err := rt.Load(context.Background(), guest.HeaderReadModule(), extension.LoadOptions{
		Capabilities: extension.NewSet(), // no capabilities granted
	})
	if err != nil {
		t.Fatalf("compile should succeed; denial happens at instantiation: %v", err)
	}
	defer mod.Close(context.Background())

	_, err = mod.Invoke(context.Background(), extension.Call{
		Operation: "process_request",
		Bridge:    &testBridge{},
	})
	if err == nil {
		t.Fatal("expected invoke to fail: guest imports a function that was not exposed")
	}
}
