package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kaeawc/datalint/internal/rules"
)

// promptName* are the stable identifiers exposed via prompts/list.
const (
	promptNameExplainRule  = "explain-rule"
	promptNameDraftFix     = "draft-fix"
	promptNameReviewCorpus = "review-corpus"
)

// promptDescriptor is the MCP shape returned from prompts/list.
type promptDescriptor struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Arguments   []promptArgument `json:"arguments,omitempty"`
}

type promptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// promptMessage is the MCP role+content shape inside prompts/get's
// `messages` array. v0 only emits "text" content blocks.
type promptMessage struct {
	Role    string        `json:"role"`
	Content promptContent `json:"content"`
}

type promptContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// promptGetResult is the body of the prompts/get response.
type promptGetResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []promptMessage `json:"messages"`
}

func (s *Server) respondPromptsList(m *Message) error {
	prompts := []promptDescriptor{
		{
			Name:        promptNameExplainRule,
			Description: "Have the model explain a datalint rule's bug class, why it matters for training-data quality, and what to do when a finding fires.",
			Arguments: []promptArgument{
				{
					Name:        "rule_id",
					Description: "The rule ID to explain (e.g. random-seed-not-set, train-eval-overlap).",
					Required:    true,
				},
			},
		},
		{
			Name:        promptNameDraftFix,
			Description: "Have the model draft a concrete patch for a finding (rule + path + optional row/line/message).",
			Arguments: []promptArgument{
				{Name: "rule_id", Description: "The rule ID the finding fired (e.g. random-seed-not-set).", Required: true},
				{Name: "path", Description: "The file the finding cites (Python source for code rules, JSONL for data rules).", Required: true},
				{Name: "line", Description: "1-based source line for code findings (optional).", Required: false},
				{Name: "row", Description: "1-based row index for data findings (optional).", Required: false},
				{Name: "message", Description: "The finding's message text (optional; helps the model phrase the patch).", Required: false},
			},
		},
		{
			Name:        promptNameReviewCorpus,
			Description: "Have the model review a corpus shape and suggest a starting datalint configuration plus commands to run.",
			Arguments: []promptArgument{
				{Name: "paths", Description: "Comma-separated list of file paths in the corpus (.jsonl, .py, .parquet).", Required: true},
				{Name: "dataset_names", Description: "Comma-separated dataset/split names if the corpus has named splits (e.g. train,eval,test).", Required: false},
				{Name: "goals", Description: "What the user is optimizing for (e.g. 'eval-set safety', 'schema stability'); shapes the config recommendation.", Required: false},
			},
		},
	}
	body, err := json.Marshal(map[string]any{"prompts": prompts})
	if err != nil {
		return err
	}
	return s.respond(m, body, nil)
}

type promptsGetParams struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments"`
}

func (s *Server) respondPromptsGet(m *Message) error {
	var p promptsGetParams
	if err := json.Unmarshal(m.Params, &p); err != nil {
		return s.respond(m, nil, &RPCError{Code: -32602, Message: "invalid params: " + err.Error()})
	}
	switch p.Name {
	case promptNameExplainRule:
		return s.handleExplainRule(m, p.Arguments)
	case promptNameDraftFix:
		return s.handleDraftFix(m, p.Arguments)
	case promptNameReviewCorpus:
		return s.handleReviewCorpus(m, p.Arguments)
	}
	return s.respond(m, nil, &RPCError{
		Code:    -32601,
		Message: fmt.Sprintf("unknown prompt: %q", p.Name),
	})
}

// requireRule pulls rule_id from args and looks it up in the registry,
// returning the rule + nil on success or a ready-to-send RPCError on
// failure. Callers respond with the error directly.
func requireRule(args map[string]string) (*rules.Rule, *RPCError) {
	ruleID, ok := args["rule_id"]
	if !ok || ruleID == "" {
		return nil, &RPCError{Code: -32602, Message: "missing required argument: rule_id"}
	}
	r := rules.ByID(ruleID)
	if r == nil {
		return nil, &RPCError{Code: -32602, Message: fmt.Sprintf("unknown rule: %q", ruleID)}
	}
	return r, nil
}

func (s *Server) handleExplainRule(m *Message, args map[string]string) error {
	rule, rpcErr := requireRule(args)
	if rpcErr != nil {
		return s.respond(m, nil, rpcErr)
	}
	return s.respondPromptResult(m, fmt.Sprintf("Explain the %s datalint rule.", rule.ID),
		explainRuleSystemPrompt, explainRuleUserPrompt(rule))
}

func (s *Server) handleDraftFix(m *Message, args map[string]string) error {
	rule, rpcErr := requireRule(args)
	if rpcErr != nil {
		return s.respond(m, nil, rpcErr)
	}
	path := args["path"]
	if path == "" {
		return s.respond(m, nil, &RPCError{Code: -32602, Message: "missing required argument: path"})
	}
	return s.respondPromptResult(m,
		fmt.Sprintf("Draft a fix for a %s finding at %s.", rule.ID, path),
		draftFixSystemPrompt, draftFixUserPrompt(rule, args))
}

// respondPromptResult packages the canonical system+user pair into a
// promptGetResult and writes it. Both prompt handlers go through here
// so the messages array shape stays uniform.
func (s *Server) respondPromptResult(m *Message, description, systemText, userText string) error {
	result := promptGetResult{
		Description: description,
		Messages: []promptMessage{
			{Role: "system", Content: promptContent{Type: "text", Text: systemText}},
			{Role: "user", Content: promptContent{Type: "text", Text: userText}},
		},
	}
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return s.respond(m, body, nil)
}

const explainRuleSystemPrompt = `You are a datalint rule explainer. Given a rule's metadata, write three short paragraphs:

1. The bug class — what kind of training-data defect this rule catches.
2. Why it matters — the failure mode at training/eval time when the bug ships.
3. What to do — concrete steps the user takes when this rule fires (e.g., remove rows, edit code, raise a config threshold).

Keep each paragraph under 4 sentences. Don't speculate beyond the metadata you've been given.`

func explainRuleUserPrompt(r *rules.Rule) string {
	return fmt.Sprintf(
		`Rule ID: %s
Category: %s
Severity: %s
Confidence: %s
Auto-fix tier: %s
Scope: %s

Explain this rule per the system instructions.`,
		r.ID,
		string(r.Category),
		r.Severity,
		r.Confidence,
		r.Fix,
		scopeLabel(r),
	)
}

const draftFixSystemPrompt = `You are a datalint fix author. Given a finding's metadata, draft a concrete patch that addresses the underlying bug:

1. For code findings (.py paths), output a unified-diff-style patch. Keep the change minimal — only fix what the rule flags, no surrounding refactors.
2. For data findings (.jsonl paths), recommend either a row replacement (with the corrected JSON) or a row removal, citing the row number.
3. If the fix needs information you don't have (e.g., a sensible random seed value), pick a defensible default and note it.
4. End with a one-line rationale tying the patch back to the rule's bug class.

If the rule doesn't have a typical mechanical fix (e.g., role-inversion is a data quality issue with no automatic patch), say so and suggest the next step the user should take.`

func (s *Server) handleReviewCorpus(m *Message, args map[string]string) error {
	paths := args["paths"]
	if paths == "" {
		return s.respond(m, nil, &RPCError{Code: -32602, Message: "missing required argument: paths"})
	}
	return s.respondPromptResult(m,
		"Review the corpus and suggest a datalint configuration.",
		reviewCorpusSystemPrompt, reviewCorpusUserPrompt(args))
}

const reviewCorpusSystemPrompt = `You are a datalint corpus reviewer. Given a list of paths and optional context, suggest a starting datalint configuration and the commands to run:

1. Identify which rules apply: data rules for .jsonl/.parquet paths, code rules for .py paths, leakage rules when the user has multiple splits.
2. Recommend config thresholds based on the corpus shape and the user's stated goals (or sensible defaults if unspecified).
3. Provide concrete CLI commands the user can run (lint, fix, diff, train-eval-overlap, cross-dataset-overlap).
4. Flag obvious gaps — e.g., if the user listed only train and eval but no test split, suggest splitting eval into a held-out portion.

Don't speculate beyond what the user told you. Note assumptions explicitly.`

func reviewCorpusUserPrompt(args map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Paths: %s\n", args["paths"])
	if names := args["dataset_names"]; names != "" {
		fmt.Fprintf(&b, "Dataset names: %s\n", names)
	}
	if goals := args["goals"]; goals != "" {
		fmt.Fprintf(&b, "Goals: %s\n", goals)
	}
	b.WriteString("\nReview this corpus per the system instructions.")
	return b.String()
}

func draftFixUserPrompt(r *rules.Rule, args map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Rule ID: %s\n", r.ID)
	fmt.Fprintf(&b, "Category: %s\n", r.Category)
	fmt.Fprintf(&b, "Severity: %s\n", r.Severity)
	fmt.Fprintf(&b, "Confidence: %s\n", r.Confidence)
	fmt.Fprintf(&b, "Auto-fix tier: %s\n", r.Fix)
	fmt.Fprintf(&b, "Scope: %s\n", scopeLabel(r))
	if path := args["path"]; path != "" {
		fmt.Fprintf(&b, "Path: %s\n", path)
	}
	if line := args["line"]; line != "" {
		fmt.Fprintf(&b, "Line: %s\n", line)
	}
	if row := args["row"]; row != "" {
		fmt.Fprintf(&b, "Row: %s\n", row)
	}
	if msg := args["message"]; msg != "" {
		fmt.Fprintf(&b, "Message: %s\n", msg)
	}
	b.WriteString("\nDraft a fix per the system instructions.")
	return b.String()
}
