package scanner_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/kaeawc/datalint/internal/scanner"
)

// rowSchema is the row shape used by every parquet test fixture in
// this file. Keeping it tiny keeps generated parquet bytes small.
type rowSchema struct {
	ID   int64  `parquet:"id"`
	Name string `parquet:"name"`
}

// writeParquet creates a parquet file at path with rowsPerGroup rows
// per group across groups groups, returning the total row count.
func writeParquet(t *testing.T, path string, groups, rowsPerGroup int) int64 {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	w := parquet.NewGenericWriter[rowSchema](f, parquet.MaxRowsPerRowGroup(int64(rowsPerGroup)))
	rows := make([]rowSchema, rowsPerGroup)
	for i := range rows {
		rows[i] = rowSchema{ID: int64(i), Name: "x"}
	}
	total := int64(0)
	for g := 0; g < groups; g++ {
		n, err := w.Write(rows)
		if err != nil {
			t.Fatalf("write group %d: %v", g, err)
		}
		total += int64(n)
		if err := w.Flush(); err != nil {
			t.Fatalf("flush group %d: %v", g, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return total
}

func TestParseParquet_SingleGroup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "single.parquet")
	total := writeParquet(t, path, 1, 50)

	pf, err := scanner.ParseParquet(path)
	if err != nil {
		t.Fatalf("ParseParquet: %v", err)
	}
	if pf.NumRows != total {
		t.Errorf("NumRows = %d, want %d", pf.NumRows, total)
	}
	if len(pf.RowGroups) != 1 {
		t.Fatalf("RowGroups len = %d, want 1", len(pf.RowGroups))
	}
	if pf.RowGroups[0].NumRows != 50 {
		t.Errorf("group[0].NumRows = %d, want 50", pf.RowGroups[0].NumRows)
	}
}

func TestParseParquet_MultipleGroups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.parquet")
	writeParquet(t, path, 3, 100)

	pf, err := scanner.ParseParquet(path)
	if err != nil {
		t.Fatalf("ParseParquet: %v", err)
	}
	if len(pf.RowGroups) != 3 {
		t.Fatalf("RowGroups len = %d, want 3", len(pf.RowGroups))
	}
	for i, rg := range pf.RowGroups {
		if rg.Index != i {
			t.Errorf("group[%d].Index = %d", i, rg.Index)
		}
		if rg.NumRows != 100 {
			t.Errorf("group[%d].NumRows = %d, want 100", i, rg.NumRows)
		}
	}
}

func TestParseParquet_FileNotFound(t *testing.T) {
	if _, err := scanner.ParseParquet("/nonexistent-parquet-for-test.parquet"); err == nil {
		t.Fatal("expected error for missing file")
	}
}
