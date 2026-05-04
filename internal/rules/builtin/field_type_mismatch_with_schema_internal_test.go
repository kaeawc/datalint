package builtin

import "testing"

// In-package tests for the path parser and walker. parsePath /
// evaluatePath are private helpers; testing them here lets the
// black-box external tests focus on the rule's user-facing
// behaviour.

func TestParsePath(t *testing.T) {
	cases := []struct {
		in  string
		out []string // "name" for a literal field or "[]" for the array marker
	}{
		{"input", []string{"input"}},
		{"meta.author", []string{"meta", "author"}},
		{"messages[]", []string{"messages", "[]"}},
		{"messages[].role", []string{"messages", "[]", "role"}},
		{"a.b[].c[].d", []string{"a", "b", "[]", "c", "[]", "d"}},
		{"", nil},
	}
	for _, c := range cases {
		segs := parsePath(c.in)
		if len(segs) != len(c.out) {
			t.Errorf("parsePath(%q): len = %d, want %d (%+v)", c.in, len(segs), len(c.out), segs)
			continue
		}
		for i, s := range segs {
			want := c.out[i]
			if want == "[]" {
				if !s.isArray {
					t.Errorf("parsePath(%q)[%d]: expected isArray=true; got %+v", c.in, i, s)
				}
			} else if s.name != want {
				t.Errorf("parsePath(%q)[%d]: name = %q, want %q", c.in, i, s.name, want)
			}
		}
	}
}

func TestEvaluatePath_TopLevelField(t *testing.T) {
	obj := map[string]any{"input": "hello"}
	got := evaluatePath(any(obj), parsePath("input"))
	if len(got) != 1 || got[0] != "hello" {
		t.Errorf("got %+v, want [hello]", got)
	}
}

func TestEvaluatePath_NestedObject(t *testing.T) {
	obj := map[string]any{"meta": map[string]any{"author": "alice"}}
	got := evaluatePath(any(obj), parsePath("meta.author"))
	if len(got) != 1 || got[0] != "alice" {
		t.Errorf("got %+v, want [alice]", got)
	}
}

func TestEvaluatePath_ArrayFanout(t *testing.T) {
	obj := map[string]any{
		"messages": []any{
			map[string]any{"role": "user"},
			map[string]any{"role": "assistant"},
			map[string]any{"role": "user"},
		},
	}
	got := evaluatePath(any(obj), parsePath("messages[].role"))
	if len(got) != 3 {
		t.Fatalf("got %d resolved values, want 3 (%+v)", len(got), got)
	}
	for i, want := range []string{"user", "assistant", "user"} {
		if got[i] != want {
			t.Errorf("[%d] = %v, want %s", i, got[i], want)
		}
	}
}

func TestEvaluatePath_PathDoesntApplyReturnsNil(t *testing.T) {
	// messages is a string here, not an array — the path
	// messages[].role doesn't apply and returns no resolved values.
	obj := map[string]any{"messages": "not an array"}
	got := evaluatePath(any(obj), parsePath("messages[].role"))
	if got != nil {
		t.Errorf("expected nil; got %+v", got)
	}
}

func TestEvaluatePath_MissingKeyReturnsNil(t *testing.T) {
	obj := map[string]any{"other": 1}
	got := evaluatePath(any(obj), parsePath("missing.path"))
	if got != nil {
		t.Errorf("expected nil; got %+v", got)
	}
}

func TestEvaluatePath_ArrayOfMixedShapes(t *testing.T) {
	// Only elements with the right shape contribute; others are
	// silently dropped.
	obj := map[string]any{
		"items": []any{
			map[string]any{"v": 1},
			"not an object",
			map[string]any{"v": 2},
		},
	}
	got := evaluatePath(any(obj), parsePath("items[].v"))
	if len(got) != 2 {
		t.Fatalf("got %d, want 2 (%+v)", len(got), got)
	}
}
