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
