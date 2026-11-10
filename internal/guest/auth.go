package guest

// AuthModule is the hand-assembled equivalent of this WAT source:
//
//	(module
//	  (type $t0 (func (result i32)))
//	  (type $t1 (func (param i32 i32 i32 i32) (result i32)))
//	  (import "dun" "request.headers.read" (func $read (type 1)))
//	  (memory (export "memory") 1)
//	  (data (i32.const 0) "Authorization")     ;; header name @ 0, len 13
//	  (func $marker (type 0) (result i32)
//	    i32.const 1)
//	  (func $process (type 0) (result i32)
//	    ;; ret = read(0, 13, 256, 128)
//	    i32.const 0 i32.const 13 i32.const 256 i32.const 128 call $read
//	    i32.const -1 i32.eq
//	    if (result i32)
//	      i32.const 2                          ;; absent -> REJECT
//	    else
//	      ;; require value to start with "Bearer " (7 bytes @256)
//	      i32.const 256 i32.load8_u i32.const 66  i32.eq   ;; 'B'
//	      i32.const 257 i32.load8_u i32.const 101 i32.eq
//	      i32.and
//	      i32.const 258 i32.load8_u i32.const 97  i32.eq
//	      i32.and
//	      i32.const 259 i32.load8_u i32.const 114 i32.eq
//	      i32.and
//	      i32.const 260 i32.load8_u i32.const 101 i32.eq
//	      i32.and
//	      i32.const 261 i32.load8_u i32.const 114 i32.eq
//	      i32.and
//	      i32.const 262 i32.load8_u i32.const 32  i32.eq
//	      i32.and
//	      if (result i32)
//	        i32.const 0                        ;; Bearer ok -> CONTINUE
//	      else
//	        i32.const 2                        ;; bad scheme -> REJECT
//	      end
//	    end)
//	  (export "dun_abi_version_0_1" (func $marker))
//	  (export "dun_process_request" (func $process))
//	)
//
// A minimal bearer-token gate: requests without an Authorization header,
// or whose value does not start with "Bearer ", are rejected. It proves
// guests can both read host-written buffers and branch on the result.
const (
	// AuthHeaderName is the request header the fixture inspects.
	AuthHeaderName = "Authorization"
	// AuthOutOffset/AuthOutCap locate the guest buffer the host writes to.
	AuthOutOffset = 256
	AuthOutCap    = 128
	// AuthBearerPrefix is the required value prefix.
	AuthBearerPrefix = "Bearer "
)

// AuthModule builds the bearer-auth guest module bytes.
func AuthModule() []byte {
	w := &wasmWriter{}

	fnType0 := funcType(nil, []byte{0x7f})
	fnType1 := funcType([]byte{0x7f, 0x7f, 0x7f, 0x7f}, []byte{0x7f})
	w.addSection(1, buildTypeSection([][]byte{fnType0, fnType1}))

	w.addSection(2, importFunc(ABIModuleName, "request.headers.read", 1))
	w.addSection(3, funcSection([]uint32{0, 0}))
	w.addSection(5, memorySectionPages(1))
	w.addSection(7, exportSection([]exportEntry{
		{name: ABIVersionMarker, funcIdx: 1},
		{name: "dun_process_request", funcIdx: 2},
	}))

	marker := i32Const(1)

	var instr []byte
	// read(name_off=0, name_len, out_off=256, out_cap=128)
	instr = append(instr, i32Const(0)...)
	instr = append(instr, i32Const(int32(len(AuthHeaderName)))...)
	instr = append(instr, i32Const(AuthOutOffset)...)
	instr = append(instr, i32Const(AuthOutCap)...)
	instr = append(instr, 0x10, 0x00)      // call import 0
	instr = append(instr, i32Const(-1)...) // sentinel
	instr = append(instr, 0x46)            // i32.eq -> absent?
	instr = append(instr, 0x04, 0x7f)      // if (result i32)
	instr = append(instr, i32Const(2)...)  // absent -> REJECT
	instr = append(instr, 0x05)            // else

	// AND-chain of 7 byte comparisons for "Bearer ".
	prefix := AuthBearerPrefix
	for i := 0; i < len(prefix); i++ {
		instr = append(instr, i32Const(AuthOutOffset+int32(i))...)
		instr = append(instr, 0x2d, 0x00, 0x00) // i32.load8_u align=0 offset=0
		instr = append(instr, i32Const(int32(prefix[i]))...)
		instr = append(instr, 0x46) // i32.eq
		if i > 0 {
			instr = append(instr, 0x71) // i32.and with previous
		}
	}
	instr = append(instr, 0x04, 0x7f)     // if (result i32)
	instr = append(instr, i32Const(0)...) // prefix ok -> CONTINUE
	instr = append(instr, 0x05)           // else
	instr = append(instr, i32Const(2)...) // bad scheme -> REJECT
	instr = append(instr, 0x0b)           // end inner if
	instr = append(instr, 0x0b)           // end outer if

	w.addSection(10, codeSection(body(marker), body(instr)))

	data := vecCount(1)
	data = append(data, dataActive(0, []byte(AuthHeaderName))...)
	w.addSection(11, data)

	return w.Bytes()
}
