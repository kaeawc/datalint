// Package pipeline orchestrates a single datalint run: discover files,
// route them through the dispatcher, collect findings.
package pipeline

import (
	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/rules"
)

// Run analyzes the given paths against every registered rule and
// returns the collected Findings. The skeleton walks no files and emits
// nothing; it exists so the CLI wiring compiles end-to-end.
func Run(_ []string, _ config.Config) ([]diag.Finding, error) {
	_ = rules.All()
	return nil, nil
}
