//! Minimal Rust bindings for Dun Core-Wasm v0.

pub const CONTINUE: u32 = 0;
pub const RESPOND: u32 = 1;
pub const REJECT: u32 = 2;
pub const ERROR: u32 = 3;

#[link(wasm_import_module = "dun")]
extern "C" {
    #[link_name = "response.headers.write"]
    fn response_headers_write(
        name_ptr: *const u8,
        name_len: u32,
        value_ptr: *const u8,
        value_len: u32,
    ) -> u32;
}

/// ABI marker required by the Dun host.
#[no_mangle]
pub extern "C" fn dun_abi_version_0_1() -> u32 {
    1
}

/// Writes a response header through the capability-gated host import.
pub fn write_response_header(name: &[u8], value: &[u8]) -> u32 {
    unsafe {
        response_headers_write(
            name.as_ptr(),
            name.len() as u32,
            value.as_ptr(),
            value.len() as u32,
        )
    }
}

/// Default request handler. Applications can provide their own exported
/// handler with the same ABI signature.
#[no_mangle]
pub extern "C" fn dun_process_request() -> u32 {
    CONTINUE
}
