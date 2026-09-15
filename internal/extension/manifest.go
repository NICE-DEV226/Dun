package extension

import (
	"os"
	"strings"

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

// ManifestError is a manifest validation error.
type ManifestError struct{ msg string }

func (e *ManifestError) Error() string { return "manifest: " + e.msg }
