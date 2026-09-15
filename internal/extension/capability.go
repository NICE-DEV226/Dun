package extension

import (
	"sort"
	"strings"
)

// Capability is a single permission granted to an extension.
type Capability string

// Well-known capability domains.
const (
	CapRequestHeadersRead   Capability = "request.headers.read"
	CapRequestHeadersWrite  Capability = "request.headers.write"
	CapRequestBodyRead      Capability = "request.body.read"
	CapRequestBodyWrite     Capability = "request.body.write"
	CapResponseStatusRead   Capability = "response.status.read"
	CapResponseStatusWrite  Capability = "response.status.write"
	CapResponseHeadersRead  Capability = "response.headers.read"
	CapResponseHeadersWrite Capability = "response.headers.write"
	CapResponseBodyRead     Capability = "response.body.read"
	CapResponseBodyWrite    Capability = "response.body.write"
	CapMetadataRead         Capability = "context.metadata.read"
	CapMetadataWrite        Capability = "context.metadata.write"
	CapStateRead            Capability = "context.state.read"
	CapStateWrite           Capability = "context.state.write"
	CapLogDebug             Capability = "log.debug"
	CapLogInfo              Capability = "log.info"
	CapLogWarn              Capability = "log.warn"
	CapLogError             Capability = "log.error"
)

// Set is a set of granted capabilities.
type Set map[Capability]struct{}

// NewSet builds a capability set from strings.
func NewSet(caps ...string) Set {
	s := make(Set, len(caps))
	for _, c := range caps {
		s[Capability(strings.TrimSpace(c))] = struct{}{}
	}
	return s
}

// Add grants a capability.
func (s Set) Add(c Capability) {
	s[c] = struct{}{}
}

// Has reports whether the capability is granted.
func (s Set) Has(c Capability) bool {
	_, ok := s[c]
	return ok
}

// HasAll reports whether all given capabilities are granted.
func (s Set) HasAll(caps ...Capability) bool {
	for _, c := range caps {
		if !s.Has(c) {
			return false
		}
	}
	return true
}

// Sorted returns the capabilities in sorted, stable order.
func (s Set) Sorted() []string {
	out := make([]string, 0, len(s))
	for c := range s {
		out = append(out, string(c))
	}
	sort.Strings(out)
	return out
}

// String returns a human-readable representation.
func (s Set) String() string {
	return strings.Join(s.Sorted(), ",")
}

// ValidateManifest checks that capabilities declared in a manifest are
// valid capability strings. Unknown capabilities are rejected.
func ValidateManifest(declared Set) error {
	for c := range declared {
		switch c {
		case CapRequestHeadersRead,
			CapRequestHeadersWrite,
			CapRequestBodyRead,
			CapRequestBodyWrite,
			CapResponseStatusRead,
			CapResponseStatusWrite,
			CapResponseHeadersRead,
			CapResponseHeadersWrite,
			CapResponseBodyRead,
			CapResponseBodyWrite,
			CapMetadataRead,
			CapMetadataWrite,
			CapStateRead,
			CapStateWrite,
			CapLogDebug,
			CapLogInfo,
			CapLogWarn,
			CapLogError:
			// valid
		default:
			return &InvalidCapabilityError{Cap: c}
		}
	}
	return nil
}

// InvalidCapabilityError is returned for an unknown capability string.
type InvalidCapabilityError struct {
	Cap Capability
}

func (e *InvalidCapabilityError) Error() string {
	return "unknown capability: " + string(e.Cap)
}

// Intersection returns the capabilities in both sets.
func (s Set) Intersection(other Set) Set {
	out := NewSet()
	for c := range s {
		if other.Has(c) {
			out.Add(c)
		}
	}
	return out
}

// Union returns the capabilities in either set.
func (s Set) Union(other Set) Set {
	out := NewSet()
	for c := range s {
		out.Add(c)
	}
	for c := range other {
		out.Add(c)
	}
	return out
}
