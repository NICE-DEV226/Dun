// Package config provides configuration loading and validation for Dun.
package config

import (
	"fmt"
	"os"

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
	Name    string `yaml:"name"`
	Path    string `yaml:"path"`
	Version string `yaml:"version"`
	limits  PluginLimits
}

// PluginLimits defines resource limits for a plugin.
type PluginLimits struct {
	Memory    string `yaml:"memory"`
	Timeout   string `yaml:"timeout"`
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
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

// Validate checks the configuration for obvious errors.
func (c *Config) Validate() error {
	if c.Listen.Address == "" {
		return fmt.Errorf("listen.address is required")
	}
	if len(c.Routes) == 0 {
		return fmt.Errorf("at least one route is required")
	}
	for i, r := range c.Routes {
		if r.Upstream == "" {
			return fmt.Errorf("route[%d]: upstream is required", i)
		}
		if r.Match.PathPrefix == "" && r.Match.PathExact == "" {
			return fmt.Errorf("route[%d]: match.path_prefix or match.path_exact is required", i)
		}
	}
	return nil
}
