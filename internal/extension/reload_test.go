// Package extension_test exercises hot reload semantics (RFC-0000 §13)
// against the real wazero host.
package extension_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/NICE-DEV226/Dun/internal/extension"
	"github.com/NICE-DEV226/Dun/internal/guest"
	"github.com/NICE-DEV226/Dun/internal/runtime/wazero"
)

func TestManagerReloadSwapsBinary(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	host, err := wazero.NewRuntime(logger)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close(context.Background())

	m := extension.NewManager(host, logger)
	m.SetPolicy("hdr", extension.NewSet("response.headers.write"))

	v1 := "default-src 'self'"
	v2 := "upgrade-insecure-requests"

	manifestV1 := &extension.Manifest{Name: "hdr", Version: "0.1.0", Caps: extension.Caps{Response: []string{"headers.write"}}}
	manifestV2 := &extension.Manifest{Name: "hdr", Version: "0.2.0", Caps: extension.Caps{Response: []string{"headers.write"}}}

	if err := m.Load("hdr", manifestV1, guest.SecurityHeaderModuleValue(guest.SecurityHeaderName, v1)); err != nil {
		t.Fatal(err)
	}
	if err := m.Activate("hdr"); err != nil {
		t.Fatal(err)
	}

	bridge := func() *reloadTestBridge { return &reloadTestBridge{} }

	// v1 active.
	if got := invokeHeader(t, m, bridge()); got != v1 {
		t.Fatalf("v1 header = %q, want %q", got, v1)
	}

	// Atomic activation: v2 replaces v1.
	if err := m.Reload("hdr", guest.SecurityHeaderModuleValue(guest.SecurityHeaderName, v2), manifestV2); err != nil {
		t.Fatal(err)
	}

	if got := invokeHeader(t, m, bridge()); got != v2 {
		t.Fatalf("v2 header = %q, want %q", got, v2)
	}

	// State must remain SERVING the whole time (no interruption).
	if st, _ := m.State("hdr"); st != extension.StateServing {
		t.Errorf("state = %s, want SERVING (no interruption)", st)
	}
}

func TestManagerReloadFailureKeepsOld(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	host, err := wazero.NewRuntime(logger)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close(context.Background())

	m := extension.NewManager(host, logger)
	m.SetPolicy("hdr", extension.NewSet("response.headers.write"))

	v1 := "default-src 'self'"
	manifestV1 := &extension.Manifest{Name: "hdr", Version: "0.1.0", Caps: extension.Caps{Response: []string{"headers.write"}}}
	if err := m.Load("hdr", manifestV1, guest.SecurityHeaderModuleValue(guest.SecurityHeaderName, v1)); err != nil {
		t.Fatal(err)
	}
	if err := m.Activate("hdr"); err != nil {
		t.Fatal(err)
	}

	// A broken artifact must not disturb the running v1.
	err = m.Reload("hdr", []byte("not a wasm binary"), &extension.Manifest{Name: "hdr", Version: "0.2.0"})
	if err == nil {
		t.Fatal("expected reload to fail for invalid artifact")
	}

	if got := invokeHeader(t, m, &reloadTestBridge{}); got != v1 {
		t.Fatalf("after failed reload, header = %q, want %q (old version stays active)", got, v1)
	}
}

func TestManagerReloadRejectsCapabilityOutsidePolicy(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	host, err := wazero.NewRuntime(logger)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close(context.Background())

	m := extension.NewManager(host, logger)
	// Policy only grants response.headers.write; the new manifest asks for
	// request.headers.read on top.
	m.SetPolicy("hdr", extension.NewSet("response.headers.write"))

	manifestV1 := &extension.Manifest{Name: "hdr", Version: "0.1.0", Caps: extension.Caps{Response: []string{"headers.write"}}}
	if err := m.Load("hdr", manifestV1, guest.SecurityHeaderModuleValue(guest.SecurityHeaderName, "v1")); err != nil {
		t.Fatal(err)
	}
	if err := m.Activate("hdr"); err != nil {
		t.Fatal(err)
	}

	manifestV2 := &extension.Manifest{
		Name:    "hdr",
		Version: "0.2.0",
		Caps: extension.Caps{
			Response: []string{"headers.write"},
			Request:  []string{"headers.read"},
		},
	}
	reloadErr := m.Reload("hdr", guest.SecurityHeaderModuleValue(guest.SecurityHeaderName, "v2"), manifestV2)
	if reloadErr == nil {
		t.Fatal("expected reload to be rejected at stage-1 validation")
	}
	if !strings.Contains(reloadErr.Error(), "outside its policy") {
		t.Errorf("expected policy rejection, got %v", reloadErr)
	}

	// Old version still serving.
	if got := invokeHeader(t, m, &reloadTestBridge{}); got != "v1" {
		t.Fatalf("after rejected reload, header = %q, want %q", got, "v1")
	}
}

func TestManagerReloadNotServing(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	host, err := wazero.NewRuntime(logger)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close(context.Background())

	m := extension.NewManager(host, logger)
	m.SetPolicy("hdr", extension.NewSet())

	// Load but do NOT Activate.
	if err := m.Load("hdr", &extension.Manifest{Name: "hdr", Version: "0.1.0"}, guest.MemoryModule(1)); err != nil {
		t.Fatal(err)
	}

	reloadErr := m.Reload("hdr", guest.MemoryModule(1), &extension.Manifest{Name: "hdr", Version: "0.2.0"})
	if reloadErr == nil {
		t.Fatal("expected reload to fail for a non-serving extension")
	}
	if !strings.Contains(reloadErr.Error(), "not serving") {
		t.Errorf("expected 'not serving' error, got %v", reloadErr)
	}
}

func TestManagerReloadUnknown(t *testing.T) {
	m := extension.NewManager(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	err := m.Reload("ghost", []byte("x"), &extension.Manifest{Name: "ghost", Version: "0.1.0"})
	if err != extension.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

type reloadTestBridge struct {
	headers map[string]string
}

func (t *reloadTestBridge) RequestHeader(string) string { return "" }
func (t *reloadTestBridge) SetResponseHeader(name, value string) {
	if t.headers == nil {
		t.headers = make(map[string]string)
	}
	t.headers[name] = value
}
func (t *reloadTestBridge) Log(int, string) {}

func invokeHeader(t *testing.T, m *extension.Manager, b *reloadTestBridge) string {
	t.Helper()
	if _, err := m.Invoke(context.Background(), "hdr", "process_request", nil, b); err != nil {
		t.Fatal(err)
	}
	return b.headers[guest.SecurityHeaderName]
}
