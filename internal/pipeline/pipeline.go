// Package pipeline orchestrates a single datalint run: classify each
// path, route it to rules whose capabilities match the file Kind,
// collect findings.
package pipeline

import (
	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/rules"
	"github.com/kaeawc/datalint/internal/scanner"
	"github.com/kaeawc/datalint/internal/suppression"
)

// Run analyzes the given paths against every per-file registered rule.
func Run(paths []string, cfg config.Config) ([]diag.Finding, error) {
	var findings []diag.Finding
	emit := func(f diag.Finding) { findings = append(findings, f) }

	registered := rules.All()
	for _, path := range paths {
		ctx := buildContext(path)
		dispatch(ctx, registered, cfg, emit)
	}
	return suppression.Filter(findings), nil
}

// RunDocument lints a single file using in-memory source bytes when
// supplied. If source is nil the function falls back to disk; this
// matches the LSP server's flow where didOpen/didChange/didSave all
// route through the same call but only the on-disk save needs the
// disk read. Currently honors source override only for Python; other
// kinds use the on-disk variant since editors rarely live-edit them.
func RunDocument(path string, source []byte, cfg config.Config) ([]diag.Finding, error) {
	ctx := buildContextWithSource(path, source)
	var findings []diag.Finding
	emit := func(f diag.Finding) { findings = append(findings, f) }
	dispatch(ctx, rules.All(), cfg, emit)
	return suppression.Filter(findings), nil
}

func dispatch(ctx *rules.Context, registered []*rules.Rule, cfg config.Config, emit func(diag.Finding)) {
	for _, rule := range registered {
		if !cfg.IsEnabled(rule.ID) {
			continue
		}
		if !rule.AppliesTo(ctx.File) {
			continue
		}
		ctx.Settings = cfg.Rule(rule.ID)
		rule.Check(ctx, emit)
	}
}

// RunCorpus runs every corpus-scope rule once against the supplied
// CorpusContext. Returns nil findings when no corpus-scope rules are
// registered or the context has no train/eval/datasets input.
func RunCorpus(corpus *rules.CorpusContext, cfg config.Config) []diag.Finding {
	if corpus == nil || (len(corpus.Train) == 0 && len(corpus.Eval) == 0 && len(corpus.Datasets) == 0) {
		return nil
	}
	var findings []diag.Finding
	emit := func(f diag.Finding) { findings = append(findings, f) }
	for _, rule := range rules.All() {
		if !rule.IsCorpusScope() {
			continue
		}
		if !cfg.IsEnabled(rule.ID) {
			continue
		}
		corpus.Settings = cfg.Rule(rule.ID)
		rule.CorpusCheck(corpus, emit)
	}
	return suppression.Filter(findings)
}

// buildContext classifies the file and parses it eagerly when the
// Kind needs project-supplied state (e.g. PythonFile, ParquetFile).
// Parse and read failures leave the corresponding context field nil;
// rules that depend on it skip such files. A future "file-unreadable"
// rule will surface these as findings instead of silently dropping
// them.
func buildContext(path string) *rules.Context {
	return buildContextWithSource(path, nil)
}

// buildContextWithSource is buildContext with an optional in-memory
// override for the file's bytes. Only honored for KindPythonSource;
// other kinds always read from disk.
func buildContextWithSource(path string, source []byte) *rules.Context {
	file := scanner.Classify(path)
	ctx := &rules.Context{File: file}
	switch file.Kind {
	case scanner.KindPythonSource:
		py := parsePythonForContext(path, source)
		if py != nil {
			ctx.Python = py
		}
	case scanner.KindParquet:
		if pq, err := scanner.ParseParquet(path); err == nil {
			ctx.Parquet = pq
		}
	}
	return ctx
}

func parsePythonForContext(path string, source []byte) *scanner.PythonFile {
	if source != nil {
		py, err := scanner.ParsePythonBytes(path, source)
		if err != nil {
			return nil
		}
		return py
	}
	py, err := scanner.ParsePython(path)
	if err != nil {
		return nil
	}
	return py
}
