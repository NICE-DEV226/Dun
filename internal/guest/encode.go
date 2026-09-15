// Package guest builds small, hand-assembled WebAssembly Core guest modules
// for Dun's v0 extension contract.
//
// The modules it produces are intentionally minimal — they exist so that the
// extension boundary can be proven end-to-end in tests and examples without
// requiring a Wasm toolchain (wat2wasm / TinyGo / Rust) in the build
// environment. Every module is equivalent to a human-readable WAT source,
// which is documented alongside the generated binary.
//
// Byte layout matches the WebAssembly Core Specification 1.0: sections are
// emitted in canonical order and all sizes/counts are unsigned LEB128.
package guest

import (
	"bytes"
	"fmt"
)

// wasmWriter assembles a wasm binary section by section.
type wasmWriter struct {
	sections [12][]byte
}

func (w *wasmWriter) addSection(id byte, content []byte) {
	w.sections[id] = append(w.sections[id], sectionBytes(id, content)...)
}

// Bytes returns the fully assembled wasm module in canonical section order.
func (w *wasmWriter) Bytes() []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00})
	for _, s := range w.sections {
		buf.Write(s)
	}
	return buf.Bytes()
}

func sectionBytes(id byte, content []byte) []byte {
	out := []byte{id}
	out = append(out, uLEB(uint32(len(content)))...)
	return append(out, content...)
}

// uLEB encodes v as an unsigned LEB128 varint.
func uLEB(v uint32) []byte {
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

// sLEB encodes v as a signed LEB128 varint (used for i32.const immediates).
func sLEB(v int32) []byte {
	var out []byte
	more := true
	for more {
		b := byte(v & 0x7f)
		v >>= 7
		if (v == 0 && b&0x40 == 0) || (v == -1 && b&0x40 != 0) {
			more = false
		} else {
			b |= 0x80
		}
		out = append(out, b)
	}
	return out
}

func vecCount(n int) []byte { return uLEB(uint32(n)) }

func nameString(s string) []byte {
	out := uLEB(uint32(len(s)))
	return append(out, s...)
}

// funcType encodes a function type section entry. params and results are
// wasm value types (e.g. 0x7f for i32).
func funcType(params, results []byte) []byte {
	out := []byte{0x60}
	out = append(out, vecCount(len(params))...)
	out = append(out, params...)
	out = append(out, vecCount(len(results))...)
	return append(out, results...)
}

// i32Const encodes an i32.const instruction.
func i32Const(v int32) []byte { return append([]byte{0x41}, sLEB(v)...) }

// typeIndex returns a function-type section entry for `fnTypeID`.
func buildTypeSection(fnTypes [][]byte) []byte {
	content := vecCount(len(fnTypes))
	for _, t := range fnTypes {
		content = append(content, t...)
	}
	return content
}

// importFunc encodes (module, field, function type index) for the import section.
func importFunc(module, field string, typeIndex uint32) []byte {
	content := vecCount(1)
	content = append(content, nameString(module)...)
	content = append(content, nameString(field)...)
	content = append(content, 0x00) // kind: function
	content = append(content, uLEB(typeIndex)...)
	return content
}

// exportEntry describes one exported function.
type exportEntry struct {
	name    string
	funcIdx uint32
}

// exportSection encodes an export section with multiple function exports.
func exportSection(entries []exportEntry) []byte {
	content := vecCount(len(entries))
	for _, e := range entries {
		content = append(content, nameString(e.name)...)
		content = append(content, 0x00) // kind: function
		content = append(content, uLEB(e.funcIdx)...)
	}
	return content
}

// body encodes a code-section entry for a function body.
// instr is the function's instruction sequence.
func body(instr []byte) []byte {
	// locals vector is empty for our generated modules.
	code := append([]byte{0x00}, instr...)
	code = append(code, 0x0b) // end
	out := uLEB(uint32(len(code)))
	return append(out, code...)
}

// dataActive encodes an active data segment targeting memory index 0.
// In the canonical binary format, flags=0x00 means active in memory 0 (no
// memidx field is encoded — memory 0 is implicit).
func dataActive(offset int32, b []byte) []byte {
	out := []byte{0x00} // flags: active in memory 0
	out = append(out, i32Const(offset)...)
	out = append(out, 0x0b) // end of const expr
	out = append(out, vecCount(len(b))...)
	return append(out, b...)
}

// ValidateWasm performs a lightweight sanity check on a generated module:
// it must start with the magic header.
func ValidateWasm(b []byte) error {
	if len(b) < 8 || !bytes.Equal(b[:4], []byte{0x00, 0x61, 0x73, 0x6d}) {
		return fmt.Errorf("invalid wasm magic")
	}
	if !bytes.Equal(b[4:8], []byte{0x01, 0x00, 0x00, 0x00}) {
		return fmt.Errorf("unsupported wasm version")
	}
	return nil
}
