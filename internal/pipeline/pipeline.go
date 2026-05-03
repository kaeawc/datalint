// Package pipeline orchestrates a single datalint run: classify each
// path, route it to rules whose capabilities match the file Kind,
// collect findings.
package pipeline

import (
	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/rules"
	"github.com/kaeawc/datalint/internal/scanner"
)

// Run analyzes the given paths against every registered rule.
func Run(paths []string, _ config.Config) ([]diag.Finding, error) {
	var findings []diag.Finding
	emit := func(f diag.Finding) { findings = append(findings, f) }

	registered := rules.All()
	for _, path := range paths {
		file := scanner.Classify(path)
		ctx := &rules.Context{File: file}
		for _, rule := range registered {
			if !rule.AppliesTo(file) {
				continue
			}
			rule.Check(ctx, emit)
		}
	}
	return findings, nil
}
