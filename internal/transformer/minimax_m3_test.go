package transformer

import (
	"encoding/json"
	"strings"
	"testing"

	"go-proxy/internal/config"
	"go-proxy/pkg/types"
)

func TestIsMiniMaxM3(t *testing.T) {
	if !IsMiniMaxM3("minimax-m3") {
		t.Error("expected minimax-m3 to be recognized")
	}
	if IsMiniMaxM3("minimax-m2.7") || IsMiniMaxM3("qwen3.7-max") {
		t.Error("expected non-m3 models to be rejected")
	}
}

func TestApplyM3RequestHardening_InjectStrictPrompt(t *testing.T) {
	cfg := config.ModelConfig{ModelID: "minimax-m3"}
	req := &types.MessageRequest{
		Model: "minimax-m3",
	}

	if err := ApplyM3RequestHardening(req, nil, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := req.SystemText()
	if !strings.Contains(text, "CRITICAL: When calling a tool") {
		t.Errorf("strict prompt not injected: %q", text)
	}
}

func TestApplyM3RequestHardening_PreservesExistingSystemPrompt(t *testing.T) {
	cfg := config.ModelConfig{ModelID: "minimax-m3"}
	req := &types.MessageRequest{
		Model:  "minimax-m3",
		System: json.RawMessage(`"You are helpful."`),
	}

	if err := ApplyM3RequestHardening(req, nil, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := req.SystemText()
	if !strings.HasPrefix(text, "You are helpful.") {
		t.Errorf("existing system prompt lost: %q", text)
	}
	if !strings.Contains(text, "CRITICAL: When calling a tool") {
		t.Errorf("strict prompt not appended: %q", text)
	}
}

func TestApplyM3RequestHardening_PreservesSystemArray(t *testing.T) {
	cfg := config.ModelConfig{ModelID: "minimax-m3"}
	req := &types.MessageRequest{
		Model:  "minimax-m3",
		System: json.RawMessage(`[{"type":"text","text":"You are helpful."},{"type":"text","text":"Be concise."}]`),
	}

	if err := ApplyM3RequestHardening(req, nil, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := req.SystemText()
	if !strings.HasPrefix(text, "You are helpful.Be concise.") {
		t.Errorf("existing system array text lost: %q", text)
	}
	if !strings.Contains(text, "CRITICAL: When calling a tool") {
		t.Errorf("strict prompt not appended to array: %q", text)
	}

	// Verify the array format is preserved.
	var blocks []types.SystemContentBlock
	if err := json.Unmarshal(req.System, &blocks); err != nil {
		t.Fatalf("system is no longer an array: %v", err)
	}
	if len(blocks) != 2 {
		t.Errorf("expected 2 system blocks, got %d", len(blocks))
	}
}

func TestApplyM3RequestHardening_NoOpForNonM3(t *testing.T) {
	cfg := config.ModelConfig{ModelID: "minimax-m2.7"}
	req := &types.MessageRequest{
		Model:  "minimax-m2.7",
		System: json.RawMessage(`"You are helpful."`),
	}

	if err := ApplyM3RequestHardening(req, nil, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.SystemText() != "You are helpful." {
		t.Errorf("non-M3 request was modified: %q", req.SystemText())
	}
}

func TestApplyM3RequestHardening_TightenSchemas(t *testing.T) {
	cfg := config.ModelConfig{ModelID: "minimax-m3"}
	tools := []types.ToolDef{
		{
			Type: "function",
			Function: types.FunctionDef{
				Name:        "bash",
				Description: "Run a command",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {"command": {"type": "string"}},
					"required": ["command"]
				}`),
			},
		},
	}
	req := &types.MessageRequest{
		Model: "minimax-m3",
		Tools: []types.Tool{
			{Name: "bash", InputSchema: json.RawMessage(`{"type": "object", "required": ["command"]}`)},
		},
	}

	if err := ApplyM3RequestHardening(req, tools, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(req.Tools[0].InputSchema, &schema); err != nil {
		t.Fatalf("failed to parse tightened schema: %v", err)
	}
	if _, ok := schema["properties"]; !ok {
		t.Errorf("tightened schema missing properties: %v", schema)
	}
	if additional, ok := schema["additionalProperties"]; !ok || additional != false {
		t.Errorf("tightened schema missing additionalProperties=false: %v", schema)
	}
}

func TestBuildM3RequestBody_AddsReasoningSplit(t *testing.T) {
	cfg := config.ModelConfig{ModelID: "minimax-m3"}
	req := &types.MessageRequest{Model: "minimax-m3", MaxTokens: 4096, Messages: []types.Message{}}

	body, err := BuildM3RequestBody(req, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(body, &obj); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}
	if obj["reasoning_split"] != true {
		t.Errorf("expected reasoning_split=true, got %v", obj["reasoning_split"])
	}
}

func TestBuildM3RequestBody_NoReasoningSplitForNonM3(t *testing.T) {
	cfg := config.ModelConfig{ModelID: "minimax-m2.7"}
	req := &types.MessageRequest{Model: "minimax-m2.7", MaxTokens: 4096, Messages: []types.Message{}}

	body, err := BuildM3RequestBody(req, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(body, &obj); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}
	if _, ok := obj["reasoning_split"]; ok {
		t.Errorf("expected no reasoning_split for non-M3 model")
	}
}

func TestBuildM3RequestBody_DisabledReasoningSplit(t *testing.T) {
	falseVal := false
	cfg := config.ModelConfig{ModelID: "minimax-m3", M3ReasoningSplit: &falseVal}
	req := &types.MessageRequest{Model: "minimax-m3", MaxTokens: 4096, Messages: []types.Message{}}

	body, err := BuildM3RequestBody(req, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(body, &obj); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}
	if _, ok := obj["reasoning_split"]; ok {
		t.Errorf("expected no reasoning_split when disabled")
	}
}

func TestSanitizeM3Response_DropsInvalidToolName(t *testing.T) {
	cfg := config.ModelConfig{ModelID: "minimax-m3"}
	tools := []types.ToolDef{
		{Type: "function", Function: types.FunctionDef{Name: "bash"}},
	}
	resp := &types.ChatCompletionResponse{
		Choices: []types.Choice{
			{
				Message: types.ChatMessage{
					Role:    "assistant",
					Content: json.RawMessage(`""`),
					ToolCalls: []types.ToolCall{
						{ID: "call_1", Type: "function", Function: types.FunctionCall{Name: "invalid", Arguments: `{}`}},
					},
				},
			},
		},
	}

	SanitizeM3Response(resp, tools, cfg)

	if len(resp.Choices[0].Message.ToolCalls) != 0 {
		t.Errorf("expected invalid tool call to be dropped, got %v", resp.Choices[0].Message.ToolCalls)
	}
	if !strings.Contains(resp.Choices[0].Message.ContentText(), "invalid and removed") {
		t.Errorf("expected warning text in content, got %q", resp.Choices[0].Message.ContentText())
	}
}

func TestSanitizeM3Response_FillsMissingRequiredArgument(t *testing.T) {
	cfg := config.ModelConfig{ModelID: "minimax-m3"}
	tools := []types.ToolDef{
		{
			Type: "function",
			Function: types.FunctionDef{
				Name: "bash",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {"command": {"type": "string"}},
					"required": ["command"]
				}`),
			},
		},
	}
	resp := &types.ChatCompletionResponse{
		Choices: []types.Choice{
			{
				Message: types.ChatMessage{
					Role:    "assistant",
					Content: json.RawMessage(`""`),
					ToolCalls: []types.ToolCall{
						{ID: "call_1", Type: "function", Function: types.FunctionCall{Name: "bash", Arguments: `{}`}},
					},
				},
			},
		},
	}

	SanitizeM3Response(resp, tools, cfg)

	if len(resp.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("expected tool call to survive, got %v", resp.Choices[0].Message.ToolCalls)
	}
	args := resp.Choices[0].Message.ToolCalls[0].Function.Arguments
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(args), &parsed); err != nil {
		t.Fatalf("arguments not valid JSON: %q", args)
	}
	if parsed["command"] != "" {
		t.Errorf("expected command to be defaulted to empty string, got %v", parsed["command"])
	}
}

func TestSanitizeM3Response_NoOpForNonM3(t *testing.T) {
	cfg := config.ModelConfig{ModelID: "minimax-m2.7"}
	resp := &types.ChatCompletionResponse{
		Choices: []types.Choice{
			{
				Message: types.ChatMessage{
					ToolCalls: []types.ToolCall{
						{ID: "call_1", Type: "function", Function: types.FunctionCall{Name: "invalid", Arguments: `{}`}},
					},
				},
			},
		},
	}

	SanitizeM3Response(resp, nil, cfg)

	if len(resp.Choices[0].Message.ToolCalls) != 1 {
		t.Errorf("expected non-M3 response to be unchanged, got %v", resp.Choices[0].Message.ToolCalls)
	}
}

func TestSanitizeM3StreamResponse_DropsInvalidToolName(t *testing.T) {
	cfg := config.ModelConfig{ModelID: "minimax-m3"}
	tools := []types.ToolDef{
		{Type: "function", Function: types.FunctionDef{Name: "bash"}},
	}
	resp := &types.MessageResponse{
		Content: []types.ContentBlock{
			{Type: "tool_use", ID: "call_1", Name: "invalid", Input: json.RawMessage(`{}`)},
		},
	}

	SanitizeM3StreamResponse(resp, tools, cfg)

	if len(resp.Content) != 1 || resp.Content[0].Type != "text" {
		t.Errorf("expected invalid tool_use to be replaced with warning text, got %v", resp.Content)
	}
}

func TestSanitizeM3StreamResponse_FillsMissingRequiredArgument(t *testing.T) {
	cfg := config.ModelConfig{ModelID: "minimax-m3"}
	tools := []types.ToolDef{
		{
			Type: "function",
			Function: types.FunctionDef{
				Name: "bash",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {"command": {"type": "string"}},
					"required": ["command"]
				}`),
			},
		},
	}
	resp := &types.MessageResponse{
		Content: []types.ContentBlock{
			{Type: "tool_use", ID: "call_1", Name: "bash", Input: json.RawMessage(`{}`)},
		},
	}

	SanitizeM3StreamResponse(resp, tools, cfg)

	if len(resp.Content) != 1 || resp.Content[0].Type != "tool_use" {
		t.Fatalf("expected tool_use to survive, got %v", resp.Content)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(resp.Content[0].Input, &parsed); err != nil {
		t.Fatalf("input not valid JSON: %q", resp.Content[0].Input)
	}
	if parsed["command"] != "" {
		t.Errorf("expected command to be defaulted, got %v", parsed["command"])
	}
}

func TestTryParseToolArguments_ExtractsFromMarkdown(t *testing.T) {
	args := "Some thinking text\n```json\n{\"command\":\"ls\"}\n```"
	parsed, raw, ok := tryParseToolArguments(args)
	if !ok {
		t.Fatal("expected to extract JSON from markdown")
	}
	if raw != `{"command":"ls"}` {
		t.Errorf("unexpected raw: %q", raw)
	}
	if parsed["command"] != "ls" {
		t.Errorf("unexpected parsed command: %v", parsed["command"])
	}
}

func TestTryParseToolArguments_FallsBackToBraces(t *testing.T) {
	args := `prefix text {"command":"ls"} suffix`
	_, raw, ok := tryParseToolArguments(args)
	if !ok {
		t.Fatal("expected to extract JSON from braces")
	}
	if raw != `{"command":"ls"}` {
		t.Errorf("unexpected raw: %q", raw)
	}
}

func TestConfigM3FlagDefaults(t *testing.T) {
	m3 := config.ModelConfig{ModelID: "minimax-m3"}
	if !m3.M3StrictPromptEnabled() || !m3.M3TightenSchemasEnabled() || !m3.M3ReasoningSplitEnabled() || !m3.M3ValidateToolCallsEnabled() {
		t.Error("expected all M3 flags to default to true")
	}

	other := config.ModelConfig{ModelID: "minimax-m2.7"}
	if other.M3StrictPromptEnabled() || other.M3TightenSchemasEnabled() || other.M3ReasoningSplitEnabled() || other.M3ValidateToolCallsEnabled() {
		t.Error("expected M3 flags to be false for non-M3 models")
	}
}
