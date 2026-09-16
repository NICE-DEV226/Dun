# Dun Core-Wasm Extension Contract v0

The Go package in this directory publishes the stable names and values of
Dun's Core-Wasm v0 contract. Runtime lifecycle and capability enforcement are
owned by the Dun host.

## Required exports

| Export | Signature | Required value |
| --- | --- | --- |
| `dun_abi_version_0_1` | `() -> i32` | `1` |
| `dun_process_request` | `() -> i32` | decision code |

Decision codes are `0=CONTINUE`, `1=RESPOND`, `2=REJECT`, and `3=ERROR`.

## Host imports

Host imports use the `dun` module namespace. v0 defines:

- `response.headers.write(name_offset, name_length, value_offset, value_length) -> errno`
- `log.info(message_offset, message_length)`
- `log.error(message_offset, message_length)`

Strings are UTF-8 byte ranges in guest linear memory. A host function returns
`0` on success and `1` when the call is invalid or unavailable.

## Compatibility

An extension must export the ABI marker and return value `1`. Its imported host
functions must be granted by the deployment policy. Dun creates a fresh Wasm
instance per request in v0; plugin instance state must not be used as durable
state.

The conformance suite is located in `tests/conformance`. Run it with:

```bash
go test ./api/extension ./tests/conformance
```
