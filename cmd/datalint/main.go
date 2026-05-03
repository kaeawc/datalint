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
	var train, eval stringSliceFlag
	flag.Var(&train, "train", "JSONL file in the train split (repeatable; pairs with --eval for corpus-scope rules)")
	flag.Var(&eval, "eval", "JSONL file in the eval split (repeatable; pairs with --train for corpus-scope rules)")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	findings, err := pipeline.Run(flag.Args(), config.Default())
	if err != nil {
		fmt.Fprintln(os.Stderr, "datalint:", err)
		os.Exit(1)
	}
	if len(train) > 0 || len(eval) > 0 {
		corpus := &rules.CorpusContext{Train: train, Eval: eval}
		findings = append(findings, pipeline.RunCorpus(corpus, config.Default())...)
	}

	if err := writeOutput(*format, findings); err != nil {
		fmt.Fprintln(os.Stderr, "datalint:", err)
		os.Exit(1)
	}
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
