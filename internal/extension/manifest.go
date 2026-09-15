package extension

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Manifest is the machine-readable metadata for a Dun plugin.
//
// Per RFC-0000 §22, the api field is descriptive metadata only. The
// authoritative ABI gate is the exported marker function checked at
// VALIDATE.
type Manifest struct {
	Name    string  `yaml:"name"`
	Version string  `yaml:"version"`
	API     string  `yaml:"api"`
	Runtime Runtime `yaml:"runtime"`
	Caps    Caps    `yaml:"capabilities"`
	Limits  Limits  `yaml:"limits"`
}

// Runtime describes the Wasm runtime target for a plugin.
type Runtime struct {
	Type   string `yaml:"type"`
	Format string `yaml:"format"`
}

// Caps holds the capability grants organized by domain.
type Caps struct {
	Request  []string `yaml:"request"`
	Response []string `yaml:"response"`
	Context  []string `yaml:"context"`
	Log      []string `yaml:"log"`
}

// Limits describes resource limits for a plugin.
type Limits struct {
	Memory    string `yaml:"memory"`
	Timeout   string `yaml:"timeout"`
	BodySize  string `yaml:"body"`
	Instances int    `yaml:"instances"`
}

// All returns every declared capability as a flat set.
// Capabilities are namespaced by their manifest domain (request., response.,
// context., log.) to match the fully-qualified capability model.
func (m *Manifest) All() Set {
	s := NewSet()
	s = addDomains(s, "request", m.Caps.Request...)
	s = addDomains(s, "response", m.Caps.Response...)
	s = addDomains(s, "context", m.Caps.Context...)
	s = addDomains(s, "log", m.Caps.Log...)
	return s
}

func addDomains(s Set, domain string, names ...string) Set {
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		// Avoid double-prefixing a fully-qualified capability.
		if !strings.HasPrefix(name, domain+".") {
			name = domain + "." + name
		}
		s.Add(Capability(name))
	}
	return s
}

// ParseManifest reads a YAML manifest from a byte slice.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if strings.TrimSpace(m.Name) == "" {
		return nil, &ManifestError{"name is required"}
	}
	if strings.TrimSpace(m.Version) == "" {
		return nil, &ManifestError{"version is required"}
	}
	if err := ValidateManifest(m.All()); err != nil {
		return nil, err
	}
	return &m, nil
}

// LoadManifest reads and parses a manifest file.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseManifest(data)
}

// ParseResourceLimits parses the stringy manifest/config limits into a
// typed ResourceLimits. Empty fields leave the corresponding value zero
// (runtime default). Sizes accept binary units (KiB, MiB, GiB) or a bare
// integer meaning bytes.
func ParseResourceLimits(l Limits) (ResourceLimits, error) {
	var out ResourceLimits

	if l.Memory != "" {
		v, err := ParseSize(l.Memory)
		if err != nil {
			return out, fmt.Errorf("memory: %w", err)
		}
		out.MaxMemoryBytes = v
	}
	if l.Timeout != "" {
		d, err := time.ParseDuration(l.Timeout)
		if err != nil {
			return out, fmt.Errorf("timeout: %w", err)
		}
		if d <= 0 {
			return out, fmt.Errorf("timeout: must be positive, got %q", l.Timeout)
		}
		out.ExecutionTimeout = d
	}
	if l.BodySize != "" {
		v, err := ParseSize(l.BodySize)
		if err != nil {
			return out, fmt.Errorf("body: %w", err)
		}
		out.MaxBodyBytes = v
	}
	if l.Instances < 0 {
		return out, fmt.Errorf("instances: must be >= 0, got %d", l.Instances)
	}
	out.MaxInstances = l.Instances

	return out, nil
}

// ParseSize parses a byte size string supporting binary units.
// Examples: "1024", "64KiB", "1MiB", "2GiB". Unit suffixes are
// case-insensitive. Decimal units (KB/MB/GB) are not supported; use
// KiB/MiB/GiB for clarity.
func ParseSize(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}

	unit := uint64(1)
	up := strings.ToUpper(s)
	switch {
	case strings.HasSuffix(up, "KIB"):
		unit, up = 1<<10, strings.TrimSuffix(up, "KIB")
	case strings.HasSuffix(up, "MIB"):
		unit, up = 1<<20, strings.TrimSuffix(up, "MIB")
	case strings.HasSuffix(up, "GIB"):
		unit, up = 1<<30, strings.TrimSuffix(up, "GIB")
	}

	n, err := strconv.ParseUint(strings.TrimSpace(up), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	return n * unit, nil
}

// ManifestError is a manifest validation error.
type ManifestError struct{ msg string }

func (e *ManifestError) Error() string { return "manifest: " + e.msg }
