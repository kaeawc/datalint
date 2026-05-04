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

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	format := flag.String("format", "json", "output format: json, sarif, or html")
	configPath := flag.String("config", "", "path to datalint.yml (default: discover datalint.yml or .datalint.yml in cwd)")
	failOn := flag.String("fail-on", "none", "exit non-zero when any finding has severity >= this (none|info|warning|error)")
	var train, eval stringSliceFlag
	flag.Var(&train, "train", "JSONL file in the train split (repeatable; pairs with --eval for corpus-scope rules)")
	flag.Var(&eval, "eval", "JSONL file in the eval split (repeatable; pairs with --train for corpus-scope rules)")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
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
	if len(train) > 0 || len(eval) > 0 {
		corpus := &rules.CorpusContext{Train: train, Eval: eval}
		findings = append(findings, pipeline.RunCorpus(corpus, cfg)...)
	}

	if err := writeOutput(*format, findings); err != nil {
		fmt.Fprintln(os.Stderr, "datalint:", err)
		os.Exit(1)
	}

	exit, err := exitCodeForFailOn(*failOn, findings)
	if err != nil {
		fmt.Fprintln(os.Stderr, "datalint:", err)
		os.Exit(2)
	}
	if exit != 0 {
		os.Exit(exit)
	}
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
