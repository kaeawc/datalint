package output

import (
	"encoding/json"
	"io"

	"github.com/kaeawc/datalint/internal/diag"
)

// SARIF schema version this writer targets.
const (
	sarifVersion        = "2.1.0"
	sarifSchemaURL      = "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json"
	sarifInformationURI = "https://github.com/kaeawc/datalint"
)

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string `json:"name"`
	Version        string `json:"version,omitempty"`
	InformationURI string `json:"informationUri,omitempty"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations,omitempty"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           *sarifRegion          `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

// WriteSARIF writes findings to w as a SARIF 2.1.0 log.
func WriteSARIF(w io.Writer, findings []diag.Finding, version string) error {
	log := sarifLog{
		Schema:  sarifSchemaURL,
		Version: sarifVersion,
		Runs: []sarifRun{
			{
				Tool: sarifTool{
					Driver: sarifDriver{
						Name:           "datalint",
						Version:        version,
						InformationURI: sarifInformationURI,
					},
				},
				Results: toSARIFResults(findings),
			},
		},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(log)
}

func toSARIFResults(findings []diag.Finding) []sarifResult {
	if len(findings) == 0 {
		return []sarifResult{}
	}
	out := make([]sarifResult, 0, len(findings))
	for _, f := range findings {
		out = append(out, sarifResult{
			RuleID:    f.RuleID,
			Level:     sarifLevel(f.Severity),
			Message:   sarifMessage{Text: f.Message},
			Locations: sarifLocationsFor(f.Location),
		})
	}
	return out
}

// sarifLevel maps Severity to SARIF's level vocabulary.
func sarifLevel(s diag.Severity) string {
	switch s {
	case diag.SeverityError:
		return "error"
	case diag.SeverityWarning:
		return "warning"
	case diag.SeverityInfo:
		return "note"
	}
	return "none"
}

// sarifLocationsFor builds the locations array. SARIF has no row
// concept distinct from line, so a Finding with Row but no Line gets
// Row mapped onto startLine — that's how data-file findings remain
// usable to GitHub's code-scanning UI.
func sarifLocationsFor(loc diag.Location) []sarifLocation {
	if loc.Path == "" {
		return nil
	}
	out := sarifLocation{
		PhysicalLocation: sarifPhysicalLocation{
			ArtifactLocation: sarifArtifactLocation{URI: loc.Path},
		},
	}
	line := loc.Line
	if line == 0 && loc.Row != 0 {
		line = loc.Row
	}
	if line > 0 {
		out.PhysicalLocation.Region = &sarifRegion{StartLine: line}
	}
	return []sarifLocation{out}
}
