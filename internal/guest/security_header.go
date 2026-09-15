package guest

// SecurityHeaderModule is the hand-assembled equivalent of this WAT source:
//
//	(module
//	  ;; Dun Extension ABI v0 (Core-Wasm)
//	  (type (func (result i32)))                    ;; type 0: () -> i32
//	  (type (func (param i32 i32 i32 i32) (result i32))) ;; type 1: header set
//	  (import "dun" "response.headers.write"
//	    (func $setResponseHeader (type 1)))
//	  (memory (export "memory") 1)
//	  (data (i32.const 0)  "X-Security-Policy")     ;; name @ 0
//	  (data (i32.const 32) "default-src 'self'")    ;; value @ 32
//	  (func $process (type 0) (result i32)
//	    i32.const 0     ;; name offset
//	    i32.const 17    ;; name length
//	    i32.const 32    ;; value offset
//	    i32.const 18    ;; value length
//	    call $setResponseHeader
//	    drop
//	    i32.const 0     ;; CONTINUE
//	  )
//	  (export "dun_abi_version_0_1" (func $process))
//	  (export "dun_process_request" (func $process))
//	)
//
// Decision codes returned by dun_process_request:
//
//	0 = CONTINUE  1 = RESPOND  2 = REJECT  3 = ERROR
const (
	SecurityHeaderName  = "X-Security-Policy"
	SecurityHeaderValue = "default-src 'self'"
)

// SecurityHeaderDataOffset is the heap offset for the header name string.
const SecurityHeaderDataOffset = 0

// SecurityHeaderValueOffset is the heap offset for the header value string.
const SecurityHeaderValueOffset = 32

// SecurityHeaderModule builds the security-header guest module bytes.
func SecurityHeaderModule() []byte {
	w := &wasmWriter{}

	// 1. Type section: two function types.
	fnType0 := funcType(nil, []byte{0x7f})                            // () -> i32
	fnType1 := funcType([]byte{0x7f, 0x7f, 0x7f, 0x7f}, []byte{0x7f}) // (i32,i32,i32,i32)->i32
	w.addSection(1, buildTypeSection([][]byte{fnType0, fnType1}))

	// 2. Import section: response.headers.write from module "dun" (type 1).
	w.addSection(2, importFunc(ABIModuleName, "response.headers.write", 1))

	// 3. Function section: module function $process (type 0).
	w.addSection(3, funcSection([]uint32{0}))

	// 4. Memory section: one page.
	w.addSection(5, memorySection())

	// 5. Export section: ABI marker + process handler, both func index 1.
	w.addSection(7, exportSection([]exportEntry{
		{name: ABIVersionMarker, funcIdx: 1},
		{name: "dun_process_request", funcIdx: 1},
	}))

	// 6. Code section: $process body.
	instr := i32Const(SecurityHeaderDataOffset)
	instr = append(instr, i32Const(int32(len(SecurityHeaderName)))...)
	instr = append(instr, i32Const(SecurityHeaderValueOffset)...)
	instr = append(instr, i32Const(int32(len(SecurityHeaderValue)))...)
	instr = append(instr, 0x10, 0x00) // call func index 0 (the import)
	instr = append(instr, 0x1a)       // drop
	instr = append(instr, i32Const(0)...)
	w.addSection(10, codeSection(body(instr)))

	// 7. Data section: header name and value.
	data := vecCount(2) // two data segments
	data = append(data, dataActive(SecurityHeaderDataOffset, []byte(SecurityHeaderName))...)
	data = append(data, dataActive(SecurityHeaderValueOffset, []byte(SecurityHeaderValue))...)
	w.addSection(11, data)

	return w.Bytes()
}

func funcSection(typeIndices []uint32) []byte {
	content := vecCount(len(typeIndices))
	for _, ti := range typeIndices {
		content = append(content, uLEB(ti)...)
	}
	return content
}

func memorySection() []byte {
	content := vecCount(1)
	content = append(content, 0x00) // no maximum (flags)
	content = append(content, 0x01) // 1 page (64 KiB)
	return content
}

func codeSection(entry []byte) []byte {
	return append([]byte{0x01}, entry...)
}

// ABIModuleName is the import namespace host functions live under.
const ABIModuleName = "dun"

// ABIVersionMarker is the exported marker function every Dun plugin binary
// must expose (checked at VALIDATE per RFC-0000 §18.1).
const ABIVersionMarker = "dun_abi_version_0_1"
