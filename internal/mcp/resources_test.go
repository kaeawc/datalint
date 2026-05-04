package mcp_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kaeawc/datalint/internal/mcp"
)

func TestServer_ResourcesListReportsBoth(t *testing.T) {
	id := json.RawMessage(`9`)
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "resources/list", Params: json.RawMessage(`{}`)},
	})
	if len(resps) != 1 {
		t.Fatalf("got %d responses, want 1", len(resps))
	}
	var result struct {
		Resources []struct {
			URI         string `json:"uri"`
			Name        string `json:"name"`
			Description string `json:"description"`
			MimeType    string `json:"mimeType"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(resps[0].Result, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := map[string]string{}
	for _, r := range result.Resources {
		got[r.URI] = r.MimeType
	}
	for uri, wantMime := range map[string]string{
		"datalint:rules/index":    "text/markdown",
		"datalint:config/example": "text/yaml",
	} {
		mime, ok := got[uri]
		if !ok {
			t.Errorf("resources/list missing %q", uri)
			continue
		}
		if mime != wantMime {
			t.Errorf("%s mimeType = %q, want %q", uri, mime, wantMime)
		}
	}
}

func TestServer_ResourcesReadRulesIndex(t *testing.T) {
	id := json.RawMessage(`10`)
	params, _ := json.Marshal(map[string]any{"uri": "datalint:rules/index"})
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "resources/read", Params: params},
	})
	if len(resps) != 1 {
		t.Fatalf("got %d responses, want 1", len(resps))
	}
	var result struct {
		Contents []struct {
			URI      string `json:"uri"`
			MimeType string `json:"mimeType"`
			Text     string `json:"text"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(resps[0].Result, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("contents = %d, want 1", len(result.Contents))
	}
	c := result.Contents[0]
	if c.URI != "datalint:rules/index" {
		t.Errorf("uri = %q", c.URI)
	}
	if c.MimeType != "text/markdown" {
		t.Errorf("mimeType = %q, want text/markdown", c.MimeType)
	}
	// Spot-check that a few well-known rules appear in the rendered table.
	for _, want := range []string{
		"# datalint rules",
		"| `jsonl-malformed-line`",
		"| `random-seed-not-set`",
		"| `cross-dataset-overlap`",
		"corpus", // scope label for corpus rules
	} {
		if !strings.Contains(c.Text, want) {
			t.Errorf("rules-index text missing %q", want)
		}
	}
}

func TestServer_ResourcesReadConfigExample(t *testing.T) {
	id := json.RawMessage(`11`)
	params, _ := json.Marshal(map[string]any{"uri": "datalint:config/example"})
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "resources/read", Params: params},
	})
	var result struct {
		Contents []struct {
			MimeType string `json:"mimeType"`
			Text     string `json:"text"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(resps[0].Result, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Contents[0].MimeType != "text/yaml" {
		t.Errorf("mimeType = %q, want text/yaml", result.Contents[0].MimeType)
	}
	for _, want := range []string{
		"enable:",
		"disable:",
		"enum-drift:",
		"lock_in_rows:",
		"train-eval-overlap:",
		"near_dup_threshold:",
		"cross-dataset-overlap:",
		"anchor: later",
	} {
		if !strings.Contains(result.Contents[0].Text, want) {
			t.Errorf("config-example text missing %q", want)
		}
	}
}

func TestServer_ResourcesReadUnknownURI(t *testing.T) {
	id := json.RawMessage(`12`)
	params, _ := json.Marshal(map[string]any{"uri": "datalint:nope"})
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "resources/read", Params: params},
	})
	if resps[0].Error == nil {
		t.Fatalf("expected RPC error for unknown URI, got %+v", resps[0])
	}
	if resps[0].Error.Code != -32601 {
		t.Errorf("error code = %d, want -32601", resps[0].Error.Code)
	}
}

func TestServer_InitializeAdvertisesResources(t *testing.T) {
	id := json.RawMessage(`13`)
	resps := runOnce(t, []*mcp.Message{
		{JSONRPC: "2.0", ID: &id, Method: "initialize", Params: json.RawMessage(`{}`)},
	})
	var result struct {
		Capabilities struct {
			Resources struct {
				ListChanged bool `json:"listChanged"`
				Subscribe   bool `json:"subscribe"`
			} `json:"resources"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(resps[0].Result, &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Both flags are explicitly false; what matters is that the
	// resources object is *present* — clients use that to decide
	// whether to send resources/list at all.
	if result.Capabilities.Resources.ListChanged || result.Capabilities.Resources.Subscribe {
		t.Errorf("resource flags should both be false, got %+v", result.Capabilities.Resources)
	}
}
