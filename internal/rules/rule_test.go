package rules_test

import (
	"testing"

	"github.com/kaeawc/datalint/internal/rules"
)

func TestConfidence_String(t *testing.T) {
	cases := []struct {
		in   rules.Confidence
		want string
	}{
		{rules.ConfidenceLow, "low"},
		{rules.ConfidenceMedium, "medium"},
		{rules.ConfidenceHigh, "high"},
		{rules.Confidence(99), "low"}, // unknown defaults to low
	}
	for _, c := range cases {
		if got := c.in.String(); got != c.want {
			t.Errorf("Confidence(%d).String() = %q, want %q", int(c.in), got, c.want)
		}
	}
}

func TestFixLevel_String(t *testing.T) {
	cases := []struct {
		in   rules.FixLevel
		want string
	}{
		{rules.FixNone, "none"},
		{rules.FixCosmetic, "cosmetic"},
		{rules.FixIdiomatic, "idiomatic"},
		{rules.FixSemantic, "semantic"},
		{rules.FixLevel(99), "none"}, // unknown defaults to none
	}
	for _, c := range cases {
		if got := c.in.String(); got != c.want {
			t.Errorf("FixLevel(%d).String() = %q, want %q", int(c.in), got, c.want)
		}
	}
}
