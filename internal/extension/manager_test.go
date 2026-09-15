package extension

import (
	"context"
	"log/slog"
	"testing"
)

type fakeHost struct {
	abiOK bool
	mod   Module
}

func (f *fakeHost) Load(_ context.Context, _ []byte, _ LoadOptions) (Module, error) {
	return f.mod, nil
}

func (f *fakeHost) CheckABI(_ context.Context, _ []byte) error {
	if !f.abiOK {
		return &TransitionError{From: StateValidate, To: StateLoad}
	}
	return nil
}

type fakeModule struct{}

func (f *fakeModule) Invoke(_ context.Context, _ Call) (Result, error) {
	return Result{Decision: "continue"}, nil
}

func (f *fakeModule) Close(_ context.Context) error { return nil }

func testManifest() *Manifest {
	return &Manifest{
		Name:    "test",
		Version: "0.1.0",
		Caps: Caps{
			Request: []string{"headers.read"},
		},
	}
}

func TestManagerLoadAndInvoke(t *testing.T) {
	host := &fakeHost{abiOK: true, mod: &fakeModule{}}
	m := NewManager(host, slog.Default())
	m.SetPolicy("test", NewSet("request.headers.read"))

	if err := m.Load("test", testManifest(), []byte("wasm")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	state, err := m.State("test")
	if err != nil {
		t.Fatal(err)
	}
	if state != StateCompile {
		t.Errorf("expected COMPILE (not yet activated), got %s", state)
	}

	if err := m.Activate("test"); err != nil {
		t.Fatal(err)
	}

	res, err := m.Invoke(context.Background(), "test", "process_request", nil, nil)
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	if res.Decision != "continue" {
		t.Errorf("expected continue, got %s", res.Decision)
	}
}

func TestManagerLoadNoPolicy(t *testing.T) {
	m := NewManager(&fakeHost{abiOK: true, mod: &fakeModule{}}, slog.Default())
	err := m.Load("test", testManifest(), []byte("wasm"))
	if err == nil {
		t.Fatal("expected error for missing policy")
	}
}

func TestManagerLoadCapabilityViolation(t *testing.T) {
	host := &fakeHost{abiOK: true, mod: &fakeModule{}}
	m := NewManager(host, slog.Default())
	// Policy does NOT include request.headers.read.
	m.SetPolicy("test", NewSet("log.info"))

	err := m.Load("test", testManifest(), []byte("wasm"))
	if err == nil {
		t.Fatal("expected capability violation error")
	}
}

func TestManagerLoadABIFailure(t *testing.T) {
	m := NewManager(&fakeHost{abiOK: false, mod: &fakeModule{}}, slog.Default())
	m.SetPolicy("test", NewSet("request.headers.read"))
	err := m.Load("test", testManifest(), []byte("wasm"))
	if err == nil {
		t.Fatal("expected ABI failure error")
	}
}

func TestManagerCloseAndRemove(t *testing.T) {
	m := NewManager(&fakeHost{abiOK: true, mod: &fakeModule{}}, slog.Default())
	m.SetPolicy("test", NewSet("request.headers.read"))
	if err := m.Load("test", testManifest(), []byte("wasm")); err != nil {
		t.Fatal(err)
	}
	if err := m.Activate("test"); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(context.Background(), "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.State("test"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestManagerInvokeNotServing(t *testing.T) {
	m := NewManager(&fakeHost{abiOK: true, mod: &fakeModule{}}, slog.Default())
	m.SetPolicy("test", NewSet("request.headers.read"))
	if err := m.Load("test", testManifest(), []byte("wasm")); err != nil {
		t.Fatal(err)
	}
	// Not activated, so Invoke should fail.
	_, err := m.Invoke(context.Background(), "test", "process_request", nil, nil)
	if err == nil {
		t.Fatal("expected error for non-serving extension")
	}
}

func TestManagerList(t *testing.T) {
	m := NewManager(&fakeHost{abiOK: true, mod: &fakeModule{}}, slog.Default())
	m.SetPolicy("a", NewSet("request.headers.read"))
	m.SetPolicy("b", NewSet("request.headers.read"))
	if err := m.Load("a", testManifest(), nil); err != nil {
		t.Fatal(err)
	}
	if err := m.Load("b", testManifest(), nil); err != nil {
		t.Fatal(err)
	}
	if got := m.List(); len(got) != 2 {
		t.Errorf("expected 2 extensions, got %d", len(got))
	}
}
