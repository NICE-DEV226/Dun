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

// ProcessRequest is the default handler for a plugin that continues.
//
//go:export dun_process_request
func ProcessRequest() uint32 { return Continue }
