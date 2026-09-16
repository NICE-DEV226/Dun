// Package extension defines the public Dun Core-Wasm extension contract.
//
// The package contains stable ABI names and values only. Runtime hosting,
// lifecycle management, and capability enforcement remain implementation
// details of the Dun core.
package extension

const (
	// ABIVersion is the numeric version returned by the ABI marker export.
	ABIVersion uint32 = 1

	// ABIMarkerExport is the required ABI marker export.
	ABIMarkerExport = "dun_abi_version_0_1"
	// ProcessRequestExport is the required request handler export.
	ProcessRequestExport = "dun_process_request"
	// HostModuleName is the module namespace used for Dun host imports.
	HostModuleName = "dun"
)

// Decision is the result returned by ProcessRequestExport.
type Decision uint32

const (
	DecisionContinue Decision = iota
	DecisionRespond
	DecisionReject
	DecisionError
)

// String returns the stable wire name for a decision.
func (d Decision) String() string {
	switch d {
	case DecisionContinue:
		return "continue"
	case DecisionRespond:
		return "respond"
	case DecisionReject:
		return "reject"
	case DecisionError:
		return "error"
	default:
		return "unknown"
	}
}

// Capability names are the fully-qualified host permissions understood by v0.
const (
	CapabilityResponseHeadersWrite = "response.headers.write"
	CapabilityLogInfo              = "log.info"
	CapabilityLogError             = "log.error"
)

// HostImport names the v0 host functions.
const (
	HostImportResponseHeadersWrite = CapabilityResponseHeadersWrite
	HostImportLogInfo              = CapabilityLogInfo
	HostImportLogError             = CapabilityLogError
)

// Errno values returned by host functions.
type Errno uint32

const (
	ErrnoOK          Errno = 0
	ErrnoInvalidCall Errno = 1
)
