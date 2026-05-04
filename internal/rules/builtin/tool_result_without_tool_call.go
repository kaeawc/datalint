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
		ID:         "tool-result-without-tool-call",
		Category:   rules.CategoryConversation,
		Severity:   diag.SeverityError,
		Confidence: rules.ConfidenceHigh,
		Fix:        rules.FixNone,
		Needs:      rules.NeedsJSONL,
		Check:      checkToolResultWithoutToolCall,
	})
}

// checkToolResultWithoutToolCall flags rows where a `tool` role
// message appears with no prior assistant message that has any
// tool_calls — a structural check that catches cases where
// unbalanced-tool-call-id can't help (variant schemas without
// tool_call_id, or "the assistant never called any tool").
//
// Reuses toolCallMsg / toolCallRow from unbalanced_tool_call_id.go.
// One finding per row, anchored at the first offending tool message.
func checkToolResultWithoutToolCall(ctx *rules.Context, emit func(diag.Finding)) {
	if ctx == nil || ctx.File == nil || ctx.File.Kind != scanner.KindJSONL {
		return
	}
	path := ctx.File.Path
	_ = scanner.StreamJSONL(path, func(row int, line []byte) error {
		checkToolResultWithoutToolCallRow(path, row, line, emit)
		return nil
	})
}

func checkToolResultWithoutToolCallRow(path string, row int, line []byte, emit func(diag.Finding)) {
	if len(line) == 0 {
		return
	}
	var r toolCallRow
	if json.Unmarshal(line, &r) != nil {
		return
	}
	idx, ok := firstToolWithoutPriorToolUse(r.Messages)
	if !ok {
		return
	}
	emit(diag.Finding{
		RuleID:   "tool-result-without-tool-call",
		Severity: diag.SeverityError,
		Message: fmt.Sprintf(
			"tool message at index %d has no preceding assistant message with tool_calls",
			idx),
		Location: diag.Location{Path: path, Row: row},
	})
}

func firstToolWithoutPriorToolUse(msgs []toolCallMsg) (int, bool) {
	sawToolUse := false
	for i, m := range msgs {
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			sawToolUse = true
			continue
		}
		if m.Role == "tool" && !sawToolUse {
			return i, true
		}
	}
	return 0, false
}
