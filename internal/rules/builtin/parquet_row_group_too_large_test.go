package builtin_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/pipeline"
	_ "github.com/kaeawc/datalint/internal/rules/builtin"
)

const parquetRowGroupRuleID = "parquet-row-group-too-large-for-streaming"

type parquetRow struct {
	ID int64 `parquet:"id"`
}

func writeParquetRowGroups(t *testing.T, path string, groups, rowsPerGroup int) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	w := parquet.NewGenericWriter[parquetRow](f, parquet.MaxRowsPerRowGroup(int64(rowsPerGroup)))
	rows := make([]parquetRow, rowsPerGroup)
	for i := range rows {
		rows[i] = parquetRow{ID: int64(i)}
	}
	for g := 0; g < groups; g++ {
		if _, err := w.Write(rows); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := w.Flush(); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestParquetRowGroupTooLarge_Default(t *testing.T) {
	// Default threshold is 1M rows. A 100-row file fires nothing.
	dir := t.TempDir()
	path := filepath.Join(dir, "ok.parquet")
	writeParquetRowGroups(t, path, 1, 100)

	got := findingsForRule(t, path, parquetRowGroupRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d: %s", len(got), joinMessages(got))
	}
}

func TestParquetRowGroupTooLarge_LowConfigThreshold(t *testing.T) {
	// Three groups of 100 rows each. With max_rows_per_group=50,
	// every group exceeds → 3 findings, one per group.
	dir := t.TempDir()
	path := filepath.Join(dir, "big.parquet")
	writeParquetRowGroups(t, path, 3, 100)

	cfg := config.Default()
	cfg.Rules[parquetRowGroupRuleID] = map[string]any{"max_rows_per_group": 50}

	all, err := pipeline.Run([]string{path}, cfg)
	if err != nil {
		t.Fatalf("pipeline.Run: %v", err)
	}
	var got []diag.Finding
	for _, f := range all {
		if f.RuleID == parquetRowGroupRuleID {
			got = append(got, f)
		}
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 findings, got %d: %s", len(got), joinMessages(got))
	}
	for i, f := range got {
		if !strings.Contains(f.Message, "100 rows") {
			t.Errorf("finding %d message missing row count: %q", i, f.Message)
		}
	}
}

func TestParquetRowGroupTooLarge_AtThresholdNoFire(t *testing.T) {
	// Group has exactly 50 rows; threshold is 50; rule must not fire
	// (the rule's condition is strictly >, not >=).
	dir := t.TempDir()
	path := filepath.Join(dir, "exactly.parquet")
	writeParquetRowGroups(t, path, 1, 50)

	cfg := config.Default()
	cfg.Rules[parquetRowGroupRuleID] = map[string]any{"max_rows_per_group": 50}

	all, err := pipeline.Run([]string{path}, cfg)
	if err != nil {
		t.Fatalf("pipeline.Run: %v", err)
	}
	for _, f := range all {
		if f.RuleID == parquetRowGroupRuleID {
			t.Errorf("at-threshold should not fire: %+v", f)
		}
	}
}
