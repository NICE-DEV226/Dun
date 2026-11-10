//go:build tinygo

// Package dun provides a minimal TinyGo SDK for Dun Core-Wasm v0.
package dun

import "unsafe"

// Decision codes returned by ProcessRequest.
const (
	Continue uint32 = iota
	Respond
	Reject
	Error
)

// ABI marker export. TinyGo users should export ABIExport from their module.
//
//go:export dun_abi_version_0_1
func ABIExport() uint32 { return 1 }

// HeaderWrite calls the capability-gated Dun host function.
//
//go:wasmimport dun response.headers.write
func headerWrite(namePtr, nameLen, valuePtr, valueLen uint32) uint32

// WriteResponseHeader writes a response header from guest memory.
func WriteResponseHeader(name, value string) uint32 {
	return headerWrite(uint32(uintptr(unsafe.Pointer(unsafe.StringData(name)))), uint32(len(name)), uint32(uintptr(unsafe.Pointer(unsafe.StringData(value)))), uint32(len(value)))
}

// ReadAbsent is the sentinel returned by ReadRequestHeader when the
// header is absent (or the call is invalid). As an i32 the guest sees -1.
const ReadAbsent = ^uint32(0)

//go:wasmimport dun request.headers.read
func headerRead(namePtr, nameLen, outPtr, outCap uint32) uint32

// ReadRequestHeader reads a request header into buf, which must live in
// guest memory. It returns the full value length — retry with a larger
// buf when the result exceeds len(buf) — or ReadAbsent when the header
// is absent. A present-but-empty header returns 0.
func ReadRequestHeader(name string, buf []byte) uint32 {
	if len(buf) == 0 {
		return ReadAbsent
	}
	return headerRead(
		uint32(uintptr(unsafe.Pointer(unsafe.StringData(name)))), uint32(len(name)),
		uint32(uintptr(unsafe.Pointer(unsafe.SliceData(buf)))), uint32(len(buf)),
	)
}

// ProcessRequest is the default handler for a plugin that continues.
//
//go:export dun_process_request
func ProcessRequest() uint32 { return Continue }
