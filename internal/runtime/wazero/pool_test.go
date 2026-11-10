package wazero

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NICE-DEV226/Dun/internal/extension"
	"github.com/NICE-DEV226/Dun/internal/guest"
)

// poolTestLimits enables pooling with the given capacity.
func poolTestLimits(max int) extension.ResourceLimits {
	return extension.ResourceLimits{MaxInstances: max}
}

func TestPooledInvokeSetsHeader(t *testing.T) {
	rt, err := NewRuntime(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	mod, err := rt.Load(context.Background(), guest.SecurityHeaderModule(), extension.LoadOptions{
		Capabilities: extension.NewSet(string(extension.CapResponseHeadersWrite)),
		Limits:       poolTestLimits(4),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mod.Close(context.Background())

	pm, ok := mod.(*pooledModule)
	if !ok {
		t.Fatalf("expected *pooledModule, got %T", mod)
	}

	// Sequential reuse: same instance must serve repeated invocations.
	for i := 0; i < 10; i++ {
		bridge := &testBridge{}
		res, err := pm.Invoke(context.Background(), extension.Call{
			Operation: "process_request",
			Bridge:    bridge,
		})
		if err != nil {
			t.Fatalf("invoke %d: %v", i, err)
		}
		if res.Decision != "continue" {
			t.Fatalf("invoke %d: decision %q", i, res.Decision)
		}
		if got := bridge.headers["X-Security-Policy"]; got != "default-src 'self'" {
			t.Fatalf("invoke %d: header %q", i, got)
		}
	}

	pm.mu.Lock()
	idle := len(pm.idle)
	live := pm.live
	pm.mu.Unlock()
	if live > 4 {
		t.Fatalf("live instances %d exceed pool max 4", live)
	}
	if idle == 0 {
		t.Fatal("expected at least one idle instance after sequential invokes")
	}
}

func TestPooledConcurrentInvoke(t *testing.T) {
	rt, err := NewRuntime(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	mod, err := rt.Load(context.Background(), guest.SecurityHeaderModule(), extension.LoadOptions{
		Capabilities: extension.NewSet(string(extension.CapResponseHeadersWrite)),
		Limits:       poolTestLimits(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mod.Close(context.Background())

	done := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() {
			bridge := &testBridge{}
			_, err := mod.Invoke(context.Background(), extension.Call{
				Operation: "process_request",
				Bridge:    bridge,
			})
			done <- err
		}()
	}
	for i := 0; i < 8; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

func TestPooledTimeoutDiscardsInstance(t *testing.T) {
	rt, err := NewRuntime(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	mod, err := rt.Load(context.Background(), guest.InfiniteLoopModule(), extension.LoadOptions{
		Limits: extension.ResourceLimits{
			MaxInstances:     2,
			ExecutionTimeout: 50 * time.Millisecond,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mod.Close(context.Background())

	if _, ok := mod.(*pooledModule); !ok {
		t.Fatalf("expected pooled module for loop guest, got %T", mod)
	}

	// The infinite-loop guest traps via timeout; the instance must not
	// poison the pool — subsequent invokes keep timing out cleanly.
	for i := 0; i < 3; i++ {
		start := time.Now()
		_, err := mod.Invoke(context.Background(), extension.Call{Operation: "process_request"})
		if err == nil {
			t.Fatal("expected timeout error")
		}
		var ive *extension.InvocationError
		if !errors.As(err, &ive) || ive.Kind != extension.FailureTimeout {
			t.Fatalf("invoke %d: kind = %v, err = %v", i, ive, err)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("invoke %d took %v", i, elapsed)
		}
	}
}

func TestPoolableArtifact(t *testing.T) {
	cases := []struct {
		name string
		art  []byte
		want bool
	}{
		{"security header", guest.SecurityHeaderModule(), true},
		{"trap", guest.TrapModule(), true},
		{"memory", guest.MemoryModule(1), true},
		{"empty", nil, false},
		{"garbage", []byte("not wasm"), false},
	}
	for _, tc := range cases {
		if got := poolableArtifact(tc.art); got != tc.want {
			t.Errorf("%s: poolableArtifact = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A module that declares a mutable global is NOT poolable: the public
// wazero API cannot enumerate or reset globals between uses.
func TestPoolableArtifactRejectsGlobals(t *testing.T) {
	if poolableArtifact(guest.GlobalModule()) {
		t.Fatal("module with global section must not be poolable")
	}
}
