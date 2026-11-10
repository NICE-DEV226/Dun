package guest

// These modules are the hand-assembled binary equivalents of small WAT
// sources used to exercise Dun's sandbox behavior (RFC-0000 §14 failure
// modes): traps, execution timeouts, and memory-limit enforcement.

// TrapModule is equivalent to:
//
//	(module
//	  (type (func (result i32)))
//	  (memory (export "memory") 1)
//	  (func $process (type 0) (result i32)
//	    unreachable)     ;; 0x00 — always traps
//	  (export "dun_abi_version_0_1" (func $process))
//	  (export "dun_process_request" (func $process))
//	)
//
// dun_process_request here traps immediately. Validation succeeds (an
// unreachable instruction makes the stack polymorphic, satisfying the
// declared i32 result), but execution always fails — this is the classic
// "hostile plugin" that must not take the core down.
func TrapModule() []byte {
	w := &wasmWriter{}

	fnType0 := funcType(nil, []byte{0x7f}) // () -> i32
	w.addSection(1, buildTypeSection([][]byte{fnType0}))
	w.addSection(3, funcSection([]uint32{0, 0}))
	w.addSection(5, memorySectionPages(1))
	w.addSection(7, exportSection([]exportEntry{
		{name: ABIVersionMarker, funcIdx: 0},
		{name: "dun_process_request", funcIdx: 1},
	}))
	w.addSection(10, codeSection(body(i32Const(1)), body([]byte{0x00}))) // unreachable
	return w.Bytes()
}

// InfiniteLoopModule is equivalent to:
//
//	(module
//	  (type (func (result i32)))
//	  (memory (export "memory") 1)
//	  (func $process (type 0) (result i32)
//	    loop (result i32)  ;; 0x03 0x7f
//	      br 0             ;; 0x0c 0x00 — jump back to loop start
//	    end
//	  )
//	  (export "dun_abi_version_0_1" (func $process))
//	  (export "dun_process_request" (func $process))
//	)
//
// dun_process_request spins forever; a sandbox timeout is the only sane
// response. The `br 0` makes the remainder of the body unreachable, which
// is why the polymorphic stack still satisfies the i32 result type.
func InfiniteLoopModule() []byte {
	w := &wasmWriter{}

	fnType0 := funcType(nil, []byte{0x7f})
	w.addSection(1, buildTypeSection([][]byte{fnType0}))
	w.addSection(3, funcSection([]uint32{0, 0}))
	w.addSection(5, memorySectionPages(1))
	w.addSection(7, exportSection([]exportEntry{
		{name: ABIVersionMarker, funcIdx: 0},
		{name: "dun_process_request", funcIdx: 1},
	}))
	instr := []byte{
		0x03, 0x7f, // loop (result i32)
		0x0c, 0x00, // br 0
		0x0b, // end (loop)
	}
	w.addSection(10, codeSection(body(i32Const(1)), body(instr)))
	return w.Bytes()
}

// MemoryModule is a minimal module whose memory declares minPages.
// Equivalent to:
//
//	(module
//	  (type (func (result i32)))
//	  (memory (export "memory") <minPages>)
//	  (func $process (type 0) (result i32)
//	    i32.const 0)
//	  (export "dun_abi_version_0_1" (func $process))
//	  (export "dun_process_request" (func $process))
//	)
//
// Used to prove memory-limit enforcement: if minPages exceeds the sandbox
// memory limit, loading fails (fail-fast at COMPILE, RFC-0000 §11).
func MemoryModule(minPages uint32) []byte {
	w := &wasmWriter{}

	fnType0 := funcType(nil, []byte{0x7f})
	w.addSection(1, buildTypeSection([][]byte{fnType0}))
	w.addSection(3, funcSection([]uint32{0, 0}))
	w.addSection(5, memorySectionPages(minPages))
	w.addSection(7, exportSection([]exportEntry{
		{name: ABIVersionMarker, funcIdx: 0},
		{name: "dun_process_request", funcIdx: 1},
	}))

	// A benign handler: constant-fold i32.const 0 -> CONTINUE.
	// (Wasm has no nop; a const on the stack of a blockless function is a
	// validation error, so we return it as the i32 result.)
	instr := i32Const(0)
	w.addSection(10, codeSection(body(i32Const(1)), body(instr)))
	return w.Bytes()
}

// GlobalModule declares one mutable i32 global. It is used to prove that
// artifacts carrying state the public wazero API cannot reset (globals,
// tables, element segments) are excluded from instance pooling.
//
//	(module
//	  (type (func (result i32)))
//	  (global (mut i32) (i32.const 0))
//	  (memory (export "memory") 1)
//	  (func $process (type 0) (result i32) i32.const 0)
//	  (export "dun_abi_version_0_1" (func $process))
//	  (export "dun_process_request" (func $process))
//	)
func GlobalModule() []byte {
	w := &wasmWriter{}

	fnType0 := funcType(nil, []byte{0x7f})
	w.addSection(1, buildTypeSection([][]byte{fnType0}))
	w.addSection(3, funcSection([]uint32{0, 0}))
	w.addSection(5, memorySectionPages(1))
	// global section: count=1, i32 (0x7f) mutable (0x01), init i32.const 0, end
	w.addSection(6, []byte{0x01, 0x7f, 0x01, 0x41, 0x00, 0x0b})
	w.addSection(7, exportSection([]exportEntry{
		{name: ABIVersionMarker, funcIdx: 0},
		{name: "dun_process_request", funcIdx: 1},
	}))
	w.addSection(10, codeSection(body(i32Const(1)), body(i32Const(0))))
	return w.Bytes()
}

// memorySectionPages writes a memory section with a single memory of
// minPages and no declared maximum.
func memorySectionPages(minPages uint32) []byte {
	content := vecCount(1)
	content = append(content, 0x00)              // flags: no max
	content = append(content, uLEB(minPages)...) // min pages
	return content
}
