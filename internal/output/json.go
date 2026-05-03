// Package output renders findings to JSON, SARIF, and HTML. Only the
// JSON formatter is implemented in the skeleton.
package output

import (
	"encoding/json"
	"io"

	"github.com/kaeawc/datalint/internal/diag"
)

// WriteJSON writes findings to w as a pretty-printed JSON array.
func WriteJSON(w io.Writer, findings []diag.Finding) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if findings == nil {
		findings = []diag.Finding{}
	}
	return enc.Encode(findings)
}
