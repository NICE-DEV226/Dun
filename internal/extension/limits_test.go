// Package extension_test exercises the extension contract from outside,
// including the real wazero host, without creating import cycles.
package extension_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/NICE-DEV226/Dun/internal/extension"
	"github.com/NICE-DEV226/Dun/internal/guest"
	"github.com/NICE-DEV226/Dun/internal/runtime/wazero"
)

func TestParseSize(t *testing.T) {
	cases := []struct {
		in   string
		want uint64
	}{
		{"1024", 1024},
		{"0", 0},
		{"64KiB", 64 * 1024},
		{"1MiB", 1 << 20},
		{"2GiB", 2 << 30},
		{"  8MiB  ", 8 << 20},
	}
	for _, c := range cases {
		got, err := extension.ParseSize(c.in)
		if err != nil {
			t.Fatalf("ParseSize(%q): %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("ParseSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}

	for _, bad := range []string{"", "abc", "MiB", "-5", "1MB", "manyMiB"} {
		if _, err := extension.ParseSize(bad); err == nil {
			t.Errorf("ParseSize(%q): expected error", bad)
		}
	}
}

func TestParseResourceLimits(t *testing.T) {
	rl, err := extension.ParseResourceLimits(extension.Limits{
		Memory:    "64MiB",
		Timeout:   "50ms",
		BodySize:  "1MiB",
		Instances: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rl.MaxMemoryBytes != 64<<20 {
		t.Errorf("MaxMemoryBytes = %d, want %d", rl.MaxMemoryBytes, 64<<20)
	}
	if rl.ExecutionTimeout != 50*time.Millisecond {
		t.Errorf("ExecutionTimeout = %v", rl.ExecutionTimeout)
	}
	if rl.MaxBodyBytes != 1<<20 {
		t.Errorf("MaxBodyBytes = %d, want %d", rl.MaxBodyBytes, 1<<20)
	}
	if rl.MaxInstances != 8 {
		t.Errorf("MaxInstances = %d, want 8", rl.MaxInstances)
	}

	if _, err := extension.ParseResourceLimits(extension.Limits{Memory: "nope"}); err == nil {
		t.Error("expected error for bad memory size")
	}
	if _, err := extension.ParseResourceLimits(extension.Limits{Timeout: "banana"}); err == nil {
		t.Error("expected error for bad timeout")
	}
	if _, err := extension.ParseResourceLimits(extension.Limits{Instances: -1}); err == nil {
		t.Error("expected error for negative instances")
	}
}

// recordingHost captures the LoadOptions it receives so tests can assert
// that policy limits flow through the manager into the runtime.
type recordingHost struct {
	lastCaps   extension.Set
	lastLimits extension.ResourceLimits
	module     extension.Module
}

func (h *recordingHost) Load(_ context.Context, _ []byte, opts extension.LoadOptions) (extension.Module, error) {
	h.lastCaps = opts.Capabilities
	h.lastLimits = opts.Limits
	if h.module == nil {
		return &okModule{}, nil
	}
	return h.module, nil
}

func (h *recordingHost) CheckABI(_ context.Context, _ []byte) error { return nil }

type okModule struct{}

func (o *okModule) Invoke(_ context.Context, _ extension.Call) (extension.Result, error) {
	return extension.Result{Decision: "continue"}, nil
}
func (o *okModule) Close(_ context.Context) error { return nil }

func TestManagerLimitsFlowToRuntime(t *testing.T) {
	host := &recordingHost{}
	m := extension.NewManager(host, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.SetPolicy("p", extension.NewSet("request.headers.read"))
	want := extension.ResourceLimits{MaxMemoryBytes: 1 << 20, ExecutionTimeout: 5 * time.Millisecond}
	m.SetLimits("p", want)

	manifest := &extension.Manifest{Name: "p", Version: "0.1.0", Caps: extension.Caps{Request: []string{"headers.read"}}}
	if err := m.Load("p", manifest, []byte("wasm")); err != nil {
		t.Fatal(err)
	}
	if host.lastLimits != want {
		t.Errorf("limits = %+v, want %+v", host.lastLimits, want)
	}
	if !host.lastCaps.Has(extension.Capability("request.headers.read")) {
		t.Errorf("caps = %v, want request.headers.read", host.lastCaps)
	}
}

// TestManagerFailureTelemetry proves the full failure chain on the real
// wazero host: a guest trap surfaces as an InvocationError with
// FailureTrap, and the manager records it in per-plugin telemetry — the
// core stays alive afterwards (RFC-0000 §14).
func TestManagerFailureTelemetry(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	host, err := wazero.NewRuntime(logger)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close(context.Background())

	m := extension.NewManager(host, logger)
	m.SetPolicy("trap", extension.NewSet())

	err = m.Load("trap", &extension.Manifest{Name: "trap", Version: "0.1.0"}, guest.TrapModule())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Activate("trap"); err != nil {
		t.Fatal(err)
	}

	_, err = m.Invoke(context.Background(), "trap", "process_request", nil, nil)
	if err == nil {
		t.Fatal("expected trap error")
	}
	var ive *extension.InvocationError
	if !errors.As(err, &ive) {
		t.Fatalf("expected InvocationError, got %T", err)
	}
	if ive.Kind != extension.FailureTrap {
		t.Errorf("kind = %s, want trap", ive.Kind)
	}

	stats := m.Failures("trap")
	if stats.Total != 1 {
		t.Errorf("Total = %d, want 1", stats.Total)
	}
	if stats.ByName[extension.FailureTrap] != 1 {
		t.Errorf("trap count = %d, want 1", stats.ByName[extension.FailureTrap])
	}

	// Second invocation confirms the counter increments and the core is
	// still serving (isolation, keep-core-alive).
	_, _ = m.Invoke(context.Background(), "trap", "process_request", nil, nil)
	stats = m.Failures("trap")
	if stats.Total != 2 {
		t.Errorf("Total = %d, want 2", stats.Total)
	}

	if st, _ := m.State("trap"); st != extension.StateServing {
		t.Errorf("state = %s, want SERVING (core kept alive)", st)
	}
}

// TestManagerTimeoutTelemetry proves execution-timeout classification
// flows through the manager into per-kind telemetry.
func TestManagerTimeoutTelemetry(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	host, err := wazero.NewRuntime(logger)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close(context.Background())

	m := extension.NewManager(host, logger)
	m.SetPolicy("loop", extension.NewSet())
	m.SetLimits("loop", extension.ResourceLimits{ExecutionTimeout: 30 * time.Millisecond})

	if err := m.Load("loop", &extension.Manifest{Name: "loop", Version: "0.1.0"}, guest.InfiniteLoopModule()); err != nil {
		t.Fatal(err)
	}
	if err := m.Activate("loop"); err != nil {
		t.Fatal(err)
	}

	_, err = m.Invoke(context.Background(), "loop", "process_request", nil, nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	var ive *extension.InvocationError
	if !errors.As(err, &ive) {
		t.Fatalf("expected InvocationError, got %T", err)
	}
	if ive.Kind != extension.FailureTimeout {
		t.Errorf("kind = %s, want timeout", ive.Kind)
	}

	stats := m.Failures("loop")
	if stats.ByName[extension.FailureTimeout] != 1 {
		t.Errorf("timeout count = %d, want 1", stats.ByName[extension.FailureTimeout])
	}
}
