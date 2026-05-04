package builtin_test

import (
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/config"
	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/pipeline"
	"github.com/kaeawc/datalint/internal/testutil"
)

const fieldTypeSchemaRuleID = "field-type-mismatch-with-schema"

// fieldTypeSchemaCfg installs the canonical schema used by the
// positive fixture: input is a string, score is a number, tags is
// an array. Mismatches against this schema are what the tests
// expect to fire.
func fieldTypeSchemaCfg() config.Config {
	cfg := config.Default()
	cfg.Rules[fieldTypeSchemaRuleID] = map[string]any{
		"field_types": map[string]any{
			"input": "string",
			"score": "number",
			"tags":  "array",
		},
	}
	return cfg
}

func findingsForRuleAndCfg(t *testing.T, path, id string, cfg config.Config) []diag.Finding {
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

func TestFieldTypeMismatchWithSchema_NoConfigNoFindings(t *testing.T) {
	// Without field_types declared, the rule stays silent — even on a
	// fixture that would trip every kind of mismatch.
	path := testutil.Fixture(t, "field-type-mismatch-with-schema/positive.jsonl")
	got := findingsForRuleAndCfg(t, path, fieldTypeSchemaRuleID, config.Default())
	if len(got) != 0 {
		t.Fatalf("expected 0 findings without config; got %d: %s",
			len(got), joinMessages(got))
	}
}

func TestFieldTypeMismatchWithSchema_DetectsAllThreeMismatches(t *testing.T) {
	// positive.jsonl mismatches against the canonical schema:
	//   row 3: score "high"  → score declared number, observed string
	//   row 4: tags  "math"  → tags declared array, observed string
	//   row 5: input 42      → input declared string, observed number
	//   row 6: score null    → score declared number, observed null
	path := testutil.Fixture(t, "field-type-mismatch-with-schema/positive.jsonl")
	got := findingsForRuleAndCfg(t, path, fieldTypeSchemaRuleID, fieldTypeSchemaCfg())
	if len(got) != 4 {
		t.Fatalf("expected 4 findings; got %d: %s", len(got), joinMessages(got))
	}

	want := map[string]struct {
		row      int
		observed string
	}{
		`"input"`: {row: 5, observed: "number"},
		`"score"`: {row: 3, observed: "string"}, // sorted alphabetically; 'null' comes after 'string' in observedTypes order
		`"tags"`:  {row: 4, observed: "string"},
	}
	seen := map[string]int{}
	for _, f := range got {
		for field := range want {
			if strings.Contains(f.Message, field) && strings.Contains(f.Message, want[field].observed) {
				seen[field]++
				if f.Location.Row != want[field].row {
					// score has two mismatches (string + null); only the
					// string one should hit row 3. The null one hits row 6.
					nullFinding := field == `"score"` && f.Location.Row == 6 && strings.Contains(f.Message, "null")
					if !nullFinding {
						t.Errorf("for field %s observed %s: row = %d, want %d (%q)",
							field, want[field].observed, f.Location.Row, want[field].row, f.Message)
					}
				}
			}
		}
	}
	if seen[`"input"`] != 1 || seen[`"tags"`] != 1 {
		t.Errorf("expected one finding each for input/tags; saw %+v", seen)
	}
}

func TestFieldTypeMismatchWithSchema_NullCountsAsMismatch(t *testing.T) {
	// Score is declared number; row 6 has "score": null. Verifies null
	// is treated as its own type rather than skipped (jsonTypeOf would
	// have returned "" — schemaTypeOf returns "null").
	path := testutil.Fixture(t, "field-type-mismatch-with-schema/positive.jsonl")
	got := findingsForRuleAndCfg(t, path, fieldTypeSchemaRuleID, fieldTypeSchemaCfg())
	var nullFinding *diag.Finding
	for i := range got {
		if strings.Contains(got[i].Message, "null") && strings.Contains(got[i].Message, `"score"`) {
			nullFinding = &got[i]
			break
		}
	}
	if nullFinding == nil {
		t.Fatalf("expected a null mismatch finding for score; got %s", joinMessages(got))
	}
	if nullFinding.Location.Row != 6 {
		t.Errorf("null finding row = %d, want 6", nullFinding.Location.Row)
	}
}

func TestFieldTypeMismatchWithSchema_SeparateFindingsPerObservedType(t *testing.T) {
	// score has both a "string" mismatch (row 3) and a "null" mismatch
	// (row 6). They must surface as TWO findings, each citing its own
	// row and count, rather than collapsing into one.
	path := testutil.Fixture(t, "field-type-mismatch-with-schema/positive.jsonl")
	got := findingsForRuleAndCfg(t, path, fieldTypeSchemaRuleID, fieldTypeSchemaCfg())
	scoreCount := 0
	for _, f := range got {
		if strings.Contains(f.Message, `"score"`) {
			scoreCount++
		}
	}
	if scoreCount != 2 {
		t.Errorf("expected 2 score findings (string + null); got %d in %s",
			scoreCount, joinMessages(got))
	}
}

func TestFieldTypeMismatchWithSchema_MissingFieldIgnored(t *testing.T) {
	// optional-field-required-by-downstream owns missing-field detection.
	// This rule must not fire when a declared field is simply absent.
	path := testutil.Fixture(t, "field-type-mismatch-with-schema/negative-clean.jsonl")
	cfg := config.Default()
	cfg.Rules[fieldTypeSchemaRuleID] = map[string]any{
		"field_types": map[string]any{
			"input":           "string",
			"score":           "number",
			"never_in_corpus": "string", // declared but never present
		},
	}
	got := findingsForRuleAndCfg(t, path, fieldTypeSchemaRuleID, cfg)
	if len(got) != 0 {
		t.Fatalf("expected 0 findings (missing field is the optional-field rule's domain); got %d: %s",
			len(got), joinMessages(got))
	}
}

func TestFieldTypeMismatchWithSchema_CleanFixtureNoFindings(t *testing.T) {
	path := testutil.Fixture(t, "field-type-mismatch-with-schema/negative-clean.jsonl")
	got := findingsForRuleAndCfg(t, path, fieldTypeSchemaRuleID, fieldTypeSchemaCfg())
	if len(got) != 0 {
		t.Fatalf("expected 0 findings on clean fixture; got %d: %s",
			len(got), joinMessages(got))
	}
}

func TestFieldTypeMismatchWithSchema_InvalidDeclaredTypeIgnored(t *testing.T) {
	// "integer" is not one of the supported tags; the rule silently
	// ignores it rather than failing. The valid "score: number"
	// declaration still applies.
	path := testutil.Fixture(t, "field-type-mismatch-with-schema/positive.jsonl")
	cfg := config.Default()
	cfg.Rules[fieldTypeSchemaRuleID] = map[string]any{
		"field_types": map[string]any{
			"input": "integer", // typo / unsupported tag
			"score": "number",
		},
	}
	got := findingsForRuleAndCfg(t, path, fieldTypeSchemaRuleID, cfg)
	for _, f := range got {
		if strings.Contains(f.Message, `"input"`) {
			t.Errorf("input had an unsupported declared type and should be ignored; got %q", f.Message)
		}
	}
	// score should still produce findings (string + null)
	scoreFindings := 0
	for _, f := range got {
		if strings.Contains(f.Message, `"score"`) {
			scoreFindings++
		}
	}
	if scoreFindings != 2 {
		t.Errorf("expected 2 score findings; got %d", scoreFindings)
	}
}
