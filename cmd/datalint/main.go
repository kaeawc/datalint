// Command datalint is the CLI entry point: it walks the paths it was
// given, runs registered rules, and writes findings to stdout.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/output"
	"github.com/kaeawc/datalint/internal/pipeline"

	// Side-effect imports register the built-in rule set.
	_ "github.com/kaeawc/datalint/internal/rules/builtin"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	format := flag.String("format", "json", "output format: json or sarif")
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
	default:
		return fmt.Errorf("unknown format %q (want json or sarif)", format)
	}
}
