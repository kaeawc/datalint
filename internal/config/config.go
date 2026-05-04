// Package config loads datalint.yml / .datalint.yml from the project
// root and exposes per-rule settings to the dispatcher.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the parsed datalint configuration. The shape is
// intentionally permissive: rules read their own keys via Rule(id).
type Config struct {
	Rules map[string]map[string]any `yaml:"rules"`
}

// Default returns an empty Config — every rule falls back to its
// hardcoded defaults.
func Default() Config {
	return Config{Rules: map[string]map[string]any{}}
}

// Load reads and parses a YAML config file at path.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if cfg.Rules == nil {
		cfg.Rules = map[string]map[string]any{}
	}
	return cfg, nil
}

// LoadDiscovered tries datalint.yml then .datalint.yml in the cwd.
// Returns Default() if neither exists.
func LoadDiscovered() (Config, error) {
	for _, name := range []string{"datalint.yml", ".datalint.yml"} {
		if _, err := os.Stat(name); err == nil {
			return Load(name)
		}
	}
	return Default(), nil
}

// Rule returns the per-rule settings block. Missing rule → empty
// RuleConfig (every Get returns its default).
func (c Config) Rule(id string) RuleConfig {
	return RuleConfig{values: c.Rules[id]}
}

// RuleConfig is the typed accessor a rule uses to read its own keys.
type RuleConfig struct {
	values map[string]any
}

// Int returns the integer value at key or def if missing or wrong-typed.
func (r RuleConfig) Int(key string, def int) int {
	if r.values == nil {
		return def
	}
	v, ok := r.values[key]
	if !ok {
		return def
	}
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	}
	return def
}

// String returns the string value at key or def if missing or wrong-typed.
func (r RuleConfig) String(key string, def string) string {
	if r.values == nil {
		return def
	}
	v, ok := r.values[key]
	if !ok {
		return def
	}
	s, ok := v.(string)
	if !ok {
		return def
	}
	return s
}
