# CLAUDE.md

Guidance for Claude Code working on datalint.

## Project Overview

datalint is a Go-first static linter for RLHF/RLAIF/SFT training-data pipelines.
It lints the Python pipeline *code* (tree-sitter), the *schemas* the data
declares, and the *data files* themselves (JSONL today; Parquet, MDS,
WebDataset later). Architecture and rule pipeline mirror Krit
(`~/kaeawc/krit/CLAUDE.md`) — read that first.

## Key Rules

- Keep analyzer and rule work in Go.
- After implementation changes, run `go build ./cmd/datalint/ && go vet ./...`.
- Run `go test ./... -count=1` for full validation; use focused package tests
  while iterating.
- Use tree-sitter AST/flat nodes for structural analysis on source code; use
  streaming readers (one row at a time) for data files — never load a corpus
  into memory.
- New rules use the registry pattern in `internal/rules/`: implement the
  `Rule` struct with a `Check` callback, declare `Capability` flags, and
  register in `init()` from a package under `internal/rules/builtin/`.
- Capability flags so far: `NeedsPythonAST`, `NeedsJSONL`, `NeedsParquet`,
  `NeedsCorpusScan`, `NeedsLSH`, `NeedsExternalEvalSet`. Add more as needed.
- Add positive and negative fixtures under `tests/fixtures/`; fixable rules
  also need fixable fixtures.
- Auto-fixes declare `FixCosmetic`, `FixIdiomatic`, or `FixSemantic`. Semantic
  fixes apply only to pipeline code, never to data rows.

## Rule Implementation Guardrails

- For Python source rules, prefer tree-sitter flat AST, identifiers, navigation
  chains, and imports over `strings.Contains` or broad regexes.
- For data rules, emit row-pointers (not byte offsets) so findings remain
  stable across reformatting.
- Stream data files; never `ReadAll`. Parquet/MDS readers must respect row
  groups and shard boundaries.
- Capability gates exist so expensive passes (corpus-wide MinHash) can be
  opted into in CI but skipped on every commit.
- Verify parser shape for important Python constructs (decorators, comprehensions,
  walrus, `yield from`) with focused parser/helper tests.

## Project Structure

- `cmd/datalint/` — CLI entry point.
- `cmd/datalint-lsp/` — LSP server (skeleton).
- `cmd/datalint-mcp/` — MCP server (skeleton).
- `internal/diag/` — `Finding`, `Location`, `Severity`.
- `internal/rules/` — registry, capability flags, rule metadata.
- `internal/rules/builtin/` — built-in rule registrations.
- `internal/scanner/` — tree-sitter Python + JSONL/Parquet ingestion.
- `internal/pipeline/` — run orchestration.
- `internal/config/` — `datalint.yml` loader.
- `internal/output/` — JSON, SARIF, HTML formatters.
- `tests/fixtures/` — positive, negative, and fixable rule fixtures.
- `ci/Dockerfile`, `.github/workflows/commit.yml` — CI tooling lifted from
  `~/kaeawc/golang-build`.

## Build & Validate

```bash
make build              # go build -o datalint ./cmd/datalint/
make vet                # go vet ./...
make test               # go test ./... -count=1
make lint               # golangci-lint run
make complexity         # gocyclo -over 10
make security           # gosec ./...
make ci                 # vet test complexity lint security licenses
make pre-push           # lint + complexity + test — run before every push
```

Always run `make pre-push` before `git push` on any PR branch. CI's
`compile-binary`, `test`, `static-analysis`, and `complexity` jobs are
required for branch protection on `main`, so a failure there blocks
auto-merge until the next push lands. `pre-push` mirrors those gates
locally with the slow/flaky ones (`gosec`, `go-licenses`) skipped.

## Adding a Rule

1. Create the rule file under `internal/rules/builtin/<id>.go`.
2. Implement the local `check` function with signature
   `func(*rules.Context, func(diag.Finding))`.
3. Register it via `rules.Register(&rules.Rule{...})` in `init()`.
4. Declare `Category`, `Severity`, `Confidence`, `Fix`, and the `Capability`
   bits the dispatcher must provide.
5. Add positive and negative fixtures under `tests/fixtures/<id>/`.
6. For autofix rules, add fixable fixtures and set the fix safety level.

## Configuration

datalint loads `datalint.yml` / `.datalint.yml` from the project root with
`--config FILE` as an override. (Loader stub only — see `internal/config/`.)

## Non-goals

- Training framework integration.
- Quality scoring of individual examples (reward-model territory).
- Replacing Great Expectations / Pandera — datalint complements them, focused
  on LLM-data-specific failure modes.
