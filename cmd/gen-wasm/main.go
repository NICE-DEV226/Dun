// Command gen-wasm generates the example guest wasm binaries.
//
// Each artifact is written to plugins/examples/<name>/<name>.wasm,
// matching the paths referenced by examples/*/dun.yaml.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/NICE-DEV226/Dun/internal/guest"
)

// artifacts maps each example plugin name to its generated module.
var artifacts = map[string][]byte{
	"security-header": guest.SecurityHeaderModule(),
	"deny-header":     guest.HeaderReadModule(),
	"auth":            guest.AuthModule(),
	"cors":            guest.CORSModule(),
}

func main() {
	root := "plugins/examples"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	for name, data := range artifacts {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", dir, err)
			os.Exit(1)
		}
		path := filepath.Join(dir, name+".wasm")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s (%d bytes)\n", path, len(data))
	}
}
