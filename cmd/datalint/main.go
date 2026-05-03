// Command datalint is the CLI entry point: it walks the paths it was
// given, runs registered rules, and writes findings to stdout.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/output"
	"github.com/kaeawc/datalint/internal/pipeline"

	// Side-effect imports register the built-in rule set.
	_ "github.com/kaeawc/datalint/internal/rules/builtin"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
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

	if err := output.WriteJSON(os.Stdout, findings); err != nil {
		fmt.Fprintln(os.Stderr, "datalint:", err)
		os.Exit(1)
	}
}
