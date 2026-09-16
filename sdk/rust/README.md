# Dun Rust SDK

Minimal Rust bindings for the Dun Core-Wasm v0 contract. The crate keeps its
surface small so it can be adapted to `no_std` plugin environments later.

Build for WebAssembly:

```bash
rustup target add wasm32-unknown-unknown
cargo build --release --target wasm32-unknown-unknown
```

The resulting module must export `dun_abi_version_0_1` returning `1` and
`dun_process_request` returning a Dun decision code. Host imports are only
available when the corresponding capability is granted by Dun.
