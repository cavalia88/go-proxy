// Package transformer handles conversion between OpenAI and Anthropic API formats.
package transformer

import (
	"encoding/json"
	"fmt"
	"time"

	"go-proxy/pkg/types"
)

// TransformAnthropicToOpenAI converts an Anthropic MessageResponse to an OpenAI ChatCompletionResponse.
//
// Mapping:
//   - content[].type="text" → choices[0].message.content
//   - content[].type="tool_use" → choices[0].message.tool_calls
//   - content[].type="thinking" → choices[0].message.reasoning_content
//   - stop_reason="end_turn" → finish_reason="stop"
//   - stop_reason="tool_use" → finish_reason="tool_calls"
//   - usage.input_tokens → usage.prompt_tokens
//   - usage.output_tokens → usage.completion_tokens
func TransformAnthropicToOpenAI(resp *types.MessageResponse, model string) (*types.ChatCompletionResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("response is nil")
	}

	// Build the message content and tool calls from content blocks
	var content string
	var toolCalls []types.ToolCall
	var reasoningContent *string

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			content += block.Text
		case "tool_use":
			toolCalls = append(toolCalls, types.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: types.FunctionCall{
					Name:      block.Name,
					Arguments: string(block.Input),
				},
			})
		case "thinking":
			rc := block.Thinking
			reasoningContent = &rc
		}
	}

	// Map stop_reason to finish_reason
	finishReason := mapStopReason(resp.StopReason)

	// Calculate token counts
	promptTokens := resp.Usage.InputTokens
	completionTokens := resp.Usage.OutputTokens
	totalTokens := promptTokens + completionTokens

	return &types.ChatCompletionResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []types.Choice{
			{
				Index: 0,
				Message: types.ChatMessage{
					Role:             "assistant",
					Content:          marshalContent(content),
					ReasoningContent: reasoningContent,
					ToolCalls:        toolCalls,
				},
				FinishReason: finishReason,
			},
		},
		Usage: types.UsageInfo{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      totalTokens,
		},
	}, nil
}

// mapStopReason converts an Anthropic stop_reason to an OpenAI finish_reason.
func mapStopReason(stopReason string) string {
	switch stopReason {
	case "end_turn":
		return "stop"
	case "tool_use":
		return "tool_calls"
	case "max_tokens":
		return "length"
	case "stop_sequence":
		return "stop"
	default:
		return "stop"
	}
}

// marshalContent returns json.RawMessage for a string content value.
func marshalContent(content string) json.RawMessage {
	// Return the string as a JSON string
	b, _ := json.Marshal(content)
	return json.RawMessage(b)
}
