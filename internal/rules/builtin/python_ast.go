package builtin

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// walkPython visits every descendant node depth-first.
func walkPython(n *sitter.Node, fn func(*sitter.Node)) {
	if n == nil {
		return
	}
	fn(n)
	for i := uint32(0); i < n.ChildCount(); i++ {
		walkPython(n.Child(int(i)), fn)
	}
}

// dottedPath returns the dotted-attribute path of an expression like
// "np.random.shuffle". Returns "" for anything that isn't a simple
// chain of identifiers — indexed/computed/parenthesized expressions
// are intentionally rejected so callers stay high-precision.
func dottedPath(n *sitter.Node, src []byte) string {
	if n == nil {
		return ""
	}
	var parts []string
	cur := n
	for cur != nil {
		switch cur.Type() {
		case "attribute":
			attr := cur.ChildByFieldName("attribute")
			if attr == nil {
				return ""
			}
			parts = append([]string{attr.Content(src)}, parts...)
			cur = cur.ChildByFieldName("object")
		case "identifier":
			parts = append([]string{cur.Content(src)}, parts...)
			cur = nil
		default:
			return ""
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ".")
}

// isNestedInCall reports whether n has a `call` ancestor — i.e. the
// node is an argument of some other call. Used to filter out cases
// like `train_test_split(random.shuffle(x))` where the inner call's
// source position is past the outer call's but it actually executes
// first.
func isNestedInCall(n *sitter.Node) bool {
	if n == nil {
		return false
	}
	parent := n.Parent()
	for parent != nil {
		if parent.Type() == "call" {
			return true
		}
		parent = parent.Parent()
	}
	return false
}
