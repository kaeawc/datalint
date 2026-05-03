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
		ID:         "role-inversion",
		Category:   rules.CategoryConversation,
		Severity:   diag.SeverityError,
		Confidence: rules.ConfidenceHigh,
		Fix:        rules.FixNone,
		Needs:      rules.NeedsJSONL,
		Check:      checkRoleInversion,
	})
}

// roleMessage is the minimal shape we read; ignoring everything else
// keeps the rule cheap and tolerant of schema variants.
type roleMessage struct {
	Role string `json:"role"`
}

type roleRow struct {
	Messages []roleMessage `json:"messages"`
}

// checkRoleInversion flags chat rows where two assistant messages
// appear back-to-back with no intervening user or tool turn. v0 only
// covers the assistant-after-assistant case; other inversions
// (system-mid-conversation, tool-without-tool-call) are tracked as
// separate rules per the README.
//
// One finding per row keeps output proportional to the bug count, not
// the message count — a row with three consecutive assistant turns
// emits once, not twice.
func checkRoleInversion(ctx *rules.Context, emit func(diag.Finding)) {
	if ctx == nil || ctx.File == nil || ctx.File.Kind != scanner.KindJSONL {
		return
	}
	path := ctx.File.Path
	_ = scanner.StreamJSONL(path, func(row int, line []byte) error {
		checkRoleInversionRow(path, row, line, emit)
		return nil
	})
}

func checkRoleInversionRow(path string, row int, line []byte, emit func(diag.Finding)) {
	if len(line) == 0 {
		return
	}
	var r roleRow
	if json.Unmarshal(line, &r) != nil {
		return
	}
	idx, ok := firstAssistantAfterAssistant(r.Messages)
	if !ok {
		return
	}
	emit(diag.Finding{
		RuleID:   "role-inversion",
		Severity: diag.SeverityError,
		Message: fmt.Sprintf(
			"assistant message at index %d follows assistant at index %d with no user/tool between",
			idx, idx-1),
		Location: diag.Location{Path: path, Row: row},
	})
}

func firstAssistantAfterAssistant(msgs []roleMessage) (int, bool) {
	for i := 1; i < len(msgs); i++ {
		if msgs[i].Role == "assistant" && msgs[i-1].Role == "assistant" {
			return i, true
		}
	}
	return 0, false
}
