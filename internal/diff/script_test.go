package diff

import "testing"

func TestClassifyRune_RecognisesScripts(t *testing.T) {
	cases := []struct {
		r    rune
		want string
	}{
		{'a', "Latin"},
		{'Z', "Latin"},
		{'д', "Cyrillic"},
		{'你', "Han"},
		{'あ', "Hiragana"},
		{'カ', "Katakana"},
		{'한', "Hangul"},
		{'ع', "Arabic"},
		{'ש', "Hebrew"},
		{'न', "Devanagari"},
		{'Ω', "Greek"},
		{'ก', "Thai"},
		{'֍', ""}, // Armenian apostrophe — punctuation, not a letter
		{'1', ""}, // digit, not a letter
		{' ', ""}, // whitespace
		{'!', ""}, // punctuation
	}
	for _, c := range cases {
		got := classifyRune(c.r)
		if got != c.want {
			t.Errorf("classifyRune(%q) = %q, want %q", c.r, got, c.want)
		}
	}
}

func TestClassifyRune_OtherForUntrackedLetters(t *testing.T) {
	// Armenian letters are letters but not in our tracked-script list.
	if got := classifyRune('Ա'); got != "Other" {
		t.Errorf("classifyRune('Ա') = %q, want \"Other\"", got)
	}
}

func TestCountScripts_PunctuationAndDigitsExcluded(t *testing.T) {
	got := countScripts("Hello, world! 123")
	if got["Latin"] != 10 { // "Helloworld"
		t.Errorf("Latin count = %d, want 10", got["Latin"])
	}
	if _, ok := got["Common"]; ok {
		t.Errorf("punctuation/digits should not appear: got %+v", got)
	}
}

func TestScriptMix_SortedByCountDescending(t *testing.T) {
	got := scriptMix(map[string]int{"Latin": 80, "Cyrillic": 20, "Han": 100})
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d: %+v", len(got), got)
	}
	if got[0].Script != "Han" || got[1].Script != "Latin" || got[2].Script != "Cyrillic" {
		t.Errorf("wrong order: %+v", got)
	}
	if got[0].Ratio != 0.5 {
		t.Errorf("Han ratio = %f, want 0.5", got[0].Ratio)
	}
}

func TestScriptMix_TieBrokenAlphabetically(t *testing.T) {
	got := scriptMix(map[string]int{"Latin": 50, "Cyrillic": 50})
	if got[0].Script != "Cyrillic" || got[1].Script != "Latin" {
		t.Errorf("tie should sort alphabetically; got %+v", got)
	}
}

func TestBuildScriptMix_SuppressedWhenBothSidesShort(t *testing.T) {
	old, new := buildScriptMix(
		map[string]int{"Latin": 5},
		map[string]int{"Latin": 10},
	)
	if old != nil || new != nil {
		t.Errorf("expected nil/nil for short sides; got old=%+v new=%+v", old, new)
	}
}

func TestBuildScriptMix_ReportedWhenOneSideMeetsThreshold(t *testing.T) {
	// Old side is short (5 letters), new side meets MinRunesForScriptMix.
	// Both should still be reported — a free-text field that grew from
	// short stubs to long bodies is exactly the kind of shift to surface.
	old, new := buildScriptMix(
		map[string]int{"Latin": 5},
		map[string]int{"Latin": MinRunesForScriptMix},
	)
	if old == nil {
		t.Errorf("old side should be reported (other side meets threshold)")
	}
	if new == nil {
		t.Errorf("new side should be reported")
	}
}
