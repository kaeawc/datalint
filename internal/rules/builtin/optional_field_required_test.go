package builtin_test

import (
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/pipeline"
	"github.com/kaeawc/datalint/internal/testutil"
)

const optionalFieldRuleID = "optional-field-required-by-downstream"

// findingsForRuleWithConfig is the config-aware sibling of
// findingsForRule (defined in field_type_mixed_test.go). The schema
// tests below need to pass a per-rule config map for the explicit
// required_fields path; the default-config helper would lose that.
func findingsForRuleWithConfig(t *testing.T, path, id string, cfg config.Config) []diag.Finding {
	t.Helper()
	all, err := pipeline.Run([]string{path}, cfg)
	if err != nil {
		t.Fatalf("pipeline.Run: %v", err)
	}
	var out []diag.Finding
	for _, f := range all {
		if f.RuleID == id {
			out = append(out, f)
		}
	}
	return out
}

func TestOptionalFieldRequired_PartialMissing(t *testing.T) {
	// 6 rows. "name" present in 5/6 = 83.3%, above default 0.8
	// threshold and below 1.0 → flag at row 5 (the first missing).
	// "id" present in 6/6 = 100% → not flagged.
	path := testutil.Fixture(t, "optional-field-required-by-downstream/positive.jsonl")
	got := findingsForRule(t, path, optionalFieldRuleID)

	if len(got) != 1 {
		t.Fatalf("expected 1 finding, got %d: %s", len(got), joinMessages(got))
	}
	if got[0].Location.Row != 5 {
		t.Errorf("row = %d, want 5 (first row missing 'name')", got[0].Location.Row)
	}
	if !strings.Contains(got[0].Message, `"name"`) {
		t.Errorf("message should name the field: %q", got[0].Message)
	}
	if !strings.Contains(got[0].Message, "5/6") {
		t.Errorf("message should report the ratio: %q", got[0].Message)
	}
}

func TestOptionalFieldRequired_AllPresent(t *testing.T) {
	// Every row has every field → every ratio is 1.0 → no finding.
	path := testutil.Fixture(t, "optional-field-required-by-downstream/negative-stable.jsonl")
	got := findingsForRule(t, path, optionalFieldRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d: %s", len(got), joinMessages(got))
	}
}

func TestOptionalFieldRequired_HalfMissingBelowThreshold(t *testing.T) {
	// "name" present in 3/6 = 50%, below 0.8 threshold → no flag.
	// The rule deliberately excludes mostly-absent fields; that's a
	// different signal than "almost-always present, occasionally
	// missing".
	path := testutil.Fixture(t, "optional-field-required-by-downstream/negative-half-missing.jsonl")
	got := findingsForRule(t, path, optionalFieldRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings (50%% presence is below threshold), got %d: %s",
			len(got), joinMessages(got))
	}
}

func TestOptionalFieldRequired_TooShortToConclude(t *testing.T) {
	// 4 rows, below default min_rows=5 → rule stays silent even
	// though "name" is missing in 1 of 4.
	path := testutil.Fixture(t, "optional-field-required-by-downstream/negative-too-short.jsonl")
	got := findingsForRule(t, path, optionalFieldRuleID)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings (below min_rows), got %d: %s",
			len(got), joinMessages(got))
	}
}

func TestOptionalFieldRequired_SchemaRequiredFieldsMissing(t *testing.T) {
	// Negative-half-missing fixture: 6 rows, "name" present in 3/6
	// (50%, below default heuristic threshold). With required_fields:
	// ["name"] declared, the schema path fires regardless of ratio
	// because schema requires 100%.
	path := testutil.Fixture(t, "optional-field-required-by-downstream/negative-half-missing.jsonl")
	cfg := config.Default()
	cfg.Rules[optionalFieldRuleID] = map[string]any{
		"required_fields": []any{"name"},
	}

	got := findingsForRuleWithConfig(t, path, optionalFieldRuleID, cfg)
	if len(got) != 1 {
		t.Fatalf("expected 1 schema finding, got %d: %s", len(got), joinMessages(got))
	}
	if !strings.Contains(got[0].Message, `schema declares "name" as required`) {
		t.Errorf("message should cite schema; got %q", got[0].Message)
	}
	if !strings.Contains(got[0].Message, "3/6") {
		t.Errorf("message should report 3/6 missing; got %q", got[0].Message)
	}
	if got[0].Location.Row != 2 {
		t.Errorf("row = %d, want 2 (first row missing 'name')", got[0].Location.Row)
	}
}

func TestOptionalFieldRequired_SchemaRequiredFieldNeverPresent(t *testing.T) {
	// "score" is in required_fields but never appears in the fixture.
	// Schema path fires citing totalRows missing rows, anchored at row 1.
	path := testutil.Fixture(t, "optional-field-required-by-downstream/positive.jsonl")
	cfg := config.Default()
	cfg.Rules[optionalFieldRuleID] = map[string]any{
		"required_fields": []any{"score"},
	}

	got := findingsForRuleWithConfig(t, path, optionalFieldRuleID, cfg)
	// Includes the existing heuristic finding for "name" (5/6 presence,
	// not in required_fields so the heuristic still applies).
	var schemaFinding *diag.Finding
	for i := range got {
		if strings.Contains(got[i].Message, `"score"`) {
			schemaFinding = &got[i]
		}
	}
	if schemaFinding == nil {
		t.Fatalf("expected a schema finding for 'score'; got %s", joinMessages(got))
	}
	if !strings.Contains(schemaFinding.Message, "6/6") {
		t.Errorf("message should report 6/6 missing; got %q", schemaFinding.Message)
	}
	if schemaFinding.Location.Row != 1 {
		t.Errorf("row = %d, want 1 (never-present field anchors at row 1)", schemaFinding.Location.Row)
	}
}

func TestOptionalFieldRequired_SchemaSuppressesHeuristicForSameField(t *testing.T) {
	// "name" at 5/6 = 83.3% would trip the heuristic by default. With
	// "name" in required_fields, the schema path covers it and the
	// heuristic must skip it — otherwise we'd double-fire.
	path := testutil.Fixture(t, "optional-field-required-by-downstream/positive.jsonl")
	cfg := config.Default()
	cfg.Rules[optionalFieldRuleID] = map[string]any{
		"required_fields": []any{"name"},
	}

	got := findingsForRuleWithConfig(t, path, optionalFieldRuleID, cfg)
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 finding (schema only, no heuristic dup); got %d: %s",
			len(got), joinMessages(got))
	}
	if !strings.Contains(got[0].Message, `schema declares "name"`) {
		t.Errorf("expected schema-style message; got %q", got[0].Message)
	}
}

func TestOptionalFieldRequired_SchemaRequiredFieldFullyPresent(t *testing.T) {
	// "id" appears in 6/6 rows. With "id" in required_fields, schema
	// path is satisfied → no finding for "id".
	path := testutil.Fixture(t, "optional-field-required-by-downstream/positive.jsonl")
	cfg := config.Default()
	cfg.Rules[optionalFieldRuleID] = map[string]any{
		"required_fields": []any{"id"},
	}

	got := findingsForRuleWithConfig(t, path, optionalFieldRuleID, cfg)
	for _, f := range got {
		if strings.Contains(f.Message, `"id"`) {
			t.Errorf("'id' should not fire (100%% present); got %q", f.Message)
		}
	}
}

func TestOptionalFieldRequired_ConfigOverride(t *testing.T) {
	// With a tightened threshold of 0.9, "name" at 83.3% no longer
	// trips the rule. Verifies ctx.Settings.Float plumbs through.
	path := testutil.Fixture(t, "optional-field-required-by-downstream/positive.jsonl")
	cfg := config.Default()
	cfg.Rules[optionalFieldRuleID] = map[string]any{
		"min_presence_ratio": 0.9,
	}

	all, err := pipeline.Run([]string{path}, cfg)
	if err != nil {
		t.Fatalf("pipeline.Run: %v", err)
	}
	var got []diag.Finding
	for _, f := range all {
		if f.RuleID == optionalFieldRuleID {
			got = append(got, f)
		}
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 findings under raised threshold, got %d: %s",
			len(got), joinMessages(got))
	}
}
