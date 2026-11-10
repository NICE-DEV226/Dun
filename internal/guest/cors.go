package guest

// CORSModule is the hand-assembled equivalent of this WAT source:
//
//	(module
//	  (type $t0 (func (result i32)))
//	  (type $t1 (func (param i32 i32 i32 i32) (result i32)))
//	  (import "dun" "response.headers.write" (func $set (type 1)))
//	  (memory (export "memory") 1)
//	  (data (i32.const 0)   "Access-Control-Allow-Origin")
//	  (data (i32.const 32)  "*")
//	  (data (i32.const 64)  "Access-Control-Allow-Methods")
//	  (data (i32.const 96)  "GET, POST, OPTIONS")
//	  (data (i32.const 128) "Access-Control-Allow-Headers")
//	  (data (i32.const 160) "Content-Type, Authorization")
//	  (func $marker (type 0) (result i32) i32.const 1)
//	  (func $process (type 0) (result i32)
//	    ;; three response.headers.write calls, then CONTINUE
//	    ... call $set; drop  (x3)
//	    i32.const 0)
//	  (export "dun_abi_version_0_1" (func $marker))
//	  (export "dun_process_request" (func $process))
//	)
//
// It exercises repeated host calls in a single invocation: three CORS
// headers are written per request.
const (
	corsOriginName  = "Access-Control-Allow-Origin"
	corsOriginValue = "*"
	corsMethodsName = "Access-Control-Allow-Methods"
	corsMethodsVal  = "GET, POST, OPTIONS"
	corsHeadersName = "Access-Control-Allow-Headers"
	corsHeadersVal  = "Content-Type, Authorization"
)

// CORSModule builds the CORS guest module bytes.
func CORSModule() []byte {
	w := &wasmWriter{}

	fnType0 := funcType(nil, []byte{0x7f})
	fnType1 := funcType([]byte{0x7f, 0x7f, 0x7f, 0x7f}, []byte{0x7f})
	w.addSection(1, buildTypeSection([][]byte{fnType0, fnType1}))

	w.addSection(2, importFunc(ABIModuleName, "response.headers.write", 1))
	w.addSection(3, funcSection([]uint32{0, 0}))
	w.addSection(5, memorySectionPages(1))
	w.addSection(7, exportSection([]exportEntry{
		{name: ABIVersionMarker, funcIdx: 1},
		{name: "dun_process_request", funcIdx: 2},
	}))

	marker := i32Const(1)

	type hdr struct {
		name, value     string
		nameOff, valOff int32
	}
	headers := []hdr{
		{corsOriginName, corsOriginValue, 0, 32},
		{corsMethodsName, corsMethodsVal, 64, 96},
		{corsHeadersName, corsHeadersVal, 128, 160},
	}

	var instr []byte
	for _, h := range headers {
		instr = append(instr, i32Const(h.nameOff)...)
		instr = append(instr, i32Const(int32(len(h.name)))...)
		instr = append(instr, i32Const(h.valOff)...)
		instr = append(instr, i32Const(int32(len(h.value)))...)
		instr = append(instr, 0x10, 0x00) // call import 0
		instr = append(instr, 0x1a)       // drop errno
	}
	instr = append(instr, i32Const(0)...)

	w.addSection(10, codeSection(body(marker), body(instr)))

	data := vecCount(len(headers) * 2)
	for _, h := range headers {
		data = append(data, dataActive(h.nameOff, []byte(h.name))...)
		data = append(data, dataActive(h.valOff, []byte(h.value))...)
	}
	w.addSection(11, data)

	return w.Bytes()
}
