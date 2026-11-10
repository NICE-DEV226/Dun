package wazero

import (
	"context"
	"testing"

	"github.com/NICE-DEV226/Dun/internal/extension"
	"github.com/NICE-DEV226/Dun/internal/guest"
)

func TestAuthModule(t *testing.T) {
	rt, err := NewRuntime(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	mod, err := rt.Load(context.Background(), guest.AuthModule(), extension.LoadOptions{
		Capabilities: extension.NewSet(string(extension.CapRequestHeadersRead)),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mod.Close(context.Background())

	cases := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{"bearer token", map[string]string{"Authorization": "Bearer abc123"}, "continue"},
		{"basic scheme", map[string]string{"Authorization": "Basic abc"}, "reject"},
		{"no auth header", map[string]string{}, "reject"},
		{"empty value", map[string]string{"Authorization": ""}, "reject"},
	}
	for _, tc := range cases {
		bridge := &testBridge{reqHeaders: tc.headers}
		res, err := mod.Invoke(context.Background(), extension.Call{
			Operation: "process_request",
			Bridge:    bridge,
		})
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if res.Decision != tc.want {
			t.Errorf("%s: decision %q, want %q", tc.name, res.Decision, tc.want)
		}
	}
}

func TestCORSModule(t *testing.T) {
	rt, err := NewRuntime(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	mod, err := rt.Load(context.Background(), guest.CORSModule(), extension.LoadOptions{
		Capabilities: extension.NewSet(string(extension.CapResponseHeadersWrite)),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mod.Close(context.Background())

	bridge := &testBridge{}
	res, err := mod.Invoke(context.Background(), extension.Call{
		Operation: "process_request",
		Bridge:    bridge,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != "continue" {
		t.Fatalf("decision %q, want continue", res.Decision)
	}
	want := map[string]string{
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Methods": "GET, POST, OPTIONS",
		"Access-Control-Allow-Headers": "Content-Type, Authorization",
	}
	for k, v := range want {
		if got := bridge.headers[k]; got != v {
			t.Errorf("header %q = %q, want %q", k, got, v)
		}
	}
}
