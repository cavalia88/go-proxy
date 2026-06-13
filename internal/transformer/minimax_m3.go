// Package transformer handles MiniMax M3 specific request/response adjustments.
package transformer

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"go-proxy/internal/config"
	"go-proxy/pkg/types"
)

const m3StrictPromptSuffix = `
CRITICAL: When calling a tool, you must strictly populate the argument parameters exactly matching the provided JSON schema. Do not include any conversational filler, markdown formatting inside the arguments, or unprompted keys. If a parameter is required, you must provide a valid value for it. Only use tool names that are explicitly listed in the available tools.`

// IsMiniMaxM3 returns true when the upstream model ID is MiniMax M3.
func IsMiniMaxM3(modelID string) bool {
	return modelID == "minimax-m3"
}

// ApplyM3RequestHardening applies MiniMax M3-specific request hardening.
// It mutates req in place. originalTools is the OpenAI tool list from the
// client request and is used when tightening schemas.
func ApplyM3RequestHardening(req *types.MessageRequest, originalTools []types.ToolDef, cfg config.ModelConfig) error {
	if !cfg.IsMiniMaxM3() {
		return nil
	}

	if cfg.M3StrictPromptEnabled() {
		injectM3StrictPrompt(req)
	}

	if cfg.M3TightenSchemasEnabled() {
		req.Tools = tightenM3ToolSchemas(req.Tools)
	}

	return nil
}

// BuildM3RequestBody builds the final Anthropic request body, optionally adding
// MiniMax-specific top-level fields such as reasoning_split.
func BuildM3RequestBody(req *types.MessageRequest, cfg config.ModelConfig) ([]byte, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	if !cfg.IsMiniMaxM3() || !cfg.M3ReasoningSplitEnabled() {
		return body, nil
	}

	// Merge reasoning_split into the top-level JSON object.
	var obj map[string]interface{}
	if err := json.Unmarshal(body, &obj); err != nil {
		return body, nil // fall back to original body
	}
	obj["reasoning_split"] = true
	return json.Marshal(obj)
}

// SanitizeM3Response cleans up malformed tool calls in a non-streaming response.
// It mutates resp in place. originalTools is the OpenAI tool list from the client
// request.
func SanitizeM3Response(resp *types.ChatCompletionResponse, originalTools []types.ToolDef, cfg config.ModelConfig) {
	if !cfg.IsMiniMaxM3() || !cfg.M3ValidateToolCallsEnabled() {
		return
	}
	if resp == nil || len(resp.Choices) == 0 {
		return
	}
	msg := &resp.Choices[0].Message
	sanitizeM3ToolCalls(msg, originalTools)
}

// SanitizeM3StreamResponse cleans up malformed tool calls in the accumulated
// stream response. It mutates resp in place.
func SanitizeM3StreamResponse(resp *types.MessageResponse, originalTools []types.ToolDef, cfg config.ModelConfig) {
	if !cfg.IsMiniMaxM3() || !cfg.M3ValidateToolCallsEnabled() {
		return
	}
	if resp == nil {
		return
	}

	validNames := buildToolNameSet(originalTools)
	var sanitized []types.ContentBlock
	var dropped []string

	for i := range resp.Content {
		block := &resp.Content[i]
		if block.Type != "tool_use" {
			sanitized = append(sanitized, *block)
			continue
		}

		if _, ok := validNames[block.Name]; !ok {
			dropped = append(dropped, fmt.Sprintf("invalid tool name %q", block.Name))
			continue
		}

		toolDef := findToolDef(originalTools, block.Name)
		repaired, replacement := repairToolUseInput(block, toolDef)
		if repaired {
			sanitized = append(sanitized, *block)
		} else {
			dropped = append(dropped, replacement)
		}
	}

	if len(dropped) > 0 {
		warning := "The following tool calls were invalid and removed: " + strings.Join(dropped, "; ")
		// Append the warning as a text block so the assistant response is still useful.
		sanitized = append(sanitized, types.ContentBlock{Type: "text", Text: "\n" + warning + "\n"})
	}

	resp.Content = sanitized
}

// injectM3StrictPrompt appends the strict tool-calling instruction to the
// system prompt, creating a system prompt if none exists.
func injectM3StrictPrompt(req *types.MessageRequest) {
	current := req.SystemText()
	suffix := strings.TrimSpace(m3StrictPromptSuffix)

	// Preserve array format if the original system was an array.
	if len(req.System) > 0 && req.System[0] == '[' {
		var blocks []types.SystemContentBlock
		if err := json.Unmarshal(req.System, &blocks); err == nil {
			// Append the suffix to the last text block, or add a new text block.
			appended := false
			for i := len(blocks) - 1; i >= 0; i-- {
				if blocks[i].Type == "text" {
					if blocks[i].Text != "" {
						blocks[i].Text += "\n\n"
					}
					blocks[i].Text += suffix
					appended = true
					break
				}
			}
			if !appended {
				blocks = append(blocks, types.SystemContentBlock{Type: "text", Text: suffix})
			}
			req.System, _ = json.Marshal(blocks)
			return
		}
	}

	if current != "" {
		current += "\n\n"
	}
	current += suffix
	req.System, _ = json.Marshal(current)
}

// tightenM3ToolSchemas makes tool input schemas stricter for MiniMax M3.
// It adds empty properties and additionalProperties: false to object schemas
// that do not already declare properties, which helps M3 avoid emitting {} or
// omitting required fields.
func tightenM3ToolSchemas(tools []types.Tool) []types.Tool {
	out := make([]types.Tool, len(tools))
	for i, tool := range tools {
		out[i] = tool
		if len(tool.InputSchema) == 0 {
			continue
		}
		tightened, err := tightenSchema(tool.InputSchema)
		if err == nil {
			out[i].InputSchema = tightened
		}
	}
	return out
}

func tightenSchema(schema json.RawMessage) (json.RawMessage, error) {
	var node map[string]interface{}
	if err := json.Unmarshal(schema, &node); err != nil {
		return schema, err
	}
	tightenNode(node)
	return json.Marshal(node)
}

func tightenNode(node map[string]interface{}) {
	typ, _ := node["type"].(string)
	if typ == "object" {
		if _, hasProps := node["properties"]; !hasProps {
			node["properties"] = map[string]interface{}{}
		}
		if _, hasAdditional := node["additionalProperties"]; !hasAdditional {
			node["additionalProperties"] = false
		}
	}

	if props, ok := node["properties"].(map[string]interface{}); ok {
		for _, v := range props {
			if child, ok := v.(map[string]interface{}); ok {
				tightenNode(child)
			}
		}
	}

	if items, ok := node["items"].(map[string]interface{}); ok {
		tightenNode(items)
	}
}

func buildToolNameSet(tools []types.ToolDef) map[string]struct{} {
	set := make(map[string]struct{}, len(tools))
	for _, t := range tools {
		set[t.Function.Name] = struct{}{}
	}
	return set
}

func findToolDef(tools []types.ToolDef, name string) *types.ToolDef {
	for i := range tools {
		if tools[i].Function.Name == name {
			return &tools[i]
		}
	}
	return nil
}

// sanitizeM3ToolCalls removes or repairs malformed tool calls in an OpenAI-format
// message. It mutates msg in place.
func sanitizeM3ToolCalls(msg *types.ChatMessage, tools []types.ToolDef) {
	if len(msg.ToolCalls) == 0 {
		return
	}

	validNames := buildToolNameSet(tools)
	var kept []types.ToolCall
	var dropped []string

	for i := range msg.ToolCalls {
		tc := &msg.ToolCalls[i]
		if _, ok := validNames[tc.Function.Name]; !ok {
			dropped = append(dropped, fmt.Sprintf("invalid tool name %q", tc.Function.Name))
			continue
		}

		toolDef := findToolDef(tools, tc.Function.Name)
		repaired, replacement := repairToolCall(tc, toolDef)
		if repaired {
			kept = append(kept, *tc)
		} else {
			dropped = append(dropped, replacement)
		}
	}

	if len(dropped) > 0 {
		warning := "The following tool calls were invalid and removed: " + strings.Join(dropped, "; ")
		contentText := msg.ContentText()
		contentText += "\n" + warning + "\n"
		msg.Content = marshalContent(contentText)
	}

	msg.ToolCalls = kept
}

// repairToolCall attempts to repair an OpenAI-format tool call. It returns
// (true, "") if the call is valid or was repaired, otherwise (false, reason).
func repairToolCall(tc *types.ToolCall, toolDef *types.ToolDef) (bool, string) {
	args := strings.TrimSpace(tc.Function.Arguments)
	if args == "" {
		args = "{}"
	}

	// Try to parse as JSON. If it fails, attempt to extract JSON from markdown
	// fences or trailing text.
	parsed, raw, ok := tryParseToolArguments(args)
	if !ok {
		return false, fmt.Sprintf("could not parse arguments for tool %q", tc.Function.Name)
	}

	if toolDef == nil || len(toolDef.Function.Parameters) == 0 {
		tc.Function.Arguments = raw
		return true, ""
	}

	required := extractRequired(toolDef.Function.Parameters)
	schemaTypes := extractPropertyTypes(toolDef.Function.Parameters)
	missing := findMissingRequired(parsed, required)
	if len(missing) == 0 {
		tc.Function.Arguments = raw
		return true, ""
	}

	// Fill missing required fields with safe defaults.
	for _, key := range missing {
		if parsed[key] == nil {
			parsed[key] = defaultValueForType(schemaTypes[key])
		}
	}

	fixed, err := json.Marshal(parsed)
	if err != nil {
		return false, fmt.Sprintf("failed to marshal repaired arguments for tool %q", tc.Function.Name)
	}
	tc.Function.Arguments = string(fixed)
	return true, ""
}

// repairToolUseInput attempts to repair an Anthropic-format tool_use input block.
func repairToolUseInput(block *types.ContentBlock, toolDef *types.ToolDef) (bool, string) {
	args := strings.TrimSpace(string(block.Input))
	if args == "" {
		args = "{}"
	}

	parsed, raw, ok := tryParseToolArguments(args)
	if !ok {
		return false, fmt.Sprintf("could not parse input for tool %q", block.Name)
	}

	if toolDef == nil || len(toolDef.Function.Parameters) == 0 {
		block.Input = json.RawMessage(raw)
		return true, ""
	}

	required := extractRequired(toolDef.Function.Parameters)
	schemaTypes := extractPropertyTypes(toolDef.Function.Parameters)
	missing := findMissingRequired(parsed, required)
	if len(missing) == 0 {
		block.Input = json.RawMessage(raw)
		return true, ""
	}

	for _, key := range missing {
		if parsed[key] == nil {
			parsed[key] = defaultValueForType(schemaTypes[key])
		}
	}

	fixed, err := json.Marshal(parsed)
	if err != nil {
		return false, fmt.Sprintf("failed to marshal repaired input for tool %q", block.Name)
	}
	block.Input = json.RawMessage(fixed)
	return true, ""
}

func tryParseToolArguments(args string) (map[string]interface{}, string, bool) {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(args), &parsed); err == nil {
		return parsed, args, true
	}

	// Attempt to extract JSON from markdown fences.
	re := regexp.MustCompile("(?s)```(?:json)?\\s*([\\s\\S]*?)```")
	matches := re.FindAllStringSubmatch(args, -1)
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		candidate := strings.TrimSpace(m[1])
		if err := json.Unmarshal([]byte(candidate), &parsed); err == nil {
			return parsed, candidate, true
		}
	}

	// Attempt to find the first { ... } substring that parses as JSON.
	start := strings.Index(args, "{")
	end := strings.LastIndex(args, "}")
	if start != -1 && end != -1 && end > start {
		candidate := args[start : end+1]
		if err := json.Unmarshal([]byte(candidate), &parsed); err == nil {
			return parsed, candidate, true
		}
	}

	return nil, "", false
}

func extractRequired(schema json.RawMessage) []string {
	var node struct {
		Required []string `json:"required"`
	}
	_ = json.Unmarshal(schema, &node)
	return node.Required
}

func extractPropertyTypes(schema json.RawMessage) map[string]string {
	types := make(map[string]string)
	var node struct {
		Properties map[string]struct {
			Type  string `json:"type"`
			Types []struct {
				Type string `json:"type"`
			} `json:"anyOf"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema, &node); err != nil {
		return types
	}
	for name, prop := range node.Properties {
		if prop.Type != "" {
			types[name] = prop.Type
			continue
		}
		if len(prop.Types) > 0 && prop.Types[0].Type != "" {
			types[name] = prop.Types[0].Type
		}
	}
	return types
}

func findMissingRequired(args map[string]interface{}, required []string) []string {
	var missing []string
	for _, key := range required {
		if v, ok := args[key]; !ok || v == nil {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	return missing
}

func defaultValueForType(typ string) interface{} {
	switch typ {
	case "string":
		return ""
	case "integer", "number":
		return 0
	case "boolean":
		return false
	case "array":
		return []interface{}{}
	case "object":
		return map[string]interface{}{}
	default:
		return ""
	}
}
