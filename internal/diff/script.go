package diff

import (
	"sort"
	"unicode"
)

// MinRunesForScriptMix is the per-side letter-rune threshold under
// which a field's script-mix section is suppressed. Fields with too
// few letters yield noisy ratios — a single non-Latin character can
// look like a 50% script shift when the field is one word long.
const MinRunesForScriptMix = 50

// scriptTable lists the Unicode scripts the diff tracks by name. The
// order is the priority for IsScript checks: only the first match
// counts. Letters that don't match any of these fall into the "Other"
// bucket. Common (digits, punctuation, whitespace) is not a letter
// and isn't counted at all.
var scriptTable = []struct {
	name  string
	table *unicode.RangeTable
}{
	{"Latin", unicode.Latin},
	{"Han", unicode.Han},
	{"Hiragana", unicode.Hiragana},
	{"Katakana", unicode.Katakana},
	{"Hangul", unicode.Hangul},
	{"Cyrillic", unicode.Cyrillic},
	{"Arabic", unicode.Arabic},
	{"Hebrew", unicode.Hebrew},
	{"Devanagari", unicode.Devanagari},
	{"Greek", unicode.Greek},
	{"Thai", unicode.Thai},
}

// classifyRune returns the script name for r, or "" if r is not a
// letter. "Other" covers letters that don't match any tracked script
// (e.g., Armenian, Tibetan, Ethiopic) — surfaced so the totals add
// up to the letter count.
func classifyRune(r rune) string {
	if !unicode.IsLetter(r) {
		return ""
	}
	for _, s := range scriptTable {
		if unicode.Is(s.table, r) {
			return s.name
		}
	}
	return "Other"
}

// countScripts walks s and returns a script→letter-count map. Empty
// strings or strings without any letters yield an empty (non-nil)
// map.
func countScripts(s string) map[string]int {
	out := map[string]int{}
	for _, r := range s {
		name := classifyRune(r)
		if name == "" {
			continue
		}
		out[name]++
	}
	return out
}

// scriptMix turns a script→count map into the sorted-by-count
// ScriptCount slice used in the FieldDistribution. Counts under
// MinRunesForScriptMix in BOTH oldCounts and newCounts are not the
// concern of this function — buildScriptMix handles the suppression.
// Ratios are computed against the per-side total (sum of all
// tracked-script counts on that side) so they sum to 1.0 within
// floating-point error.
func scriptMix(counts map[string]int) []ScriptCount {
	if len(counts) == 0 {
		return nil
	}
	total := 0
	for _, c := range counts {
		total += c
	}
	out := make([]ScriptCount, 0, len(counts))
	for name, c := range counts {
		out = append(out, ScriptCount{
			Script: name,
			Count:  c,
			Ratio:  float64(c) / float64(total),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Script < out[j].Script
	})
	return out
}

// buildScriptMix returns the (old, new) script-mix pair for one
// field. If neither side has at least MinRunesForScriptMix tracked
// letters, the field is suppressed (both slices nil) — this is the
// "too noisy to report" case. Otherwise both sides are reported,
// even if one is empty (an enum field that became a free-text field
// or vice versa is exactly the kind of shift worth surfacing).
func buildScriptMix(oldCounts, newCounts map[string]int) (oldOut, newOut []ScriptCount) {
	if sumCounts(oldCounts) < MinRunesForScriptMix && sumCounts(newCounts) < MinRunesForScriptMix {
		return nil, nil
	}
	return scriptMix(oldCounts), scriptMix(newCounts)
}

func sumCounts(m map[string]int) int {
	total := 0
	for _, c := range m {
		total += c
	}
	return total
}
