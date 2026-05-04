# datalint — a static linter for RLHF / RLAIF / SFT data pipelines

## What you're building

Training-data quality is the silent killer of model quality. The bugs are mundane: train/eval leakage from a sloppy split, label drift after a schema migration, prompt-template version skew across rows of the same dataset, malformed tool-call traces, conversation-turn role inversions, hidden duplicates from a normalization mismatch. Most of this is mechanically detectable, and yet the standard stack (pandas + ad-hoc scripts + a notebook) catches almost none of it pre-train.

datalint is a Krit-shaped static analyzer for training-data pipelines: it lints the *code* that produces the data, lints the *schemas* the data declares, and lints the *files themselves* (JSONL and Parquet today; MDS, WebDataset on the roadmap). Read `~/kaeawc/krit/CLAUDE.md` first; you're reusing the architecture pattern.

## Quickstart

```bash
make build                                                       # builds ./datalint, ./datalint-lsp, ./datalint-mcp

# CLI
./datalint tests/fixtures/jsonl-malformed-line/positive.jsonl    # JSON output by default
./datalint --format=html  ...  > report.html                     # self-contained HTML
./datalint --format=sarif ...                                    # SARIF 2.1.0 for code-scanning
./datalint --format=drops ... | sort -u                          # row-removal manifest: path<TAB>row<TAB>rules
./datalint --train train.jsonl --eval eval.jsonl                 # corpus-scope leakage rules
./datalint --config datalint.yml ...                             # custom thresholds & filters
./datalint --fix path/to/pipeline.py                             # apply auto-fixes in place
./datalint --fail-on=error --min-severity=warning ...            # CI exit codes + display filter
./datalint --diff-old old.jsonl --diff-new new.jsonl             # row count + field set + distribution + length stats
./datalint --diff-old ... --diff-new ... --diff-format=json      # same diff in JSON for scripted consumers
./datalint --dataset train=t.jsonl --dataset eval=e.jsonl ...    # N-way cross-dataset overlap

# IDE / agent integrations (JSON-RPC over stdio)
./datalint-lsp                                                   # Language Server Protocol
./datalint-mcp                                                   # Model Context Protocol
```

A representative `datalint.yml`:

```yaml
enable:
  - jsonl-malformed-line
  - role-inversion
  - train-eval-overlap
disable:
  - dedup-key-misses-normalization   # known-noisy on this codebase
rules:
  enum-drift:
    lock_in_rows: 50
    max_distinct: 20
  optional-field-required-by-downstream:
    min_presence_ratio: 0.95
    min_rows: 50
  train-eval-overlap:
    prompt_field: input
    near_dup_threshold: 0.85
  parquet-row-group-too-large-for-streaming:
    max_rows_per_group: 500000
  system-prompt-leaks-eval-instructions:
    extra_patterns:
      - "(?i)reply with one of"
      - "MMLU"
  privacy-pii-detected:
    extra_patterns:
      - "internal-id=INT-\\d{6,}"
  cross-dataset-overlap:
    prompt_field: input
    near_dup_threshold: 0.85
    anchor: later   # later (default) | earlier — which side of each pair hosts findings
```

In-source / in-data suppression:

```python
random.shuffle(data)  # datalint:disable=random-seed-not-set
```

```jsonl
{"messages": [...], "_datalint_disable": ["role-inversion"]}
```

## Status

Sixteen rules across all five README categories. Configurable thresholds, enable/disable lists, four output formats (JSON / SARIF / HTML / drops), MinHash + LSH near-duplicate detection, suppression markers, auto-fix for `random-seed-not-set`, diff mode with per-field distribution shifts and character-length percentiles (text or JSON), N-way cross-dataset overlap, and live-linting LSP / MCP servers.

| ID | Category | Severity | Confidence | Source | Auto-fix |
|---|---|---|---|---|---|
| `jsonl-malformed-line` | file | error | high | per-file (JSONL) | — |
| `parquet-row-group-too-large-for-streaming` | file | warning | medium | per-file (Parquet) | — |
| `field-type-mixed-across-rows` | schema | warning | high | per-file (JSONL) | — |
| `enum-drift` | schema | warning | medium | per-file (JSONL) | — |
| `optional-field-required-by-downstream` | schema | warning | medium | per-file (JSONL) | — |
| `role-inversion` | conversation | error | high | per-file (JSONL) | — |
| `system-message-mid-conversation` | conversation | error | high | per-file (JSONL) | — |
| `unbalanced-tool-call-id` | conversation | error | high | per-file (JSONL) | — |
| `tool-result-without-tool-call` | conversation | error | high | per-file (JSONL) | — |
| `random-seed-not-set` | pipeline | warning | medium | per-file (Python AST) | idiomatic |
| `shuffle-after-split` | pipeline | error | medium | per-file (Python AST) | — |
| `dedup-key-misses-normalization` | pipeline | warning | low | per-file (Python AST) | — |
| `train-eval-overlap` | leakage | error | high | corpus-scope (`--train`/`--eval`) | — |
| `cross-dataset-overlap` | leakage | error | high | corpus-scope (`--dataset`) | — |
| `system-prompt-leaks-eval-instructions` | leakage | warning | medium | per-file (JSONL) | — |
| `privacy-pii-detected` | file | error | medium | per-file (JSONL) | — |

Outputs: JSON (default), SARIF 2.1.0, self-contained HTML, `drops` (row-removal manifest: `path<TAB>row<TAB>rules`). Per-rule and global enable/disable via `datalint.yml`. Corpus-scope dispatch via `--train`/`--eval` (2-way) or `--dataset NAME=PATH[,PATH...]` (N-way pairwise). CI: `--fail-on={none,info,warning,error}` for exit codes; `--min-severity={...}` for output filtering. Diff mode: `--diff-old` / `--diff-new` reports row-count delta, field-set delta, per-field top-value distribution shifts, and linearly-interpolated character-length percentiles (count / mean / min / p50 / p90 / p99 / max); `--diff-format=text|json` (text default).

### IDE / agent integrations

- **`datalint-lsp`** — Language Server speaking JSON-RPC 2.0 over stdio. Capabilities: `textDocumentSync` (Incremental), `diagnosticProvider`, `codeActionProvider`. Lints on `didOpen` / `didChange` (live, against the in-memory buffer for Python; range-bearing changes splice into the buffer using UTF-16 character offsets per the LSP default) and `didSave`. Auto-fixes surface as `quickfix` code actions — same edits the CLI's `--fix` would apply.
- **`datalint-mcp`** — Model Context Protocol server with newline-delimited JSON-RPC 2.0 over stdio. Surface:
  - **Tools**: `lint` (returns findings as a JSON text block) and `fix` (lints, applies fixes via `internal/fixer`, returns a summary plus the pre-fix findings).
  - **Resources**: `datalint:rules/index` (Markdown table of every registered rule) and `datalint:config/example` (annotated `datalint.yml` covering every config knob).
  - **Prompts**: three templates that return system+user message pairs for the agent to specialize on. `explain-rule` (`rule_id`) — explain a rule's bug class, why it matters, and what to do when it fires. `draft-fix` (`rule_id`, `path`, optional `line` / `row` / `message`) — draft a unified-diff patch for code findings or a row replacement/removal for data findings. `review-corpus` (`paths`, optional `dataset_names` / `goals`) — suggest a starting datalint configuration plus the CLI commands to run on the listed corpus.

## Rule taxonomy

**Schema discipline**
- `field-type-mixed-across-rows` — `score` is float in 99% of rows and string in 1%.
- `optional-field-required-by-downstream` — fields almost-always present (presence-ratio heuristic; explicit-schema declaration is a follow-up).
- `enum-drift` — new label appears mid-file with no schema update.

**Conversation/tool-call hygiene**
- `role-inversion` — `assistant` follows `assistant` with no `user` in between.
- `tool-result-without-tool-call` — `tool` role with no preceding `tool_use`.
- `system-message-mid-conversation`.
- `unbalanced-tool-call-id` — `tool_use_id` referenced but never opened.

**Leakage**
- `train-eval-overlap` — exact or near-duplicate prompts appear in both splits (MinHash + 32×4 LSH bands).
- `cross-dataset-overlap` — N-way pairwise generalization of `train-eval-overlap` for projects with more than two splits (`--dataset NAME=PATH[,PATH...]`).
- `eval-prompt-in-pretrain-corpus` — given an eval set, detect contamination in a training shard. *(covered by `train-eval-overlap` / `cross-dataset-overlap` with renamed flags; dedicated rule is a follow-up)*
- `system-prompt-leaks-eval-instructions`.

**Pipeline code**
- `random-seed-not-set` — split function uses unseeded RNG. Auto-fix inserts a seed call after the last import.
- `shuffle-after-split` — order corruption, breaks reproducibility.
- `dedup-key-misses-normalization` — dedup runs before unicode/whitespace normalization, undercounts duplicates.

**File-level**
- `jsonl-malformed-line` — non-JSON line, pinpointed.
- `parquet-row-group-too-large-for-streaming` — row group's `NumRows` exceeds the streaming-friendly threshold.
- `privacy-pii-detected` — string fields match email / US SSN / phone / credit-card patterns; project-specific patterns added via `extra_patterns`.
- `mds-shard-size-imbalanced` — *(not yet implemented; needs MDS reader)*

## Architecture

- **Go**, tree-sitter Python (for pipeline code) + JSONL streaming + Parquet metadata read in Go. MDS / WebDataset on the roadmap.
- **Two layers**:
  1. *Code rules* — same shape as Krit, walk Python AST to flag pipeline mistakes.
  2. *Data rules* — stream the dataset, compute row-level + corpus-level stats, emit findings with line/row pointers.
- **Capability gates** — `NeedsCorpusScan`, `NeedsLSH`, `NeedsExternalEvalSet`, `NeedsPythonAST`, `NeedsJSONL`, `NeedsParquet`. Declared on each rule; the dispatcher routes per-file vs corpus-scope accordingly.
- **Outputs**: JSON, SARIF 2.1.0, HTML report, drops (per-row removal manifest).
- **Autofix tiers** — `cosmetic`, `idiomatic`, `semantic`. `random-seed-not-set` emits an `idiomatic` fix; the `--fix` flag applies dedup'd edits in reverse-line order. The same fix surfaces through LSP `textDocument/codeAction` and the MCP `fix` tool.
- **LSP server** — incremental `didOpen` / `didChange` / `didSave` / `didClose` lifecycle, in-memory buffer store for live linting Python, `quickfix` code actions for fixes in the editor's selected range.
- **MCP server** — `tools/list` + `tools/call` for `lint` and `fix`; `resources/list` + `resources/read` for the rules-index Markdown and the annotated config example; `prompts/list` + `prompts/get` for `explain-rule`, `draft-fix`, and `review-corpus`. Same rule pipeline as the CLI.

## MVP

1. Skeleton + tree-sitter Python. ✓
2. JSONL streaming reader with row-pointer findings. ✓
3. Five rules (mix of code + data + leakage). ✓ (sixteen)
4. HTML report. ✓
5. CI on a public RLHF corpus (e.g. HH-RLHF, UltraFeedback) — hand-label to compare. *(internal smoke corpus covers regression at small scale; full public-corpus run is a follow-up)*

## Stretch

- **MDS, WebDataset** support — Parquet landed, MDS is the remaining file format.
- **Auto-fix on more rules** — currently only `random-seed-not-set` emits one.
- **Explicit schema declarations** — turn `optional-field-required-by-downstream` from a presence-ratio heuristic into a literal schema-vs-data check.
- **Per-rowgroup byte heuristic** for the parquet rule (waits for an upstream API surface).
- **Language mix shifts** in diff mode — top-value and length-percentile shifts landed; language profiles are the next data dimension.
- **More MCP prompts** — `explain-rule`, `draft-fix`, and `review-corpus` ship today; further templates (e.g. a per-finding triage walkthrough) are an open follow-up.

## Why this is the right shape

Training-data bugs are almost universally caught after the fact, by an eval regression or — worse — a release. The cost asymmetry is enormous: catching a leakage issue at lint time saves a training run. Krit's incremental + capability-gated architecture is exactly right because data-rule passes can be expensive (corpus-wide MinHash) and you don't want to run them on every CI commit.

## Non-goals

- Training framework integration.
- Quality scoring of individual examples (that's reward-model territory).
- Replacing existing data validation libs (Great Expectations, Pandera) — datalint complements them by focusing on LLM-data-specific failure modes.
