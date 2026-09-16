// Package config provides configuration loading and validation for Dun.
package config

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level Dun configuration.
type Config struct {
	Listen  ListenConfig   `yaml:"listen"`
	Routes  []RouteConfig  `yaml:"routes"`
	Log     LogConfig      `yaml:"log"`
	Plugins []PluginConfig `yaml:"plugins"`
}

// ListenConfig defines the HTTP listener settings.
type ListenConfig struct {
	Address string `yaml:"address"`
}

// RouteConfig defines a single routing rule.
type RouteConfig struct {
	Name     string            `yaml:"name"`
	Match    MatchConfig       `yaml:"match"`
	Upstream string            `yaml:"upstream"`
	Plugins  []string          `yaml:"plugins"`
	Headers  map[string]string `yaml:"headers"`
}

// MatchConfig defines how incoming requests are matched to a route.
type MatchConfig struct {
	PathPrefix string `yaml:"path_prefix"`
	PathExact  string `yaml:"path_exact"`
	Host       string `yaml:"host"`
	Method     string `yaml:"method"`
}

// LogConfig controls logging behavior.
type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// PluginConfig defines a plugin entry.
type PluginConfig struct {
	Name         string       `yaml:"name"`
	Path         string       `yaml:"path"`
	Version      string       `yaml:"version"`
	API          string       `yaml:"api"`
	SHA256       string       `yaml:"sha256"`
	Capabilities []string     `yaml:"capabilities"`
	Limits       PluginLimits `yaml:"limits"`
}

// PluginLimits defines resource limits for a plugin.
type PluginLimits struct {
	Memory    string `yaml:"memory"`
	Timeout   string `yaml:"timeout"`
	BodySize  string `yaml:"body"`
	Instances int    `yaml:"instances"`
}

// DefaultConfig returns a minimal default configuration.
func DefaultConfig() *Config {
	return &Config{
		Listen: ListenConfig{
			Address: ":8080",
		},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

// Load reads and parses a YAML configuration file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := DefaultConfig()
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

// Validate checks the configuration for obvious errors.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.Listen.Address) == "" {
		return fmt.Errorf("listen.address is required")
	}
	if len(c.Routes) == 0 {
		return fmt.Errorf("at least one route is required")
	}
	routeNames := make(map[string]struct{}, len(c.Routes))
	pluginNames := make(map[string]struct{}, len(c.Plugins))
	pluginPaths := make(map[string]string, len(c.Plugins))
	for i, p := range c.Plugins {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			return fmt.Errorf("plugin[%d]: name is required", i)
		}
		if _, exists := pluginNames[name]; exists {
			return fmt.Errorf("plugin[%d]: duplicate name %q", i, name)
		}
		pluginNames[name] = struct{}{}
		pluginPaths[name] = strings.TrimSpace(p.Path)
		if p.SHA256 != "" {
			digest := strings.TrimSpace(p.SHA256)
			if len(digest) != sha256HexLength || !isHexDigest(digest) {
				return fmt.Errorf("plugin[%d]: sha256 must be a 64-character hexadecimal digest", i)
			}
		}
	}
	for i, r := range c.Routes {
		name := strings.TrimSpace(r.Name)
		if name == "" {
			return fmt.Errorf("route[%d]: name is required", i)
		}
		if _, exists := routeNames[name]; exists {
			return fmt.Errorf("route[%d]: duplicate name %q", i, name)
		}
		routeNames[name] = struct{}{}

		upstream, err := url.ParseRequestURI(r.Upstream)
		if err != nil || upstream.Scheme == "" || upstream.Host == "" || (upstream.Scheme != "http" && upstream.Scheme != "https") {
			return fmt.Errorf("route[%d]: upstream must be an absolute http(s) URL", i)
		}
		if r.Upstream == "" {
			return fmt.Errorf("route[%d]: upstream is required", i)
		}
		if r.Match.PathPrefix == "" && r.Match.PathExact == "" {
			return fmt.Errorf("route[%d]: match.path_prefix or match.path_exact is required", i)
		}
		if r.Match.PathPrefix != "" && r.Match.PathExact != "" {
			return fmt.Errorf("route[%d]: match.path_prefix and match.path_exact are mutually exclusive", i)
		}
		path := r.Match.PathPrefix
		if path == "" {
			path = r.Match.PathExact
		}
		if !strings.HasPrefix(path, "/") {
			return fmt.Errorf("route[%d]: match path must start with '/'", i)
		}
		if r.Match.Method != "" && !validMethods[strings.ToUpper(r.Match.Method)] {
			return fmt.Errorf("route[%d]: unsupported HTTP method %q", i, r.Match.Method)
		}
		for _, plugin := range r.Plugins {
			if _, exists := pluginNames[plugin]; !exists {
				return fmt.Errorf("route[%d]: plugin %q is not defined", i, plugin)
			}
			if pluginPaths[plugin] == "" {
				return fmt.Errorf("route[%d]: plugin %q has no wasm path", i, plugin)
			}
		}
	}
	return nil
}

var validMethods = map[string]bool{
	"CONNECT": true, "DELETE": true, "GET": true, "HEAD": true,
	"OPTIONS": true, "PATCH": true, "POST": true, "PUT": true, "TRACE": true,
}

const sha256HexLength = 64

func isHexDigest(value string) bool {
	_, err := hex.DecodeString(value)
	return err == nil
}
