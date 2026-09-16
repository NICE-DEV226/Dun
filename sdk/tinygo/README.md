# Dun TinyGo SDK

Minimal TinyGo bindings for the Dun Core-Wasm v0 contract.

The SDK exposes the ABI marker, decision codes, and the
`response.headers.write` host import. Capabilities are still granted by the
Dun deployment configuration.

Example:

```go
package main

import "github.com/NICE-DEV226/Dun/sdk/tinygo/dun"

//go:export dun_process_request
func process() uint32 {
	return dun.Continue
}

func main() {}
```

Build with TinyGo:

```bash
tinygo build -target wasm -o plugin.wasm ./path/to/plugin
```

The plugin must export `dun_abi_version_0_1` returning `1` and
`dun_process_request` returning a Dun decision code.
