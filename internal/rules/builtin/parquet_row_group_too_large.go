package builtin

import (
	"fmt"

	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/rules"
)

func init() {
	rules.Register(&rules.Rule{
		ID:         "parquet-row-group-too-large-for-streaming",
		Category:   rules.CategoryFile,
		Severity:   diag.SeverityWarning,
		Confidence: rules.ConfidenceMedium,
		Fix:        rules.FixNone,
		Needs:      rules.NeedsParquet,
		Check:      checkParquetRowGroupTooLarge,
	})
}

// parquetRowGroupMaxRowsDefault is the per-group row count above which
// most streaming pipelines suffer (the row group has to be fully
// loaded before columnar projection can run). 1M rows lines up with
// the parquet community's typical recommendation of ≤256MB groups
// for moderately-sized rows.
const parquetRowGroupMaxRowsDefault = 1_000_000

func checkParquetRowGroupTooLarge(ctx *rules.Context, emit func(diag.Finding)) {
	if ctx == nil || ctx.Parquet == nil {
		return
	}
	maxRows := int64(ctx.Settings.Int("max_rows_per_group", parquetRowGroupMaxRowsDefault))
	for _, rg := range ctx.Parquet.RowGroups {
		if rg.NumRows <= maxRows {
			continue
		}
		emit(diag.Finding{
			RuleID:   "parquet-row-group-too-large-for-streaming",
			Severity: diag.SeverityWarning,
			Message: fmt.Sprintf(
				"row group %d has %d rows (>%d); streaming readers will buffer the whole group before projecting columns",
				rg.Index, rg.NumRows, maxRows),
			Location: diag.Location{Path: ctx.Parquet.Path, Row: rg.Index + 1},
		})
	}
}
