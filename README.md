# datalint — a static linter for RLHF / RLAIF / SFT data pipelines

## What you're building

Training-data quality is the silent killer of model quality. The bugs are mundane: train/eval leakage from a sloppy split, label drift after a schema migration, prompt-template version skew across rows of the same dataset, malformed tool-call traces, conversation-turn role inversions, hidden duplicates from a normalization mismatch. Most of this is mechanically detectable, and yet the standard stack (pandas + ad-hoc scripts + a notebook) catches almost none of it pre-train.

datalint is a Krit-shaped static analyzer for training-data pipelines: it lints the *code* that produces the data, lints the *schemas* the data declares, and lints the *files themselves* (JSONL today; Parquet, MDS, WebDataset are on the roadmap). Read `~/kaeawc/krit/CLAUDE.md` first; you're reusing the architecture pattern.

## Quickstart

```bash
make build                                                      # produces ./datalint
./datalint tests/fixtures/jsonl-malformed-line/positive.jsonl    # JSON output by default
./datalint --format=html  ...  > report.html                     # self-contained HTML
./datalint --format=sarif ...                                   # SARIF 2.1.0 for code-scanning
./datalint --train train.jsonl --eval eval.jsonl                 # corpus-scope leakage rules
./datalint --config datalint.yml ...                             # custom thresholds & filters
```

A minimal `datalint.yml`:

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
  train-eval-overlap:
    prompt_field: input
    near_dup_threshold: 0.85
  system-prompt-leaks-eval-instructions:
    extra_patterns:
      - "(?i)reply with one of"
      - "MMLU"
```

## Status

Twelve rules implemented across all five README categories; configurable thresholds, enable/disable lists, three output formats, MinHash-based near-duplicate detection.

| ID | Category | Severity | Confidence | Source |
|---|---|---|---|---|
| `jsonl-malformed-line` | file | error | high | per-file (JSONL) |
| `field-type-mixed-across-rows` | schema | warning | high | per-file (JSONL) |
| `enum-drift` | schema | warning | medium | per-file (JSONL) |
| `role-inversion` | conversation | error | high | per-file (JSONL) |
| `system-message-mid-conversation` | conversation | error | high | per-file (JSONL) |
| `unbalanced-tool-call-id` | conversation | error | high | per-file (JSONL) |
| `tool-result-without-tool-call` | conversation | error | high | per-file (JSONL) |
| `random-seed-not-set` | pipeline | warning | medium | per-file (Python AST) |
| `shuffle-after-split` | pipeline | error | medium | per-file (Python AST) |
| `dedup-key-misses-normalization` | pipeline | warning | low | per-file (Python AST) |
| `train-eval-overlap` | leakage | error | high | corpus-scope |
| `system-prompt-leaks-eval-instructions` | leakage | warning | medium | per-file (JSONL) |

Outputs: JSON (default), SARIF 2.1.0, self-contained HTML report. Per-rule and global enable/disable via `datalint.yml`. Corpus-scope dispatch via `--train` / `--eval` flags.

## Rule taxonomy

**Schema discipline**
- `field-type-mixed-across-rows` — `score` is float in 99% of rows and string in 1%.
- `optional-field-required-by-downstream` — schema marks optional, downstream consumer crashes on missing. *(not yet implemented)*
- `enum-drift` — new label appears mid-file with no schema update.

**Conversation/tool-call hygiene**
- `role-inversion` — `assistant` follows `assistant` with no `user` in between.
- `tool-result-without-tool-call` — `tool` role with no preceding `tool_use`.
- `system-message-mid-conversation`.
- `unbalanced-tool-call-id` — `tool_use_id` referenced but never opened.

**Leakage**
- `train-eval-overlap` — exact or near-duplicate prompts appear in both splits (MinHash; LSH bands are a follow-up).
- `eval-prompt-in-pretrain-corpus` — given an eval set, detect contamination in a training shard. *(covered by train-eval-overlap with renamed flags; dedicated rule is a follow-up)*
- `system-prompt-leaks-eval-instructions`.

**Pipeline code**
- `random-seed-not-set` — split function uses unseeded RNG.
- `shuffle-after-split` — order corruption, breaks reproducibility.
- `dedup-key-misses-normalization` — dedup runs before unicode/whitespace normalization, undercounts duplicates.

**File-level**
- `jsonl-malformed-line` — non-JSON line, pinpointed.
- `parquet-row-group-too-large-for-streaming`. *(not yet implemented)*
- `mds-shard-size-imbalanced`. *(not yet implemented)*

## Architecture

- **Go**, tree-sitter Python (for pipeline code) + JSONL streaming ingestion in Go. Parquet/MDS/WebDataset on the roadmap.
- **Two layers**:
  1. *Code rules* — same shape as Krit, walk Python AST to flag pipeline mistakes.
  2. *Data rules* — stream the dataset, compute row-level + corpus-level stats, emit findings with line/row pointers.
- **Capability gates** — `NeedsCorpusScan`, `NeedsLSH`, `NeedsExternalEvalSet`, `NeedsPythonAST`, `NeedsJSONL`, `NeedsParquet`. Declared on each rule; the dispatcher routes per-file vs corpus-scope accordingly.
- **Outputs**: JSON, SARIF 2.1.0, HTML report (LSP, MCP planned).
- **Autofix tiers** — `cosmetic`, `idiomatic`, `semantic`. Schema in place; no rule currently emits a fix.

## MVP

1. Skeleton + tree-sitter Python. ✓
2. JSONL streaming reader with row-pointer findings. ✓
3. Five rules (mix of code + data + leakage). ✓ (twelve)
4. HTML report. ✓
5. CI on a public RLHF corpus (e.g. HH-RLHF, UltraFeedback) — hand-label to compare. *(open)*

## Stretch

- **Parquet, MDS, WebDataset** support.
- **LSH bands** for sublinear near-duplicate lookup (currently O(M*N) per eval row).
- **Cross-dataset analysis** — three datasets in, find which have overlapping prompts.
- **Diff mode** — between two versions of a dataset, show what changed in distribution (label balance, length, language mix).
- **Active suggestion** — propose specific rows to drop, with reasons.
- **Privacy scan** — flag rows with PII patterns that shouldn't be in training data.
- **LSP / MCP servers** — `cmd/datalint-lsp` and `cmd/datalint-mcp` are skeletons.
- **Suppression mechanism** — per-line / per-file rule suppression (e.g. `# datalint:disable=random-seed-not-set`).
- **Auto-fix** — at least one rule emitting a `cosmetic` or `idiomatic` fix.

## Why this is the right shape

Training-data bugs are almost universally caught after the fact, by an eval regression or — worse — a release. The cost asymmetry is enormous: catching a leakage issue at lint time saves a training run. Krit's incremental + capability-gated architecture is exactly right because data-rule passes can be expensive (corpus-wide MinHash) and you don't want to run them on every CI commit.

## Non-goals

- Training framework integration.
- Quality scoring of individual examples (that's reward-model territory).
- Replacing existing data validation libs (Great Expectations, Pandera) — datalint complements them by focusing on LLM-data-specific failure modes.
