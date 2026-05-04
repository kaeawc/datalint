// Command datalint is the CLI entry point: it walks the paths it was
// given, runs registered rules, and writes findings to stdout.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/diff"
	"github.com/kaeawc/datalint/internal/fixer"
	"github.com/kaeawc/datalint/internal/output"
	"github.com/kaeawc/datalint/internal/pipeline"
	"github.com/kaeawc/datalint/internal/rules"

	// Side-effect imports register the built-in rule set.
	_ "github.com/kaeawc/datalint/internal/rules/builtin"
)

var version = "dev"

// stringSliceFlag accepts a repeatable flag like --train a.jsonl --train b.jsonl.
type stringSliceFlag []string

func (s *stringSliceFlag) String() string     { return strings.Join(*s, ",") }
func (s *stringSliceFlag) Set(v string) error { *s = append(*s, v); return nil }

// datasetFlag accepts the `--dataset NAME=PATH[,PATH...]` shape and
// folds repeats into a single map[name][]paths. Same name used twice
// concatenates: `--dataset train=a.jsonl --dataset train=b.jsonl` ⇒
// {train: [a.jsonl, b.jsonl]}.
type datasetFlag map[string][]string

func (d datasetFlag) String() string {
	parts := make([]string, 0, len(d))
	for name, paths := range d {
		parts = append(parts, name+"="+strings.Join(paths, ","))
	}
	return strings.Join(parts, " ")
}

func (d datasetFlag) Set(v string) error {
	idx := strings.IndexByte(v, '=')
	if idx <= 0 {
		return fmt.Errorf("expected NAME=PATH[,PATH...], got %q", v)
	}
	name := v[:idx]
	paths := strings.Split(v[idx+1:], ",")
	for _, p := range paths {
		if p == "" {
			return fmt.Errorf("empty path in --dataset value %q", v)
		}
	}
	d[name] = append(d[name], paths...)
	return nil
}

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	format := flag.String("format", "json", "output format: json, sarif, or html")
	configPath := flag.String("config", "", "path to datalint.yml (default: discover datalint.yml or .datalint.yml in cwd)")
	failOn := flag.String("fail-on", "none", "exit non-zero when any finding has severity >= this (none|info|warning|error)")
	minSeverity := flag.String("min-severity", "none", "drop findings below this severity from output (none|info|warning|error); does not affect --fail-on")
	autoFix := flag.Bool("fix", false, "apply auto-fixes for findings whose rule emits one (modifies files in place)")
	diffOld := flag.String("diff-old", "", "JSONL path of the old dataset version (paired with --diff-new); enables diff mode and skips the rule pipeline")
	diffNew := flag.String("diff-new", "", "JSONL path of the new dataset version (paired with --diff-old)")
	diffFormat := flag.String("diff-format", "text", "diff output format: text or json (default text)")
	var train, eval stringSliceFlag
	flag.Var(&train, "train", "JSONL file in the train split (repeatable; pairs with --eval for corpus-scope rules)")
	flag.Var(&eval, "eval", "JSONL file in the eval split (repeatable; pairs with --train for corpus-scope rules)")
	datasets := datasetFlag{}
	flag.Var(datasets, "dataset", "named JSONL split for cross-dataset rules: NAME=PATH[,PATH...] (repeatable; same NAME concatenates)")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	if *diffOld != "" || *diffNew != "" {
		runDiff(*diffOld, *diffNew, *diffFormat)
		return
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "datalint:", err)
		os.Exit(1)
	}

	findings, err := pipeline.Run(flag.Args(), cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "datalint:", err)
		os.Exit(1)
	}
	findings = append(findings, runCorpusIfRequested(train, eval, datasets, cfg)...)

	displayed, err := filterBySeverity(findings, *minSeverity)
	if err != nil {
		fmt.Fprintln(os.Stderr, "datalint:", err)
		os.Exit(2)
	}
	if err := writeOutput(*format, displayed); err != nil {
		fmt.Fprintln(os.Stderr, "datalint:", err)
		os.Exit(1)
	}

	applyFixesIfRequested(*autoFix, findings)

	// fail-on intentionally inspects the unfiltered set: --min-severity
	// is a display preference, --fail-on is a CI gate. Hiding errors
	// from output shouldn't also hide them from the gate.
	enforceFailOn(*failOn, findings)
}

// runCorpusIfRequested runs corpus-scope rules when at least one of
// the corpus inputs is non-empty. Pulled out of main to keep main's
// gocyclo complexity under 10.
func runCorpusIfRequested(train, eval []string, datasets datasetFlag, cfg config.Config) []diag.Finding {
	if len(train) == 0 && len(eval) == 0 && len(datasets) == 0 {
		return nil
	}
	corpus := &rules.CorpusContext{Train: train, Eval: eval, Datasets: datasets}
	return pipeline.RunCorpus(corpus, cfg)
}

func applyFixesIfRequested(autoFix bool, findings []diag.Finding) {
	if !autoFix {
		return
	}
	res, err := fixer.Apply(findings)
	if err != nil {
		fmt.Fprintln(os.Stderr, "datalint:", err)
		os.Exit(1)
	}
	if res.FilesModified > 0 {
		fmt.Fprintf(os.Stderr, "datalint: applied %d edit(s) across %d file(s)\n",
			res.EditsApplied, res.FilesModified)
	}
}

func enforceFailOn(level string, findings []diag.Finding) {
	exit, err := exitCodeForFailOn(level, findings)
	if err != nil {
		fmt.Fprintln(os.Stderr, "datalint:", err)
		os.Exit(2)
	}
	if exit != 0 {
		os.Exit(exit)
	}
}

// runDiff handles the --diff-old / --diff-new code path. Both path
// flags must be set; missing one is a configuration error (exit 2).
// format selects the output renderer; bad values exit 2 too.
func runDiff(oldPath, newPath, format string) {
	if oldPath == "" || newPath == "" {
		fmt.Fprintln(os.Stderr, "datalint: --diff-old and --diff-new must both be set")
		os.Exit(2)
	}
	report, err := diff.Compute(oldPath, newPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "datalint:", err)
		os.Exit(1)
	}
	if err := writeDiffReport(format, report); err != nil {
		fmt.Fprintln(os.Stderr, "datalint:", err)
		os.Exit(2)
	}
}

func writeDiffReport(format string, report diff.Report) error {
	switch format {
	case "text":
		return diff.WriteText(os.Stdout, report)
	case "json":
		return diff.WriteJSON(os.Stdout, report)
	}
	return fmt.Errorf("unknown diff format %q (want text or json)", format)
}

func loadConfig(path string) (config.Config, error) {
	if path != "" {
		return config.Load(path)
	}
	return config.LoadDiscovered()
}

func writeOutput(format string, findings []diag.Finding) error {
	switch format {
	case "json":
		return output.WriteJSON(os.Stdout, findings)
	case "sarif":
		return output.WriteSARIF(os.Stdout, findings, version)
	case "html":
		return output.WriteHTML(os.Stdout, findings, version, time.Now())
	default:
		return fmt.Errorf("unknown format %q (want json, sarif, or html)", format)
	}
}

// exitCodeForFailOn returns 1 when any finding has severity at or
// above the requested threshold, else 0. "none" disables the gate
// entirely (the default — current behavior of always exit 0).
func exitCodeForFailOn(level string, findings []diag.Finding) (int, error) {
	if level == "none" {
		return 0, nil
	}
	threshold, err := parseSeverity(level)
	if err != nil {
		return 0, err
	}
	for _, f := range findings {
		if f.Severity >= threshold {
			return 1, nil
		}
	}
	return 0, nil
}

// filterBySeverity returns findings with severity >= the requested
// threshold. "none" passes through unchanged. Bad levels yield an
// error so the caller can exit with a config-error code.
func filterBySeverity(findings []diag.Finding, level string) ([]diag.Finding, error) {
	if level == "none" {
		return findings, nil
	}
	threshold, err := parseSeverity(level)
	if err != nil {
		return nil, err
	}
	out := make([]diag.Finding, 0, len(findings))
	for _, f := range findings {
		if f.Severity >= threshold {
			out = append(out, f)
		}
	}
	return out, nil
}

func parseSeverity(s string) (diag.Severity, error) {
	switch s {
	case "info":
		return diag.SeverityInfo, nil
	case "warning":
		return diag.SeverityWarning, nil
	case "error":
		return diag.SeverityError, nil
	}
	return 0, fmt.Errorf("invalid severity %q (want info, warning, error, or none)", s)
}
