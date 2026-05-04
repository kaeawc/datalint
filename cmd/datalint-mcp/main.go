// Command datalint-mcp is a Model Context Protocol entry point. It
// speaks JSON-RPC 2.0 over stdio with newline-delimited framing,
// advertises a single `lint` tool that runs datalint's rule pipeline,
// and returns findings as a text content block.
//
// Supported methods (v0):
//   - initialize
//   - tools/list (returns the lint tool descriptor)
//   - tools/call name=lint (runs pipeline.Run / pipeline.RunCorpus)
//
// Larger surface (resources/* for fixture files, prompts/* for
// rule-explanation prompts, code-action-style fix application via a
// dedicated `fix` tool) is a follow-up.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/mcp"
)

func main() {
	cfg, err := config.LoadDiscovered()
	if err != nil {
		fmt.Fprintln(os.Stderr, "datalint-mcp:", err)
		os.Exit(1)
	}
	err = mcp.Run(os.Stdin, os.Stdout, cfg)
	if errors.Is(err, mcp.ErrShutdown) || err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "datalint-mcp:", err)
	os.Exit(1)
}
