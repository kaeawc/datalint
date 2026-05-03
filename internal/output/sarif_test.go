package output_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/output"
)

// decoded is a structurally-typed view of the SARIF log we emit.
// Keeping it separate from output.sarifLog (unexported) keeps the
// public contract observable: tests only see fields they parse.
type decoded struct {
	Schema  string `json:"$schema"`
	Version string `json:"version"`
	Runs    []struct {
		Tool struct {
			Driver struct {
				Name           string `json:"name"`
				Version        string `json:"version"`
				InformationURI string `json:"informationUri"`
			} `json:"driver"`
		} `json:"tool"`
		Results []struct {
			RuleID  string `json:"ruleId"`
			Level   string `json:"level"`
			Message struct {
				Text string `json:"text"`
			} `json:"message"`
			Locations []struct {
				PhysicalLocation struct {
					ArtifactLocation struct {
						URI string `json:"uri"`
					} `json:"artifactLocation"`
					Region *struct {
						StartLine int `json:"startLine"`
					} `json:"region"`
				} `json:"physicalLocation"`
			} `json:"locations"`
		} `json:"results"`
	} `json:"runs"`
}

func runWriter(t *testing.T, findings []diag.Finding, version string) decoded {
	t.Helper()
	var buf bytes.Buffer
	if err := output.WriteSARIF(&buf, findings, version); err != nil {
		t.Fatalf("WriteSARIF: %v", err)
	}
	var got decoded
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, buf.String())
	}
	return got
}

func TestWriteSARIF_Empty(t *testing.T) {
	got := runWriter(t, nil, "1.2.3")

	if got.Version != "2.1.0" {
		t.Errorf("version = %q, want 2.1.0", got.Version)
	}
	if !strings.Contains(got.Schema, "sarif-schema-2.1.0.json") {
		t.Errorf("schema URL unexpected: %q", got.Schema)
	}
	if len(got.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(got.Runs))
	}
	if got.Runs[0].Tool.Driver.Name != "datalint" {
		t.Errorf("driver name = %q, want datalint", got.Runs[0].Tool.Driver.Name)
	}
	if got.Runs[0].Tool.Driver.Version != "1.2.3" {
		t.Errorf("driver version = %q, want 1.2.3", got.Runs[0].Tool.Driver.Version)
	}
	if len(got.Runs[0].Results) != 0 {
		t.Errorf("results = %d, want 0", len(got.Runs[0].Results))
	}
}

func TestWriteSARIF_RowMapsToStartLine(t *testing.T) {
	findings := []diag.Finding{
		{
			RuleID:   "jsonl-malformed-line",
			Severity: diag.SeverityError,
			Message:  "invalid JSON",
			Location: diag.Location{Path: "data.jsonl", Row: 7},
		},
	}
	got := runWriter(t, findings, "dev")

	if len(got.Runs[0].Results) != 1 {
		t.Fatalf("results = %d, want 1", len(got.Runs[0].Results))
	}
	r := got.Runs[0].Results[0]
	if r.RuleID != "jsonl-malformed-line" {
		t.Errorf("ruleId = %q", r.RuleID)
	}
	if r.Level != "error" {
		t.Errorf("level = %q, want error", r.Level)
	}
	if r.Message.Text != "invalid JSON" {
		t.Errorf("message = %q", r.Message.Text)
	}
	if len(r.Locations) != 1 {
		t.Fatalf("locations = %d, want 1", len(r.Locations))
	}
	if r.Locations[0].PhysicalLocation.ArtifactLocation.URI != "data.jsonl" {
		t.Errorf("uri = %q", r.Locations[0].PhysicalLocation.ArtifactLocation.URI)
	}
	region := r.Locations[0].PhysicalLocation.Region
	if region == nil {
		t.Fatal("region is nil; expected startLine from Row")
	}
	if region.StartLine != 7 {
		t.Errorf("startLine = %d, want 7", region.StartLine)
	}
}

func TestWriteSARIF_LineWinsOverRow(t *testing.T) {
	findings := []diag.Finding{
		{
			RuleID:   "x",
			Severity: diag.SeverityWarning,
			Message:  "m",
			Location: diag.Location{Path: "f.py", Line: 12, Row: 99},
		},
	}
	got := runWriter(t, findings, "dev")
	r := got.Runs[0].Results[0]
	if len(r.Locations) != 1 {
		t.Fatalf("locations = %d, want 1", len(r.Locations))
	}
	region := r.Locations[0].PhysicalLocation.Region
	if region == nil || region.StartLine != 12 {
		t.Fatalf("expected startLine=12 from Line, got %+v", region)
	}
}

func TestWriteSARIF_NoLocationWhenPathEmpty(t *testing.T) {
	findings := []diag.Finding{
		{
			RuleID:   "x",
			Severity: diag.SeverityInfo,
			Message:  "m",
			Location: diag.Location{},
		},
	}
	got := runWriter(t, findings, "dev")
	r := got.Runs[0].Results[0]
	if r.Level != "note" {
		t.Errorf("level = %q, want note", r.Level)
	}
	if len(r.Locations) != 0 {
		t.Errorf("locations = %d, want 0 when Path is empty", len(r.Locations))
	}
}
