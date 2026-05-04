package scanner

import (
	"fmt"
	"os"

	"github.com/parquet-go/parquet-go"
)

// ParquetFile is a parsed Parquet file. v0 exposes only the row-group
// metadata the file-level rule needs; streaming row data through the
// rule pipeline is a future expansion.
type ParquetFile struct {
	Path      string
	NumRows   int64
	RowGroups []ParquetRowGroup
}

// ParquetRowGroup carries the row count for one group. v0 uses
// NumRows as a proxy for streaming cost — a row group with 10M rows
// is too big to stream regardless of how many bytes it compresses to,
// and most pipelines target ≤1M rows per group as the rule of thumb.
// On-disk bytes is a follow-up once parquet-go exposes a stable
// per-rowgroup size accessor.
type ParquetRowGroup struct {
	Index   int
	NumRows int64
}

// ParseParquet reads only the file's metadata — no row data — and
// returns the row-group manifest.
func ParseParquet(path string) (*ParquetFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	pf, err := parquet.OpenFile(f, stat.Size())
	if err != nil {
		return nil, fmt.Errorf("parse parquet %s: %w", path, err)
	}
	groups := make([]ParquetRowGroup, 0, len(pf.RowGroups()))
	for i, rg := range pf.RowGroups() {
		groups = append(groups, ParquetRowGroup{
			Index:   i,
			NumRows: rg.NumRows(),
		})
	}
	return &ParquetFile{
		Path:      path,
		NumRows:   pf.NumRows(),
		RowGroups: groups,
	}, nil
}
