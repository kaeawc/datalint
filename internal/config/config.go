// Package config loads datalint.yml / .datalint.yml from the project
// root. The skeleton ships with a default and no on-disk loader yet.
package config

// Config is the user-facing configuration surface.
type Config struct {
	Disable []string `yaml:"disable"`
	Enable  []string `yaml:"enable"`
}

// Default returns the zero-value config.
func Default() Config { return Config{} }
