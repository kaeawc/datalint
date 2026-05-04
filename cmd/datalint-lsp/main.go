// Command datalint-lsp is a Language Server Protocol entry point.
// It speaks JSON-RPC 2.0 over stdio and publishes datalint findings
// as LSP diagnostics on textDocument/didOpen and textDocument/didSave.
//
// Supported methods (v0):
//   - initialize / initialized
//   - shutdown / exit
//   - textDocument/didOpen
//   - textDocument/didSave
//   - textDocument/didClose (clears diagnostics for the file)
//
// Larger surface (didChange with incremental sync, code actions for
// auto-fixes, workspace/* methods) is a follow-up.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/lsp"
)

func main() {
	cfg, err := config.LoadDiscovered()
	if err != nil {
		fmt.Fprintln(os.Stderr, "datalint-lsp:", err)
		os.Exit(1)
	}
	err = lsp.Run(os.Stdin, os.Stdout, cfg)
	if errors.Is(err, lsp.ErrShutdown) || err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "datalint-lsp:", err)
	os.Exit(1)
}
