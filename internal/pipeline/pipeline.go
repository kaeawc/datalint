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

// Run analyzes the given paths against every per-file registered rule.
func Run(paths []string, cfg config.Config) ([]diag.Finding, error) {
	var findings []diag.Finding
	emit := func(f diag.Finding) { findings = append(findings, f) }

	registered := rules.All()
	for _, path := range paths {
		ctx := buildContext(path)
		for _, rule := range registered {
			if !rule.AppliesTo(ctx.File) {
				continue
			}
			ctx.Settings = cfg.Rule(rule.ID)
			rule.Check(ctx, emit)
		}
	}
	return findings, nil
}

// RunCorpus runs every corpus-scope rule once against the supplied
// CorpusContext. Returns nil findings when no corpus-scope rules are
// registered or the context has no train/eval input.
func RunCorpus(corpus *rules.CorpusContext, cfg config.Config) []diag.Finding {
	if corpus == nil || (len(corpus.Train) == 0 && len(corpus.Eval) == 0) {
		return nil
	}
	var findings []diag.Finding
	emit := func(f diag.Finding) { findings = append(findings, f) }
	for _, rule := range rules.All() {
		if !rule.IsCorpusScope() {
			continue
		}
		corpus.Settings = cfg.Rule(rule.ID)
		rule.CorpusCheck(corpus, emit)
	}
	return findings
}

// buildContext classifies the file and parses it eagerly when the
// Kind needs project-supplied state (e.g. PythonFile). Parse and read
// failures leave Python nil; rules that depend on it skip such files.
// A future "file-unreadable" rule will surface these as findings
// instead of silently dropping them.
func buildContext(path string) *rules.Context {
	file := scanner.Classify(path)
	ctx := &rules.Context{File: file}
	if file.Kind == scanner.KindPythonSource {
		if py, err := scanner.ParsePython(path); err == nil {
			ctx.Python = py
		}
	}
	return ctx
}
