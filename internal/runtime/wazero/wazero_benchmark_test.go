package wazero

import (
	"context"
	"testing"

	"github.com/NICE-DEV226/Dun/internal/extension"
	"github.com/NICE-DEV226/Dun/internal/guest"
)

func BenchmarkInvokeSecurityHeader(b *testing.B) {
	runtime, err := NewRuntime(nil)
	if err != nil {
		b.Fatal(err)
	}
	defer runtime.Close(context.Background())

	module, err := runtime.Load(context.Background(), guest.SecurityHeaderModule(), extension.LoadOptions{
		Capabilities: extension.NewSet(string(extension.CapResponseHeadersWrite)),
	})
	if err != nil {
		b.Fatal(err)
	}
	defer module.Close(context.Background())

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, err := module.Invoke(context.Background(), extension.Call{
			Operation: "process_request",
			Bridge:    &testBridge{},
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkInvokePooledSecurityHeader measures the §15.2 pooled
// instantiation model against the same guest: instances are checked out,
// invoked and returned with a memory-snapshot reset instead of being
// instantiated and closed per call.
func BenchmarkInvokePooledSecurityHeader(b *testing.B) {
	runtime, err := NewRuntime(nil)
	if err != nil {
		b.Fatal(err)
	}
	defer runtime.Close(context.Background())

	module, err := runtime.Load(context.Background(), guest.SecurityHeaderModule(), extension.LoadOptions{
		Capabilities: extension.NewSet(string(extension.CapResponseHeadersWrite)),
		Limits:       extension.ResourceLimits{MaxInstances: 4},
	})
	if err != nil {
		b.Fatal(err)
	}
	defer module.Close(context.Background())

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, err := module.Invoke(context.Background(), extension.Call{
			Operation: "process_request",
			Bridge:    &testBridge{},
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}
