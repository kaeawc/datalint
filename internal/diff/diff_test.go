package diff_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/diff"
)

func writeJSONL(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCompute_RowCountAndFieldDelta(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.jsonl")
	newPath := filepath.Join(dir, "new.jsonl")

	// old: 3 rows, fields {id, name, score}
	writeJSONL(t, oldPath,
		"{\"id\": 1, \"name\": \"a\", \"score\": 0.1}\n"+
			"{\"id\": 2, \"name\": \"b\", \"score\": 0.2}\n"+
			"{\"id\": 3, \"name\": \"c\", \"score\": 0.3}\n")

	// new: 5 rows, fields {id, name, label} — score removed, label added
	writeJSONL(t, newPath,
		"{\"id\": 1, \"name\": \"a\", \"label\": \"x\"}\n"+
			"{\"id\": 2, \"name\": \"b\", \"label\": \"y\"}\n"+
			"{\"id\": 3, \"name\": \"c\", \"label\": \"z\"}\n"+
			"{\"id\": 4, \"name\": \"d\", \"label\": \"x\"}\n"+
			"{\"id\": 5, \"name\": \"e\", \"label\": \"y\"}\n")

	r, err := diff.Compute(oldPath, newPath)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	if r.OldRows != 3 {
		t.Errorf("OldRows = %d, want 3", r.OldRows)
	}
	if r.NewRows != 5 {
		t.Errorf("NewRows = %d, want 5", r.NewRows)
	}
	if !equalStrings(r.Added, []string{"label"}) {
		t.Errorf("Added = %v, want [label]", r.Added)
	}
	if !equalStrings(r.Removed, []string{"score"}) {
		t.Errorf("Removed = %v, want [score]", r.Removed)
	}
	if !equalStrings(r.Common, []string{"id", "name"}) {
		t.Errorf("Common = %v, want [id, name]", r.Common)
	}
}

func TestCompute_MalformedRowsSkipped(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.jsonl")
	newPath := filepath.Join(dir, "new.jsonl")
	writeJSONL(t, oldPath, "{\"id\": 1}\nnot json\n{\"id\": 2}\n")
	writeJSONL(t, newPath, "{\"id\": 1}\n")

	r, err := diff.Compute(oldPath, newPath)
	if err != nil {
		t.Fatal(err)
	}
	// Old has 2 parseable rows (the malformed one in the middle is
	// skipped), new has 1.
	if r.OldRows != 2 {
		t.Errorf("OldRows = %d, want 2 (malformed line skipped)", r.OldRows)
	}
	if r.NewRows != 1 {
		t.Errorf("NewRows = %d, want 1", r.NewRows)
	}
}

func TestCompute_EmptyFiles(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.jsonl")
	newPath := filepath.Join(dir, "new.jsonl")
	writeJSONL(t, oldPath, "")
	writeJSONL(t, newPath, "")

	r, err := diff.Compute(oldPath, newPath)
	if err != nil {
		t.Fatal(err)
	}
	if r.OldRows != 0 || r.NewRows != 0 {
		t.Errorf("rows = (%d, %d), want (0, 0)", r.OldRows, r.NewRows)
	}
	if len(r.Added) != 0 || len(r.Removed) != 0 || len(r.Common) != 0 {
		t.Errorf("expected empty field sets; added=%v removed=%v common=%v",
			r.Added, r.Removed, r.Common)
	}
}

func TestCompute_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.jsonl")
	writeJSONL(t, good, "{}\n")
	if _, err := diff.Compute("/nonexistent.jsonl", good); err == nil {
		t.Error("expected error when old path missing")
	}
	if _, err := diff.Compute(good, "/nonexistent.jsonl"); err == nil {
		t.Error("expected error when new path missing")
	}
}

func TestWriteText_DeltaSign(t *testing.T) {
	cases := []struct {
		name      string
		report    diff.Report
		wantInOut []string
		notInOut  []string
	}{
		{
			name:   "growth uses + sign",
			report: diff.Report{OldPath: "a", NewPath: "b", OldRows: 5, NewRows: 8, Added: []string{"x"}},
			wantInOut: []string{
				"old: a  (5 rows)",
				"new: b  (8 rows; +3)",
				"fields added:    [x]",
			},
		},
		{
			name:   "shrinkage uses - sign (no extra +)",
			report: diff.Report{OldPath: "a", NewPath: "b", OldRows: 10, NewRows: 4, Removed: []string{"y"}},
			wantInOut: []string{
				"new: b  (4 rows; -6)",
				"fields removed:  [y]",
			},
			notInOut: []string{"+-6"},
		},
		{
			name:   "zero delta",
			report: diff.Report{OldPath: "a", NewPath: "b", OldRows: 7, NewRows: 7},
			wantInOut: []string{
				"new: b  (7 rows; 0)",
			},
			notInOut: []string{"+0", "-0"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := diff.WriteText(&buf, c.report); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			for _, want := range c.wantInOut {
				if !strings.Contains(out, want) {
					t.Errorf("missing %q in:\n%s", want, out)
				}
			}
			for _, unwanted := range c.notInOut {
				if strings.Contains(out, unwanted) {
					t.Errorf("unexpected %q in:\n%s", unwanted, out)
				}
			}
		})
	}
}

func TestCompute_DistributionsCommonField(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.jsonl")
	newPath := filepath.Join(dir, "new.jsonl")

	// "label" is enum-like in both; "name" is high-cardinality.
	writeJSONL(t, oldPath,
		"{\"name\": \"alice\", \"label\": \"good\"}\n"+
			"{\"name\": \"bob\", \"label\": \"good\"}\n"+
			"{\"name\": \"carol\", \"label\": \"bad\"}\n"+
			"{\"name\": \"dan\", \"label\": \"good\"}\n")
	writeJSONL(t, newPath,
		"{\"name\": \"erin\", \"label\": \"bad\"}\n"+
			"{\"name\": \"frank\", \"label\": \"medium\"}\n"+
			"{\"name\": \"gina\", \"label\": \"good\"}\n"+
			"{\"name\": \"henry\", \"label\": \"medium\"}\n")

	r, err := diff.Compute(oldPath, newPath)
	if err != nil {
		t.Fatal(err)
	}

	var labelDist *diff.FieldDistribution
	for i := range r.Distributions {
		if r.Distributions[i].Field == "label" {
			labelDist = &r.Distributions[i]
			break
		}
	}
	if labelDist == nil {
		t.Fatalf("expected 'label' in distributions; got %+v", r.Distributions)
	}

	if len(labelDist.OldTop) != 2 {
		t.Fatalf("old top len = %d, want 2", len(labelDist.OldTop))
	}
	if labelDist.OldTop[0].Value != "good" || labelDist.OldTop[0].Count != 3 {
		t.Errorf("old top[0] = %+v, want {good 3}", labelDist.OldTop[0])
	}

	if len(labelDist.NewTop) != 3 {
		t.Fatalf("new top len = %d, want 3", len(labelDist.NewTop))
	}
	if labelDist.NewTop[0].Value != "medium" || labelDist.NewTop[0].Count != 2 {
		t.Errorf("new top[0] = %+v, want {medium 2}", labelDist.NewTop[0])
	}
	// "bad" and "good" both have count 1 — alphabetical tiebreak puts "bad" first.
	if labelDist.NewTop[1].Value != "bad" {
		t.Errorf("new top[1] = %+v, want {bad 1}", labelDist.NewTop[1])
	}
}

func TestCompute_DistributionsHighCardinalityFieldOmitsTopButKeepsLength(t *testing.T) {
	// Free-text fields (more than MaxDistinctForDistribution unique
	// values) get a FieldDistribution entry but with nil OldTop /
	// NewTop — top-K of thousands isn't useful. Length stats are
	// always populated when string occurrences exist; they're
	// useful for free-text fields ("prompts grew 3× longer").
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.jsonl")
	newPath := filepath.Join(dir, "new.jsonl")

	var oldBody, newBody strings.Builder
	for i := 0; i < 25; i++ {
		oldBody.WriteString("{\"name\":\"u" + strconv.Itoa(i) + "\"}\n")
		newBody.WriteString("{\"name\":\"u" + strconv.Itoa(i+5) + "\"}\n")
	}
	writeJSONL(t, oldPath, oldBody.String())
	writeJSONL(t, newPath, newBody.String())

	r, err := diff.Compute(oldPath, newPath)
	if err != nil {
		t.Fatal(err)
	}
	var nameDist *diff.FieldDistribution
	for i := range r.Distributions {
		if r.Distributions[i].Field == "name" {
			nameDist = &r.Distributions[i]
			break
		}
	}
	if nameDist == nil {
		t.Fatalf("'name' should be in distributions even when high-cardinality (length stats and scripts still informative)")
	}
	if len(nameDist.OldTop) != 0 || len(nameDist.NewTop) != 0 {
		t.Errorf("high-cardinality field should have nil OldTop/NewTop; got old=%v new=%v",
			nameDist.OldTop, nameDist.NewTop)
	}
	if nameDist.OldLength.Count == 0 || nameDist.NewLength.Count == 0 {
		t.Errorf("length stats should be populated; got old=%+v new=%+v",
			nameDist.OldLength, nameDist.NewLength)
	}
}

func TestCompute_DistributionsSkipsNonStringField(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.jsonl")
	newPath := filepath.Join(dir, "new.jsonl")

	// "id" is numeric in both; should not appear in distributions
	// even though it's a shared field — only string values get counted.
	writeJSONL(t, oldPath, "{\"id\": 1}\n{\"id\": 2}\n")
	writeJSONL(t, newPath, "{\"id\": 3}\n{\"id\": 4}\n")

	r, err := diff.Compute(oldPath, newPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, fd := range r.Distributions {
		if fd.Field == "id" {
			t.Errorf("'id' (numeric) should be skipped from distributions")
		}
	}
}

func TestWriteText_RendersDistributionSection(t *testing.T) {
	r := diff.Report{
		OldPath: "old.jsonl",
		NewPath: "new.jsonl",
		OldRows: 4,
		NewRows: 4,
		Common:  []string{"label"},
		Distributions: []diff.FieldDistribution{
			{
				Field: "label",
				OldTop: []diff.ValueCount{
					{Value: "good", Count: 3},
					{Value: "bad", Count: 1},
				},
				NewTop: []diff.ValueCount{
					{Value: "medium", Count: 2},
					{Value: "good", Count: 1},
				},
			},
		},
	}
	var buf bytes.Buffer
	if err := diff.WriteText(&buf, r); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"field distributions",
		"label:",
		"old top:     [good:3, bad:1]",
		"new top:     [medium:2, good:1]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestWriteText_OmitsDistributionSectionWhenEmpty(t *testing.T) {
	// Pre-existing behavior pinned: a Report with no Distributions
	// must NOT print the section header.
	r := diff.Report{OldPath: "a", NewPath: "b", OldRows: 1, NewRows: 1}
	var buf bytes.Buffer
	if err := diff.WriteText(&buf, r); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "field distributions") {
		t.Errorf("section should be omitted when no distributions:\n%s", buf.String())
	}
}

func TestCompute_LengthStatsForCommonField(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.jsonl")
	newPath := filepath.Join(dir, "new.jsonl")

	// Old "label": values of length 4, 4, 3, 4 — sorted: [3, 4, 4, 4]
	//   mean = 15/4 = 3.75; min = 3; max = 4
	//   p50: rank = 0.50 * 3 = 1.5 → blend(sorted[1], sorted[2], 0.5) = 4.0
	//   p90: rank = 0.90 * 3 = 2.7 → blend(sorted[2], sorted[3], 0.7) = 4.0
	//   p99: rank = 0.99 * 3 = 2.97 → blend(sorted[2], sorted[3], 0.97) = 4.0
	// New "label": lengths 6, 6, 4 — sorted: [4, 6, 6]
	//   mean ≈ 5.33; min = 4; max = 6
	//   p50: rank = 0.50 * 2 = 1.0 → sorted[1] = 6.0
	//   p90: rank = 0.90 * 2 = 1.8 → blend(sorted[1], sorted[2], 0.8) = 6.0
	//   p99: rank = 0.99 * 2 = 1.98 → blend(sorted[1], sorted[2], 0.98) = 6.0
	writeJSONL(t, oldPath,
		"{\"label\": \"good\"}\n"+
			"{\"label\": \"good\"}\n"+
			"{\"label\": \"bad\"}\n"+
			"{\"label\": \"good\"}\n")
	writeJSONL(t, newPath,
		"{\"label\": \"medium\"}\n"+
			"{\"label\": \"medium\"}\n"+
			"{\"label\": \"good\"}\n")

	r, err := diff.Compute(oldPath, newPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Distributions) != 1 {
		t.Fatalf("distributions = %d, want 1", len(r.Distributions))
	}
	fd := r.Distributions[0]

	if fd.OldLength.Count != 4 {
		t.Errorf("old count = %d, want 4", fd.OldLength.Count)
	}
	if fd.OldLength.Mean != 3.75 {
		t.Errorf("old mean = %f, want 3.75", fd.OldLength.Mean)
	}
	if fd.OldLength.Min != 3 {
		t.Errorf("old min = %d, want 3", fd.OldLength.Min)
	}
	if fd.OldLength.Max != 4 {
		t.Errorf("old max = %d, want 4", fd.OldLength.Max)
	}
	if fd.OldLength.P50 != 4.0 {
		t.Errorf("old p50 = %f, want 4.0", fd.OldLength.P50)
	}
	if fd.OldLength.P90 != 4.0 {
		t.Errorf("old p90 = %f, want 4.0", fd.OldLength.P90)
	}
	if fd.OldLength.P99 != 4.0 {
		t.Errorf("old p99 = %f, want 4.0", fd.OldLength.P99)
	}

	if fd.NewLength.Count != 3 {
		t.Errorf("new count = %d, want 3", fd.NewLength.Count)
	}
	if fd.NewLength.Min != 4 {
		t.Errorf("new min = %d, want 4", fd.NewLength.Min)
	}
	if fd.NewLength.Max != 6 {
		t.Errorf("new max = %d, want 6", fd.NewLength.Max)
	}
	if fd.NewLength.P50 != 6.0 {
		t.Errorf("new p50 = %f, want 6.0", fd.NewLength.P50)
	}
	if fd.NewLength.P90 != 6.0 {
		t.Errorf("new p90 = %f, want 6.0", fd.NewLength.P90)
	}
}

// TestComputeLengthStats_LinearInterpolation pins the actual
// interpolation behaviour with a vector that exercises the blend.
// [4, 6] alone yields p50 = 5.0 (midway), which the old nearest-rank
// implementation would have rounded to 6.
func TestCompute_LengthStatsLinearInterpolation(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.jsonl")
	newPath := filepath.Join(dir, "new.jsonl")

	// Old "label": lengths 4, 6 — sorted: [4, 6]
	//   p50: rank = 0.5 * 1 = 0.5 → blend(sorted[0], sorted[1], 0.5) = 5.0
	//   p90: rank = 0.9 * 1 = 0.9 → blend(sorted[0], sorted[1], 0.9) = 5.8
	//   p99: rank = 0.99 * 1 → 5.98
	writeJSONL(t, oldPath, "{\"label\": \"good\"}\n{\"label\": \"medium\"}\n")
	writeJSONL(t, newPath, "{\"label\": \"good\"}\n{\"label\": \"medium\"}\n")

	r, err := diff.Compute(oldPath, newPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Distributions) != 1 {
		t.Fatalf("distributions = %d, want 1", len(r.Distributions))
	}
	old := r.Distributions[0].OldLength
	if old.P50 != 5.0 {
		t.Errorf("interpolated p50 = %f, want 5.0", old.P50)
	}
	if old.P90 < 5.79 || old.P90 > 5.81 {
		t.Errorf("interpolated p90 = %f, want ~5.8", old.P90)
	}
	if old.P99 < 5.97 || old.P99 > 5.99 {
		t.Errorf("interpolated p99 = %f, want ~5.98", old.P99)
	}
}

func TestWriteText_RendersLengthLines(t *testing.T) {
	r := diff.Report{
		OldPath: "a", NewPath: "b", OldRows: 1, NewRows: 1,
		Common: []string{"label"},
		Distributions: []diff.FieldDistribution{{
			Field:     "label",
			OldTop:    []diff.ValueCount{{Value: "good", Count: 1}},
			NewTop:    []diff.ValueCount{{Value: "medium", Count: 1}},
			OldLength: diff.LengthStats{Count: 1, Mean: 4.0, Min: 4, P50: 4.0, P90: 4.0, P99: 4.0, Max: 4},
			NewLength: diff.LengthStats{Count: 1, Mean: 6.0, Min: 6, P50: 6.0, P90: 6.0, P99: 6.0, Max: 6},
		}},
	}
	var buf bytes.Buffer
	if err := diff.WriteText(&buf, r); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"old length:  count=1 mean=4.0 min=4 p50=4.0 p90=4.0 p99=4.0 max=4",
		"new length:  count=1 mean=6.0 min=6 p50=6.0 p90=6.0 p99=6.0 max=6",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestWriteText_OmitsLengthLineWhenCountZero(t *testing.T) {
	// When one side has the field but it's never a string (or the
	// field appears in only one version), the other side's length
	// section should be omitted instead of rendering "count=0".
	r := diff.Report{
		OldPath: "a", NewPath: "b",
		Common: []string{"label"},
		Distributions: []diff.FieldDistribution{{
			Field:     "label",
			OldTop:    []diff.ValueCount{{Value: "good", Count: 1}},
			OldLength: diff.LengthStats{Count: 1, Mean: 4.0, Min: 4, P50: 4.0, P90: 4.0, P99: 4.0, Max: 4},
			// NewLength left zero-valued.
		}},
	}
	var buf bytes.Buffer
	if err := diff.WriteText(&buf, r); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "old length:") {
		t.Errorf("expected 'old length:' line:\n%s", out)
	}
	if strings.Contains(out, "new length:") {
		t.Errorf("'new length:' should be omitted when count=0:\n%s", out)
	}
}

func TestWriteJSON_RoundTrip(t *testing.T) {
	r := diff.Report{
		OldPath: "old.jsonl",
		NewPath: "new.jsonl",
		OldRows: 4,
		NewRows: 5,
		Added:   []string{"label"},
		Removed: []string{"score"},
		Common:  []string{"id", "name"},
		Distributions: []diff.FieldDistribution{
			{
				Field:  "label",
				OldTop: []diff.ValueCount{{Value: "good", Count: 3}, {Value: "bad", Count: 1}},
				NewTop: []diff.ValueCount{{Value: "medium", Count: 2}, {Value: "good", Count: 1}},
			},
		},
	}
	var buf bytes.Buffer
	if err := diff.WriteJSON(&buf, r); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var got diff.Report
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, buf.String())
	}
	if got.OldPath != r.OldPath || got.NewPath != r.NewPath {
		t.Errorf("paths round-tripped wrong: got=%+v", got)
	}
	if got.OldRows != 4 || got.NewRows != 5 {
		t.Errorf("rows round-tripped wrong: got=(%d,%d)", got.OldRows, got.NewRows)
	}
	if !equalStrings(got.Added, r.Added) || !equalStrings(got.Removed, r.Removed) || !equalStrings(got.Common, r.Common) {
		t.Errorf("field lists round-tripped wrong: got=%+v", got)
	}
	if len(got.Distributions) != 1 {
		t.Fatalf("distributions len = %d, want 1", len(got.Distributions))
	}
	d := got.Distributions[0]
	if d.Field != "label" {
		t.Errorf("distribution field = %q, want label", d.Field)
	}
	if len(d.OldTop) != 2 || d.OldTop[0].Value != "good" || d.OldTop[0].Count != 3 {
		t.Errorf("old top round-tripped wrong: %+v", d.OldTop)
	}
}

func TestCompute_ScriptMixSurfacedForLongStringField(t *testing.T) {
	// "text" field crosses MinRunesForScriptMix (50) on each side:
	// 5 rows × 26 latin letters = 130 latin runes per side. New side
	// also adds Cyrillic runes → mix shifts from 100% Latin to a
	// mixed Latin+Cyrillic profile.
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.jsonl")
	newPath := filepath.Join(dir, "new.jsonl")

	const latin = "abcdefghijklmnopqrstuvwxyz"
	const mixedNew = "abcdefghijklmnopqrstuvwxyz" + "пртпр" // 26 Latin + 5 Cyrillic
	var oldBody, newBody strings.Builder
	for i := 0; i < 5; i++ {
		oldBody.WriteString("{\"text\":\"" + latin + "\"}\n")
		newBody.WriteString("{\"text\":\"" + mixedNew + "\"}\n")
	}
	writeJSONL(t, oldPath, oldBody.String())
	writeJSONL(t, newPath, newBody.String())

	r, err := diff.Compute(oldPath, newPath)
	if err != nil {
		t.Fatal(err)
	}
	var fd *diff.FieldDistribution
	for i := range r.Distributions {
		if r.Distributions[i].Field == "text" {
			fd = &r.Distributions[i]
			break
		}
	}
	if fd == nil {
		t.Fatalf("'text' should be in distributions")
	}
	if len(fd.OldScripts) != 1 || fd.OldScripts[0].Script != "Latin" {
		t.Errorf("old scripts should be Latin-only; got %+v", fd.OldScripts)
	}
	if len(fd.NewScripts) < 2 {
		t.Fatalf("new scripts should include both Latin and Cyrillic; got %+v", fd.NewScripts)
	}
	if fd.NewScripts[0].Script != "Latin" {
		t.Errorf("new scripts should rank Latin first; got %+v", fd.NewScripts)
	}
	var cyrillicCount int
	for _, sc := range fd.NewScripts {
		if sc.Script == "Cyrillic" {
			cyrillicCount = sc.Count
		}
	}
	if cyrillicCount != 25 { // 5 rows × 5 cyrillic letters
		t.Errorf("expected 25 Cyrillic runes (5×5); got %d", cyrillicCount)
	}
}

func TestCompute_ScriptMixSuppressedForShortField(t *testing.T) {
	// Short enum fields (sum of letters < MinRunesForScriptMix on
	// BOTH sides) get nil OldScripts/NewScripts to avoid noise.
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.jsonl")
	newPath := filepath.Join(dir, "new.jsonl")
	writeJSONL(t, oldPath, "{\"label\":\"good\"}\n{\"label\":\"bad\"}\n")
	writeJSONL(t, newPath, "{\"label\":\"good\"}\n{\"label\":\"medium\"}\n")

	r, err := diff.Compute(oldPath, newPath)
	if err != nil {
		t.Fatal(err)
	}
	var fd *diff.FieldDistribution
	for i := range r.Distributions {
		if r.Distributions[i].Field == "label" {
			fd = &r.Distributions[i]
			break
		}
	}
	if fd == nil {
		t.Fatalf("'label' should be in distributions")
	}
	if fd.OldScripts != nil || fd.NewScripts != nil {
		t.Errorf("short field should suppress script mix; got old=%+v new=%+v",
			fd.OldScripts, fd.NewScripts)
	}
}

func TestWriteText_RendersScriptLines(t *testing.T) {
	r := diff.Report{
		OldPath: "a", NewPath: "b", OldRows: 1, NewRows: 1,
		Common: []string{"text"},
		Distributions: []diff.FieldDistribution{{
			Field: "text",
			OldScripts: []diff.ScriptCount{
				{Script: "Latin", Count: 100, Ratio: 1.0},
			},
			NewScripts: []diff.ScriptCount{
				{Script: "Latin", Count: 80, Ratio: 0.8},
				{Script: "Cyrillic", Count: 20, Ratio: 0.2},
			},
		}},
	}
	var buf bytes.Buffer
	if err := diff.WriteText(&buf, r); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"old scripts: [Latin:100 (100%)]",
		"new scripts: [Latin:80 (80%), Cyrillic:20 (20%)]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestWriteJSON_PrettyPrinted(t *testing.T) {
	// Pin the indented form so consumers diffing JSON output between
	// versions don't see spurious whitespace churn.
	var buf bytes.Buffer
	if err := diff.WriteJSON(&buf, diff.Report{OldPath: "a", NewPath: "b"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "\n  \"OldPath\"") {
		t.Errorf("expected 2-space indent on top-level keys; got:\n%s", buf.String())
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
