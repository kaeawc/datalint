package lsp

import "testing"

// applyContentChange and positionToByte are package-private; the
// tests live in package lsp (not lsp_test) so they can exercise the
// helpers directly without going through a full RPC round-trip.

func TestApplyContentChange_FullReplacement(t *testing.T) {
	got := applyContentChange([]byte("old\n"), contentChange{Text: "new\n"})
	if string(got) != "new\n" {
		t.Errorf("got %q, want %q", got, "new\n")
	}
}

func TestApplyContentChange_SingleLineSplice(t *testing.T) {
	// "hello world" → replace "world" (line 0, chars 6-11) with "there"
	in := []byte("hello world\n")
	ch := contentChange{
		Range: &lspRange{
			Start: lspPosition{Line: 0, Character: 6},
			End:   lspPosition{Line: 0, Character: 11},
		},
		Text: "there",
	}
	got := applyContentChange(in, ch)
	if string(got) != "hello there\n" {
		t.Errorf("got %q, want %q", got, "hello there\n")
	}
}

func TestApplyContentChange_PureInsertion(t *testing.T) {
	// "ac" → insert "b" at line 0, char 1 (Start == End)
	in := []byte("ac")
	ch := contentChange{
		Range: &lspRange{
			Start: lspPosition{Line: 0, Character: 1},
			End:   lspPosition{Line: 0, Character: 1},
		},
		Text: "b",
	}
	got := applyContentChange(in, ch)
	if string(got) != "abc" {
		t.Errorf("got %q, want %q", got, "abc")
	}
}

func TestApplyContentChange_PureDeletion(t *testing.T) {
	// "abc" → delete "b" (range [1, 2), text="")
	in := []byte("abc")
	ch := contentChange{
		Range: &lspRange{
			Start: lspPosition{Line: 0, Character: 1},
			End:   lspPosition{Line: 0, Character: 2},
		},
		Text: "",
	}
	got := applyContentChange(in, ch)
	if string(got) != "ac" {
		t.Errorf("got %q, want %q", got, "ac")
	}
}

func TestApplyContentChange_MultiLineRange(t *testing.T) {
	// Replace lines 1-2 entirely (range from line 1 char 0 to line 3 char 0)
	// with a single line of new content.
	in := []byte("line0\nline1\nline2\nline3\n")
	ch := contentChange{
		Range: &lspRange{
			Start: lspPosition{Line: 1, Character: 0},
			End:   lspPosition{Line: 3, Character: 0},
		},
		Text: "replacement\n",
	}
	got := applyContentChange(in, ch)
	want := "line0\nreplacement\nline3\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApplyContentChange_SurrogatePairCounted_AsTwoUnits(t *testing.T) {
	// "🙂abc" — the smiley is U+1F642, which is a 4-byte UTF-8
	// sequence and a UTF-16 surrogate pair (2 code units). LSP
	// position 2 on line 0 should resolve to the byte after the
	// smiley, i.e. just before 'a'.
	in := []byte("🙂abc")
	ch := contentChange{
		Range: &lspRange{
			Start: lspPosition{Line: 0, Character: 2},
			End:   lspPosition{Line: 0, Character: 3},
		},
		Text: "X",
	}
	got := applyContentChange(in, ch)
	if string(got) != "🙂Xbc" {
		t.Errorf("surrogate-pair handling wrong: got %q, want %q", got, "🙂Xbc")
	}
}

func TestApplyContentChange_PositionPastEOFClamps(t *testing.T) {
	// Range whose end is past EOF should clamp to len(buf).
	in := []byte("abc")
	ch := contentChange{
		Range: &lspRange{
			Start: lspPosition{Line: 0, Character: 1},
			End:   lspPosition{Line: 5, Character: 0},
		},
		Text: "Z",
	}
	got := applyContentChange(in, ch)
	if string(got) != "aZ" {
		t.Errorf("got %q, want %q", got, "aZ")
	}
}

func TestPositionToByte_LineStarts(t *testing.T) {
	in := []byte("aa\nbb\ncc\n")
	cases := []struct {
		line, char int
		want       int
	}{
		{0, 0, 0},
		{1, 0, 3}, // after first '\n'
		{2, 0, 6}, // after second '\n'
		{2, 2, 8}, // 'c'+'c' on line 2
	}
	for _, c := range cases {
		got := positionToByte(in, lspPosition{Line: c.line, Character: c.char})
		if got != c.want {
			t.Errorf("positionToByte(%d, %d) = %d, want %d", c.line, c.char, got, c.want)
		}
	}
}

func TestPositionToByte_CharacterPastLineEndClampsToNewline(t *testing.T) {
	in := []byte("ab\ncd\n")
	// Line 0 has chars [0..2); requesting char 99 should stop at '\n' (offset 2).
	got := positionToByte(in, lspPosition{Line: 0, Character: 99})
	if got != 2 {
		t.Errorf("got %d, want 2", got)
	}
}
