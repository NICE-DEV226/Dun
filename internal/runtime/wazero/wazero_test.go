package wazero

import (
	"context"
	"testing"

	"github.com/NICE-DEV226/Dun/internal/extension"
	"github.com/tetratelabs/wazero"
)

// buildABIModule builds a minimal valid wasm module.
// When exportMarker is true, the module exports a function named
// dun_abi_version_0_1 implementing () -> i32 { return 1 }.
//
// Module layout:
//
//	(module
//	  (func (export "dun_abi_version_0_1") (result i32)
//	    i32.const 1
//	  )
//	)
func buildABIModule(t *testing.T, exportMarker bool) []byte {
	t.Helper()

	// -- Type section (0x01) --
	typeBody := []byte{0x60, 0x00, 0x01, 0x7f} // () -> i32
	typeSection := section(0x01, append([]byte{0x01}, typeBody...))

	// -- Function section (0x03) --
	// vec of type indices, one entry: type 0
	funcSection := section(0x03, []byte{0x01, 0x00})

	// -- Code section (0x0a) --
	// body: 0 locals, i32.const 1, end
	body := []byte{0x00, 0x41, 0x01, 0x0b}
	entry := prependLen(body) // size prefix
	codeSection := section(0x0a, append([]byte{0x01}, entry...))

	// -- Export section (0x07) --
	var exports []byte
	if exportMarker {
		name := []byte(ABIVersionMarker)
		entry := []byte{0x00, 0x00} // function export, index 0
		content := append([]byte{0x01, byte(len(name))}, name...)
		content = append(content, entry...)
		exports = section(0x07, content)
	} else {
		exports = section(0x07, []byte{
			0x01, 0x05,
			'o', 't', 'h', 'e', 'r',
			0x00, 0x00,
		})
	}

	module := append([]byte{}, 0x00, 0x61, 0x73, 0x6d) // \0asm
	module = append(module, 0x01, 0x00, 0x00, 0x00)    // version 1
	module = append(module, typeSection...)
	module = append(module, funcSection...)
	module = append(module, exports...)
	module = append(module, codeSection...)

	// Sanity: validate it compiles with wazero before returning.
	rt := wazero.NewRuntime(context.Background())
	defer rt.Close(context.Background())
	if _, err := rt.CompileModule(context.Background(), module); err != nil {
		t.Fatalf("test module failed to compile: %v", err)
	}
	return module
}

// section wraps a section id plus content with a length prefix.
func section(id byte, content []byte) []byte {
	out := []byte{id}
	out = append(out, uint32LEB(uint32(len(content)))...)
	out = append(out, content...)
	return out
}

// uint32LEB encodes a uint32 as an unsigned LEB128 varint
// (the format wasm uses for lengths and vec counts).
func uint32LEB(v uint32) []byte {
	var out []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if v == 0 {
			return out
		}
	}
}

// prependLen prefixes content with its byte length as an LEB128 u32.
func prependLen(content []byte) []byte {
	return append(uint32LEB(uint32(len(content))), content...)
}

func TestCheckABIValidModule(t *testing.T) {
	r, err := NewRuntime(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close(context.Background())

	mod := buildABIModule(t, true)
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

	mod := buildABIModule(t, false)
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

	if _, err := r.Load(context.Background(), []byte("not wasm")); err == nil {
		t.Fatal("expected error for invalid wasm bytes")
	}
}

func TestLoadProducesModule(t *testing.T) {
	r, err := NewRuntime(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close(context.Background())

	mod := buildABIModule(t, true)
	m, err := r.Load(context.Background(), mod)
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	defer func() {
		if err := m.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	}()

	res, err := m.Invoke(context.Background(), extension.Call{Operation: "process_request"})
	if err != nil {
		t.Fatalf("unexpected invoke error: %v", err)
	}
	if res.Decision != "continue" {
		t.Errorf("expected continue decision, got %q", res.Decision)
	}
}

func TestNewRuntimeIsolated(t *testing.T) {
	r1, err := NewRuntime(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r1.Close(context.Background())

	r2, err := NewRuntime(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Close(context.Background())

	r1mod := buildABIModule(t, true)
	r2mod := buildABIModule(t, true)

	// Both runtimes can load the same bytes independently.
	if _, err := r1.Load(context.Background(), r1mod); err != nil {
		t.Fatal(err)
	}
	if _, err := r2.Load(context.Background(), r2mod); err != nil {
		t.Fatal(err)
	}
}
