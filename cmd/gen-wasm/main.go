// Command gen-wasm generates the security-header guest wasm binary.
package main

import (
	"fmt"
	"os"

	"github.com/NICE-DEV226/Dun/internal/guest"
)

func main() {
	data := guest.SecurityHeaderModule()
	if len(os.Args) > 1 {
		if err := os.WriteFile(os.Args[1], data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s (%d bytes)\n", os.Args[1], len(data))
	} else {
		os.Stdout.Write(data)
	}
}
