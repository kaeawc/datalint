package diag_test

import (
	"testing"

	"github.com/kaeawc/datalint/internal/diag"
)

func TestSeverity_String(t *testing.T) {
	cases := []struct {
		in   diag.Severity
		want string
	}{
		{diag.SeverityInfo, "info"},
		{diag.SeverityWarning, "warning"},
		{diag.SeverityError, "error"},
		{diag.Severity(99), "info"}, // unknown defaults to info
	}
	for _, c := range cases {
		if got := c.in.String(); got != c.want {
			t.Errorf("Severity(%d).String() = %q, want %q", int(c.in), got, c.want)
		}
	}
}
