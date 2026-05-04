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
		ID:         "system-message-mid-conversation",
		Category:   rules.CategoryConversation,
		Severity:   diag.SeverityError,
		Confidence: rules.ConfidenceHigh,
		Fix:        rules.FixNone,
		Needs:      rules.NeedsJSONL,
		Check:      checkSystemMidConversation,
	})
}

// checkSystemMidConversation flags rows whose `messages` array has a
// `system` role at any index other than 0. The standard chat shape
// is one optional system prompt at the start, then user/assistant
// alternation; a system message in the middle is almost always a
// template-rendering bug.
//
// One finding per row, anchored at the first offending system
// message's index.
func checkSystemMidConversation(ctx *rules.Context, emit func(diag.Finding)) {
	if ctx == nil || ctx.File == nil || ctx.File.Kind != scanner.KindJSONL {
		return
	}
	path := ctx.File.Path
	_ = scanner.StreamJSONL(path, func(row int, line []byte) error {
		checkSystemMidConversationRow(path, row, line, emit)
		return nil
	})
}

func checkSystemMidConversationRow(path string, row int, line []byte, emit func(diag.Finding)) {
	if len(line) == 0 {
		return
	}
	var r roleRow
	if json.Unmarshal(line, &r) != nil {
		return
	}
	idx, ok := firstMidConversationSystem(r.Messages)
	if !ok {
		return
	}
	emit(diag.Finding{
		RuleID:   "system-message-mid-conversation",
		Severity: diag.SeverityError,
		Message: fmt.Sprintf(
			"system message at index %d (only index 0 is conventional); likely a misformatted prompt template",
			idx),
		Location: diag.Location{Path: path, Row: row},
	})
}

func firstMidConversationSystem(msgs []roleMessage) (int, bool) {
	for i := 1; i < len(msgs); i++ {
		if msgs[i].Role == "system" {
			return i, true
		}
	}
	return 0, false
}
