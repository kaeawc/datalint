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
	var explain *struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Arguments   []struct {
			Name     string `json:"name"`
			Required bool   `json:"required"`
		} `json:"arguments"`
	}
	for i := range result.Prompts {
		if result.Prompts[i].Name == "explain-rule" {
			explain = &result.Prompts[i]
			break
		}
	}
	if explain == nil {
		t.Fatalf("prompts/list missing 'explain-rule': %+v", result.Prompts)
	}
	if len(explain.Arguments) != 1 || explain.Arguments[0].Name != "rule_id" || !explain.Arguments[0].Required {
		t.Errorf("explain-rule arguments = %+v, want one required rule_id", explain.Arguments)
	}
}

func TestServer_PromptsListReportsAllThreePrompts(t *testing.T) {
	id := json.RawMessage(`30`)
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "prompts/list", Params: json.RawMessage(`{}`)},
	})
	var result struct {
		Prompts []struct {
			Name      string `json:"name"`
			Arguments []struct {
				Name     string `json:"name"`
				Required bool   `json:"required"`
			} `json:"arguments"`
		} `json:"prompts"`
	}
	if err := json.Unmarshal(resps[0].Result, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	names := map[string]bool{}
	for _, p := range result.Prompts {
		names[p.Name] = true
	}
	for _, want := range []string{"explain-rule", "draft-fix", "review-corpus"} {
		if !names[want] {
			t.Errorf("prompts/list missing %q", want)
		}
	}
	// Spot-check review-corpus required vs optional flags.
	for _, p := range result.Prompts {
		if p.Name != "review-corpus" {
			continue
		}
		req := map[string]bool{}
		for _, a := range p.Arguments {
			req[a.Name] = a.Required
		}
		if !req["paths"] {
			t.Errorf("review-corpus paths should be required")
		}
		for _, opt := range []string{"dataset_names", "goals"} {
			if req[opt] {
				t.Errorf("review-corpus %q should be optional", opt)
			}
		}
	}
}

func TestServer_PromptsGetReviewCorpusReturnsMessages(t *testing.T) {
	id := json.RawMessage(`31`)
	params, _ := json.Marshal(map[string]any{
		"name": "review-corpus",
		"arguments": map[string]string{
			"paths":         "train.jsonl,eval.jsonl,pipeline.py",
			"dataset_names": "train,eval",
			"goals":         "eval-set safety",
		},
	})
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "prompts/get", Params: params},
	})
	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %+v", resps[0].Error)
	}
	var result struct {
		Description string `json:"description"`
		Messages    []struct {
			Role    string `json:"role"`
			Content struct {
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
	user := result.Messages[1].Content.Text
	for _, want := range []string{
		"Paths: train.jsonl,eval.jsonl,pipeline.py",
		"Dataset names: train,eval",
		"Goals: eval-set safety",
	} {
		if !strings.Contains(user, want) {
			t.Errorf("user message missing %q in:\n%s", want, user)
		}
	}
}

func TestServer_PromptsGetReviewCorpusOmitsOptionalLines(t *testing.T) {
	id := json.RawMessage(`32`)
	params, _ := json.Marshal(map[string]any{
		"name":      "review-corpus",
		"arguments": map[string]string{"paths": "data.jsonl"},
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
	if !strings.Contains(user, "Paths: data.jsonl") {
		t.Errorf("required Paths line missing:\n%s", user)
	}
	for _, mustNot := range []string{"Dataset names:", "Goals:"} {
		if strings.Contains(user, mustNot) {
			t.Errorf("%q should be omitted when arg not provided:\n%s", mustNot, user)
		}
	}
}

func TestServer_PromptsGetReviewCorpusMissingPathsReturnsError(t *testing.T) {
	id := json.RawMessage(`33`)
	params, _ := json.Marshal(map[string]any{
		"name":      "review-corpus",
		"arguments": map[string]string{}, // no paths
	})
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "prompts/get", Params: params},
	})
	if resps[0].Error == nil {
		t.Fatal("expected RPC error for missing paths")
	}
	if resps[0].Error.Code != -32602 {
		t.Errorf("error code = %d, want -32602", resps[0].Error.Code)
	}
	if !strings.Contains(resps[0].Error.Message, "paths") {
		t.Errorf("error message should cite missing paths: %q", resps[0].Error.Message)
	}
}

func TestServer_PromptsListReportsBothExplainAndDraftFix(t *testing.T) {
	id := json.RawMessage(`21`)
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "prompts/list", Params: json.RawMessage(`{}`)},
	})
	var result struct {
		Prompts []struct {
			Name      string `json:"name"`
			Arguments []struct {
				Name     string `json:"name"`
				Required bool   `json:"required"`
			} `json:"arguments"`
		} `json:"prompts"`
	}
	if err := json.Unmarshal(resps[0].Result, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := map[string]struct {
		Args     []string
		Required map[string]bool
	}{}
	for _, p := range result.Prompts {
		entry := got[p.Name]
		entry.Required = map[string]bool{}
		for _, a := range p.Arguments {
			entry.Args = append(entry.Args, a.Name)
			entry.Required[a.Name] = a.Required
		}
		got[p.Name] = entry
	}
	if _, ok := got["explain-rule"]; !ok {
		t.Errorf("prompts/list missing explain-rule")
	}
	df, ok := got["draft-fix"]
	if !ok {
		t.Fatalf("prompts/list missing draft-fix")
	}
	for _, want := range []string{"rule_id", "path", "line", "row", "message"} {
		if !contains(df.Args, want) {
			t.Errorf("draft-fix arguments missing %q: %v", want, df.Args)
		}
	}
	for _, mustReq := range []string{"rule_id", "path"} {
		if !df.Required[mustReq] {
			t.Errorf("draft-fix %q should be required", mustReq)
		}
	}
	for _, mustNotReq := range []string{"line", "row", "message"} {
		if df.Required[mustNotReq] {
			t.Errorf("draft-fix %q should be optional", mustNotReq)
		}
	}
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
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

func TestServer_PromptsGetDraftFixReturnsMessages(t *testing.T) {
	id := json.RawMessage(`22`)
	params, _ := json.Marshal(map[string]any{
		"name": "draft-fix",
		"arguments": map[string]string{
			"rule_id": "random-seed-not-set",
			"path":    "pipeline.py",
			"line":    "4",
			"message": "random.shuffle called without random.seed",
		},
	})
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "prompts/get", Params: params},
	})
	if resps[0].Error != nil {
		t.Fatalf("unexpected error: %+v", resps[0].Error)
	}
	var result struct {
		Description string `json:"description"`
		Messages    []struct {
			Role    string `json:"role"`
			Content struct {
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
	if !strings.Contains(result.Description, "random-seed-not-set") || !strings.Contains(result.Description, "pipeline.py") {
		t.Errorf("description should cite rule + path: %q", result.Description)
	}
	user := result.Messages[1].Content.Text
	for _, want := range []string{
		"Rule ID: random-seed-not-set",
		"Path: pipeline.py",
		"Line: 4",
		"Message: random.shuffle called without random.seed",
	} {
		if !strings.Contains(user, want) {
			t.Errorf("user message missing %q in:\n%s", want, user)
		}
	}
	// Optional args not provided must not appear with empty values.
	if strings.Contains(user, "Row:") {
		t.Errorf("Row line should be omitted when arg not provided:\n%s", user)
	}
}

func TestServer_PromptsGetDraftFixOmitsOptionalLines(t *testing.T) {
	// Only required args supplied — optional Line/Row/Message lines
	// must not render with empty values.
	id := json.RawMessage(`23`)
	params, _ := json.Marshal(map[string]any{
		"name": "draft-fix",
		"arguments": map[string]string{
			"rule_id": "random-seed-not-set",
			"path":    "pipeline.py",
		},
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
	for _, mustNot := range []string{"Line:", "Row:", "Message:"} {
		if strings.Contains(user, mustNot) {
			t.Errorf("%q should be omitted when arg not provided:\n%s", mustNot, user)
		}
	}
	if !strings.Contains(user, "Path: pipeline.py") {
		t.Errorf("required Path line missing:\n%s", user)
	}
}

func TestServer_PromptsGetDraftFixMissingPathReturnsError(t *testing.T) {
	id := json.RawMessage(`24`)
	params, _ := json.Marshal(map[string]any{
		"name":      "draft-fix",
		"arguments": map[string]string{"rule_id": "random-seed-not-set"},
	})
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "prompts/get", Params: params},
	})
	if resps[0].Error == nil {
		t.Fatal("expected RPC error for missing path")
	}
	if resps[0].Error.Code != -32602 {
		t.Errorf("error code = %d, want -32602", resps[0].Error.Code)
	}
	if !strings.Contains(resps[0].Error.Message, "path") {
		t.Errorf("error message should cite missing path: %q", resps[0].Error.Message)
	}
}

func TestServer_PromptsGetDraftFixUnknownRuleReturnsError(t *testing.T) {
	id := json.RawMessage(`25`)
	params, _ := json.Marshal(map[string]any{
		"name": "draft-fix",
		"arguments": map[string]string{
			"rule_id": "nope",
			"path":    "pipeline.py",
		},
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
