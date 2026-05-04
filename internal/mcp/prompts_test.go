package mcp_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/mcp"
)

func TestServer_PromptsListReportsExplainRule(t *testing.T) {
	id := json.RawMessage(`14`)
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "prompts/list", Params: json.RawMessage(`{}`)},
	})
	if len(resps) != 1 {
		t.Fatalf("got %d responses, want 1", len(resps))
	}
	var result struct {
		Prompts []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Arguments   []struct {
				Name     string `json:"name"`
				Required bool   `json:"required"`
			} `json:"arguments"`
		} `json:"prompts"`
	}
	if err := json.Unmarshal(resps[0].Result, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Prompts) != 1 {
		t.Fatalf("prompts count = %d, want 1", len(result.Prompts))
	}
	p := result.Prompts[0]
	if p.Name != "explain-rule" {
		t.Errorf("name = %q, want explain-rule", p.Name)
	}
	if len(p.Arguments) != 1 || p.Arguments[0].Name != "rule_id" || !p.Arguments[0].Required {
		t.Errorf("arguments = %+v, want one required rule_id", p.Arguments)
	}
}

func TestServer_PromptsGetExplainRuleReturnsMessages(t *testing.T) {
	id := json.RawMessage(`15`)
	params, _ := json.Marshal(map[string]any{
		"name":      "explain-rule",
		"arguments": map[string]string{"rule_id": "random-seed-not-set"},
	})
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "prompts/get", Params: params},
	})
	if len(resps) != 1 {
		t.Fatalf("got %d responses, want 1", len(resps))
	}
	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %+v", resps[0].Error)
	}

	var result struct {
		Description string `json:"description"`
		Messages    []struct {
			Role    string `json:"role"`
			Content struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(resps[0].Result, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Messages) != 2 {
		t.Fatalf("messages count = %d, want 2", len(result.Messages))
	}
	if result.Messages[0].Role != "system" || result.Messages[1].Role != "user" {
		t.Errorf("role sequence = [%s %s], want [system user]",
			result.Messages[0].Role, result.Messages[1].Role)
	}
	if !strings.Contains(result.Description, "random-seed-not-set") {
		t.Errorf("description should mention rule id: %q", result.Description)
	}

	user := result.Messages[1].Content.Text
	for _, want := range []string{
		"Rule ID: random-seed-not-set",
		"Category: pipeline",
		"Severity: warning",
		"Auto-fix tier: idiomatic",
		"Scope: per-file",
	} {
		if !strings.Contains(user, want) {
			t.Errorf("user message missing %q in:\n%s", want, user)
		}
	}
}

func TestServer_PromptsGetCorpusRuleHasCorpusScope(t *testing.T) {
	// train-eval-overlap is corpus-scope; the user message's Scope
	// line should reflect that.
	id := json.RawMessage(`16`)
	params, _ := json.Marshal(map[string]any{
		"name":      "explain-rule",
		"arguments": map[string]string{"rule_id": "train-eval-overlap"},
	})
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "prompts/get", Params: params},
	})
	var result struct {
		Messages []struct {
			Content struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(resps[0].Result, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	user := result.Messages[1].Content.Text
	if !strings.Contains(user, "Scope: corpus") {
		t.Errorf("expected 'Scope: corpus' in user message:\n%s", user)
	}
}

func TestServer_PromptsGetMissingRuleIDReturnsError(t *testing.T) {
	id := json.RawMessage(`17`)
	params, _ := json.Marshal(map[string]any{
		"name":      "explain-rule",
		"arguments": map[string]string{}, // no rule_id
	})
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "prompts/get", Params: params},
	})
	if resps[0].Error == nil {
		t.Fatal("expected RPC error for missing rule_id")
	}
	if resps[0].Error.Code != -32602 {
		t.Errorf("error code = %d, want -32602", resps[0].Error.Code)
	}
}

func TestServer_PromptsGetUnknownRuleReturnsError(t *testing.T) {
	id := json.RawMessage(`18`)
	params, _ := json.Marshal(map[string]any{
		"name":      "explain-rule",
		"arguments": map[string]string{"rule_id": "no-such-rule"},
	})
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "prompts/get", Params: params},
	})
	if resps[0].Error == nil {
		t.Fatal("expected RPC error for unknown rule")
	}
	if resps[0].Error.Code != -32602 {
		t.Errorf("error code = %d, want -32602", resps[0].Error.Code)
	}
}

func TestServer_PromptsGetUnknownPromptReturnsError(t *testing.T) {
	id := json.RawMessage(`19`)
	params, _ := json.Marshal(map[string]any{"name": "nope"})
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "prompts/get", Params: params},
	})
	if resps[0].Error == nil {
		t.Fatal("expected RPC error for unknown prompt")
	}
	if resps[0].Error.Code != -32601 {
		t.Errorf("error code = %d, want -32601", resps[0].Error.Code)
	}
}

func TestServer_InitializeAdvertisesPrompts(t *testing.T) {
	id := json.RawMessage(`20`)
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "initialize", Params: json.RawMessage(`{}`)},
	})
	var result struct {
		Capabilities struct {
			Prompts struct {
				ListChanged bool `json:"listChanged"`
			} `json:"prompts"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(resps[0].Result, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Capabilities.Prompts.ListChanged {
		t.Errorf("listChanged = true, want false (prompt set is static)")
	}
}
