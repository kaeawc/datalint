package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/kaeawc/datalint/internal/rules"
)

// promptName* are the stable identifiers exposed via prompts/list.
const (
	promptNameExplainRule = "explain-rule"
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
	if p.Name != promptNameExplainRule {
		return s.respond(m, nil, &RPCError{
			Code:    -32601,
			Message: fmt.Sprintf("unknown prompt: %q", p.Name),
		})
	}
	ruleID, ok := p.Arguments["rule_id"]
	if !ok || ruleID == "" {
		return s.respond(m, nil, &RPCError{
			Code:    -32602,
			Message: "missing required argument: rule_id",
		})
	}
	rule := rules.ByID(ruleID)
	if rule == nil {
		return s.respond(m, nil, &RPCError{
			Code:    -32602,
			Message: fmt.Sprintf("unknown rule: %q", ruleID),
		})
	}

	result := promptGetResult{
		Description: fmt.Sprintf("Explain the %s datalint rule.", ruleID),
		Messages: []promptMessage{
			{
				Role: "system",
				Content: promptContent{
					Type: "text",
					Text: explainRuleSystemPrompt,
				},
			},
			{
				Role: "user",
				Content: promptContent{
					Type: "text",
					Text: explainRuleUserPrompt(rule),
				},
			},
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
