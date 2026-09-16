//go:build !tinygo

package dun

const (
	Continue uint32 = iota
	Respond
	Reject
	Error
)

func ABIExport() uint32 { return 1 }

// WriteResponseHeader is a host-call stub used only when checking the SDK
// with the standard Go toolchain.
func WriteResponseHeader(_, _ string) uint32 { return 1 }

func ProcessRequest() uint32 { return Continue }
