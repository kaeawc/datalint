// Package config loads datalint.yml / .datalint.yml from the project
// root and exposes per-rule settings to the dispatcher.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the parsed datalint configuration. The shape is
// intentionally permissive: rules read their own keys via Rule(id),
// and enable/disable are honored by the dispatcher via IsEnabled.
//
// Precedence: a rule in Disable is always off. An empty Enable list
// means "all rules on" (subject to Disable). A non-empty Enable list
// means "only these rules on" (still subject to Disable).
type Config struct {
	Rules   map[string]map[string]any `yaml:"rules"`
	Disable []string                  `yaml:"disable"`
	Enable  []string                  `yaml:"enable"`
}

// Default returns an empty Config — every rule falls back to its
// hardcoded defaults and runs.
func Default() Config {
	return Config{Rules: map[string]map[string]any{}}
}

// IsEnabled reports whether the rule with the given ID should run
// under this configuration.
func (c Config) IsEnabled(id string) bool {
	for _, d := range c.Disable {
		if d == id {
			return false
		}
	}
	if len(c.Enable) == 0 {
		return true
	}
	for _, e := range c.Enable {
		if e == id {
			return true
		}
	}
	return false
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

// Float returns the float64 value at key or def if missing or
// wrong-typed. Accepts ints transparently so YAML scalars like 0
// or 1 read fine into a float-shaped knob.
func (r RuleConfig) Float(key string, def float64) float64 {
	if r.values == nil {
		return def
	}
	v, ok := r.values[key]
	if !ok {
		return def
	}
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
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

// StringMap returns a map[string]string at key, or nil if missing.
// Non-string values are silently skipped. yaml.v3 unmarshalling into
// map[string]any produces map[string]any for nested mappings; we
// also accept the older map[any]any shape for resilience.
func (r RuleConfig) StringMap(key string) map[string]string {
	if r.values == nil {
		return nil
	}
	raw := r.values[key]
	out := map[string]string{}
	switch m := raw.(type) {
	case map[string]any:
		for k, v := range m {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
	case map[any]any:
		for k, v := range m {
			ks, kok := k.(string)
			vs, vok := v.(string)
			if kok && vok {
				out[ks] = vs
			}
		}
	default:
		return nil
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// StringSlice returns the list of strings at key, or nil if missing.
// Non-string elements in the list are silently skipped.
func (r RuleConfig) StringSlice(key string) []string {
	if r.values == nil {
		return nil
	}
	raw, ok := r.values[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
