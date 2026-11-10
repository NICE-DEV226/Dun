package guest

// HeaderReadModule is the hand-assembled equivalent of this WAT source:
//
//	(module
//	  (type $t0 (func (result i32)))
//	  (type $t1 (func (param i32 i32 i32 i32) (result i32)))
//	  (import "dun" "request.headers.read" (func $read (type 1)))
//	  (memory (export "memory") 1)
//	  (data (i32.const 0) "X-Denied")          ;; header name @ 0, len 8
//	  (func $marker (type 0) (result i32)
//	    i32.const 1)
//	  (func $process (type 0) (result i32)
//	    i32.const 0                          ;; name offset
//	    i32.const 8                          ;; name length
//	    i32.const 256                        ;; out offset
//	    i32.const 128                        ;; out capacity
//	    call $read                           ;; -> i32 (len or -1)
//	    i32.const -1
//	    i32.eq                               ;; absent?
//	    if (result i32)
//	      i32.const 0                        ;; absent -> CONTINUE
//	    else
//	      i32.const 2                        ;; present -> REJECT
//	    end)
//	  (export "dun_abi_version_0_1" (func $marker))
//	  (export "dun_process_request" (func $process))
//	)
//
// It exercises the request.headers.read host import end-to-end: the guest
// rejects any request carrying an X-Denied header.
const (
	// HeaderReadName is the request header the fixture looks up.
	HeaderReadName = "X-Denied"
	// HeaderReadOutOffset is where the host writes the header value.
	HeaderReadOutOffset = 256
	// HeaderReadOutCap is the buffer capacity passed to the host.
	HeaderReadOutCap = 128
)

// HeaderReadModule builds the header-read guest module bytes.
func HeaderReadModule() []byte {
	w := &wasmWriter{}

	// 1. Type section: () -> i32 and (i32,i32,i32,i32) -> i32.
	fnType0 := funcType(nil, []byte{0x7f})
	fnType1 := funcType([]byte{0x7f, 0x7f, 0x7f, 0x7f}, []byte{0x7f})
	w.addSection(1, buildTypeSection([][]byte{fnType0, fnType1}))

	// 2. Import section: request.headers.read (func index 0, type 1).
	w.addSection(2, importFunc(ABIModuleName, "request.headers.read", 1))

	// 3. Function section: marker and process handler (both type 0,
	//    func indices 1 and 2).
	w.addSection(3, funcSection([]uint32{0, 0}))

	// 4. Memory section: one page.
	w.addSection(5, memorySectionPages(1))

	// 5. Export section.
	w.addSection(7, exportSection([]exportEntry{
		{name: ABIVersionMarker, funcIdx: 1},
		{name: "dun_process_request", funcIdx: 2},
	}))

	// 6. Code section.
	marker := i32Const(1)

	var instr []byte
	instr = append(instr, i32Const(0)...)                          // name offset
	instr = append(instr, i32Const(int32(len(HeaderReadName)))...) // name length
	instr = append(instr, i32Const(HeaderReadOutOffset)...)        // out offset
	instr = append(instr, i32Const(HeaderReadOutCap)...)           // out capacity
	instr = append(instr, 0x10, 0x00)                              // call func 0 (import)
	instr = append(instr, i32Const(-1)...)                         // sentinel
	instr = append(instr, 0x46)                                    // i32.eq
	instr = append(instr, 0x04, 0x7f)                              // if (result i32)
	instr = append(instr, i32Const(0)...)                          // then: CONTINUE
	instr = append(instr, 0x05)                                    // else
	instr = append(instr, i32Const(2)...)                          // reject: REJECT
	instr = append(instr, 0x0b)                                    // end if
	w.addSection(10, codeSection(body(marker), body(instr)))

	// 7. Data section: the header name at offset 0.
	data := vecCount(1)
	data = append(data, dataActive(0, []byte(HeaderReadName))...)
	w.addSection(11, data)

	return w.Bytes()
}
