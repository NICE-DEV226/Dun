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

    #[link_name = "request.headers.read"]
    fn request_headers_read(
        name_ptr: *const u8,
        name_len: u32,
        out_ptr: *mut u8,
        out_cap: u32,
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

/// Sentinel returned when a request header is absent or the call is
/// invalid. As an i32 the guest sees -1.
pub const READ_ABSENT: u32 = u32::MAX;

/// Reads a request header into `buf`, returning the full value length —
/// retry with a larger buffer when the result exceeds `buf.len()` — or
/// [`READ_ABSENT`] when the header is absent. A present-but-empty header
/// returns 0.
pub fn read_request_header(name: &[u8], buf: &mut [u8]) -> u32 {
    if buf.is_empty() {
        return READ_ABSENT;
    }
    unsafe {
        request_headers_read(
            name.as_ptr(),
            name.len() as u32,
            buf.as_mut_ptr(),
            buf.len() as u32,
        )
    }
}

/// Default request handler. Applications can provide their own exported
/// handler with the same ABI signature.
#[no_mangle]
pub extern "C" fn dun_process_request() -> u32 {
    CONTINUE
}
