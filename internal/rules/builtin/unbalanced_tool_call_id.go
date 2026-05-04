package builtin

import (
	"encoding/json"
	"fmt"

	"github.com/kaeawc/datalint/internal/diag"
	"github.com/kaeawc/datalint/internal/rules"
	"github.com/kaeawc/datalint/internal/scanner"
)

func init() {
	rules.Register(&rules.Rule{
		ID:         "unbalanced-tool-call-id",
		Category:   rules.CategoryConversation,
		Severity:   diag.SeverityError,
		Confidence: rules.ConfidenceHigh,
		Fix:        rules.FixNone,
		Needs:      rules.NeedsJSONL,
		Check:      checkUnbalancedToolCallID,
	})
}

// toolCallMsg is the minimal OpenAI-style chat message we need: role,
// optional tool_calls (declared on assistant messages), and optional
// tool_call_id (referenced from tool messages).
type toolCallMsg struct {
	Role       string         `json:"role"`
	ToolCallID string         `json:"tool_call_id"`
	ToolCalls  []toolCallDecl `json:"tool_calls"`
}

type toolCallDecl struct {
	ID string `json:"id"`
}

type toolCallRow struct {
	Messages []toolCallMsg `json:"messages"`
}

// checkUnbalancedToolCallID walks each row's messages forward, building
// the set of tool_call IDs declared on assistant messages, and flags
// the first tool message whose tool_call_id isn't yet declared. That
// catches both "no matching declaration anywhere" and "declared later
// (forward reference)" — both indicate the trace is malformed.
//
// Tool messages without a tool_call_id are tolerated (some schema
// variants omit the field), as are rows without a messages array.
// One finding per row, anchored at the first offender.
func checkUnbalancedToolCallID(ctx *rules.Context, emit func(diag.Finding)) {
	if ctx == nil || ctx.File == nil || ctx.File.Kind != scanner.KindJSONL {
		return
	}
	path := ctx.File.Path
	_ = scanner.StreamJSONL(path, func(row int, line []byte) error {
		checkUnbalancedToolCallIDRow(path, row, line, emit)
		return nil
	})
}

func checkUnbalancedToolCallIDRow(path string, row int, line []byte, emit func(diag.Finding)) {
	if len(line) == 0 {
		return
	}
	var r toolCallRow
	if json.Unmarshal(line, &r) != nil {
		return
	}
	idx, id, ok := firstUnbalancedToolID(r.Messages)
	if !ok {
		return
	}
	emit(diag.Finding{
		RuleID:   "unbalanced-tool-call-id",
		Severity: diag.SeverityError,
		Message: fmt.Sprintf(
			"tool message at index %d references tool_call_id %q with no preceding assistant tool_calls entry",
			idx, id),
		Location: diag.Location{Path: path, Row: row},
	})
}

func firstUnbalancedToolID(msgs []toolCallMsg) (int, string, bool) {
	declared := map[string]bool{}
	for i, m := range msgs {
		if m.Role == "assistant" {
			for _, tc := range m.ToolCalls {
				if tc.ID != "" {
					declared[tc.ID] = true
				}
			}
			continue
		}
		if m.Role == "tool" && m.ToolCallID != "" && !declared[m.ToolCallID] {
			return i, m.ToolCallID, true
		}
	}
	return 0, "", false
}
