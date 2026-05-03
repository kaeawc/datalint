# datalint — a static linter for RLHF / RLAIF / SFT data pipelines

## What you're building

Training-data quality is the silent killer of model quality. The bugs are mundane: train/eval leakage from a sloppy split, label drift after a schema migration, prompt-template version skew across rows of the same dataset, malformed tool-call traces, conversation-turn role inversions, hidden duplicates from a normalization mismatch. Most of this is mechanically detectable, and yet the standard stack (pandas + ad-hoc scripts + a notebook) catches almost none of it pre-train.

datalint is a Krit-shaped static analyzer for training-data pipelines: it lints the *code* that produces the data, lints the *schemas* the data declares, and lints the *files themselves* (JSONL, Parquet, MDS, WebDataset). Read `~/kaeawc/krit/CLAUDE.md` first; you're reusing the architecture pattern.

## Rule taxonomy

**Schema discipline**
- `field-type-mixed-across-rows` — `score` is float in 99% of rows and string in 1%.
- `optional-field-required-by-downstream` — schema marks optional, downstream consumer crashes on missing.
- `enum-drift` — new label appears mid-file with no schema update.

**Conversation/tool-call hygiene**
- `role-inversion` — `assistant` follows `assistant` with no `user` in between.
- `tool-result-without-tool-call` — `tool` role with no preceding `tool_use`.
- `system-message-mid-conversation`.
- `unbalanced-tool-call-id` — `tool_use_id` referenced but never opened.

**Leakage**
- `train-eval-overlap` — exact or near-duplicate prompts appear in both splits (MinHash + LSH).
- `eval-prompt-in-pretrain-corpus` — given an eval set, detect contamination in a training shard.
- `system-prompt-leaks-eval-instructions`.

**Pipeline code**
- `random-seed-not-set` — split function uses unseeded RNG.
- `shuffle-after-split` — order corruption, breaks reproducibility.
- `dedup-key-misses-normalization` — dedup runs before unicode/whitespace normalization, undercounts duplicates.

**File-level**
- `jsonl-malformed-line` — non-JSON line, pinpointed.
- `parquet-row-group-too-large-for-streaming`.
- `mds-shard-size-imbalanced`.

## Architecture

- **Go**, tree-sitter Python (for pipeline code) + JSONL/Parquet/Arrow ingestion in Go.
- **Two layers**:
  1. *Code rules* — same shape as Krit, walk Python AST to flag pipeline mistakes.
  2. *Data rules* — stream the dataset, compute row-level + corpus-level stats, emit findings with line/row pointers.
- **Capability gates** — `NeedsCorpusScan` (expensive, opt-in), `NeedsLSH` (loads MinHash index), `NeedsExternalEvalSet` (compares against a pinned eval corpus for leakage).
- **Outputs**: SARIF, JSON, HTML report (with histograms — corpus stats are most legible visually), LSP, MCP server.
- **Autofix tiers** — `cosmetic` (whitespace normalize), `idiomatic` (add missing field with default), `semantic` (rewrite the split function to use a seeded RNG). Semantic fixes only on pipeline code, never on data rows.

## MVP

1. Skeleton + tree-sitter Python.
2. JSONL streaming reader with row-pointer findings.
3. Five rules (mix of code + data + leakage).
4. HTML report.
5. CI on a public RLHF corpus (e.g. HH-RLHF, UltraFeedback) — hand-label to compare.

## Stretch

- **Parquet, MDS, WebDataset** support.
- **Cross-dataset analysis** — three datasets in, find which have overlapping prompts.
- **Diff mode** — between two versions of a dataset, show what changed in distribution (label balance, length, language mix).
- **Active suggestion** — propose specific rows to drop, with reasons.
- **Privacy scan** — flag rows with PII patterns that shouldn't be in training data.

## Why this is the right shape

Training-data bugs are almost universally caught after the fact, by an eval regression or — worse — a release. The cost asymmetry is enormous: catching a leakage issue at lint time saves a training run. Krit's incremental + capability-gated architecture is exactly right because data-rule passes can be expensive (corpus-wide MinHash) and you don't want to run them on every CI commit.

## Non-goals

- Training framework integration.
- Quality scoring of individual examples (that's reward-model territory).
- Replacing existing data validation libs (Great Expectations, Pandera) — datalint complements them by focusing on LLM-data-specific failure modes.
