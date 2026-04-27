// Package transformer handles conversion between OpenAI and Anthropic API formats.
package transformer

import (
	"encoding/json"
	"fmt"

	"go-proxy/pkg/types"
)

// TransformOpenAIToAnthropic converts an OpenAI ChatCompletionRequest to an Anthropic MessageRequest.
// This is the "reverse" transform: OpenAI format → Anthropic format.
//
// Mapping:
//   - messages[role="system"] → system field
//   - messages[].tool_calls → tool_use content blocks
//   - messages[].tool_call_id + content → tool_result content blocks
//   - messages[].reasoning_content → thinking content blocks
//   - tools[].function → tools[].input_schema
//   - max_tokens → max_tokens
//   - temperature → temperature
//   - stream → stream
func TransformOpenAIToAnthropic(req *types.ChatCompletionRequest) (*types.MessageRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}

	anthropicReq := &types.MessageRequest{
		Model: req.Model,
	}

	// Extract max_tokens (required by Anthropic)
	if req.MaxTokens != nil {
		anthropicReq.MaxTokens = *req.MaxTokens
	} else {
		// Anthropic requires max_tokens, default to 4096
		anthropicReq.MaxTokens = 4096
	}

	// Copy optional fields
	anthropicReq.Stream = req.Stream
	anthropicReq.Temperature = req.Temperature
	anthropicReq.TopP = req.TopP

	// Convert tools
	if len(req.Tools) > 0 {
		anthropicReq.Tools = convertTools(req.Tools)
	}

	// Convert messages
	var systemPrompt json.RawMessage
	var messages []types.Message

	for _, msg := range req.Messages {
		switch msg.Role {
		case "system":
			// System messages go to the top-level system field
			systemPrompt = msg.Content
		case "assistant":
			// Assistant messages may contain tool_calls or reasoning_content
			content, err := convertAssistantMessage(&msg)
			if err != nil {
				return nil, fmt.Errorf("converting assistant message: %w", err)
			}
			messages = append(messages, types.Message{
				Role:    "assistant",
				Content: content,
			})
		case "tool":
			// Tool result messages → tool_result content blocks in a user message
			content, err := convertToolResultMessage(&msg)
			if err != nil {
				return nil, fmt.Errorf("converting tool result message: %w", err)
			}
			messages = append(messages, types.Message{
				Role:    "user",
				Content: content,
			})
		default:
			// user or other roles
			content, err := convertUserMessage(&msg)
			if err != nil {
				return nil, fmt.Errorf("converting user message: %w", err)
			}
			messages = append(messages, types.Message{
				Role:    msg.Role,
				Content: content,
			})
		}
	}

	anthropicReq.System = systemPrompt
	anthropicReq.Messages = messages

	return anthropicReq, nil
}

// convertTools converts OpenAI tool definitions to Anthropic tool definitions.
func convertTools(tools []types.ToolDef) []types.Tool {
	result := make([]types.Tool, 0, len(tools))
	for _, tool := range tools {
		result = append(result, types.Tool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: tool.Function.Parameters,
		})
	}
	return result
}

// convertAssistantMessage converts an OpenAI assistant message to Anthropic content blocks.
func convertAssistantMessage(msg *types.ChatMessage) (json.RawMessage, error) {
	var blocks []types.ContentBlock

	// Add text content if present
	if len(msg.Content) > 0 {
		text := msg.ContentText()
		if text != "" {
			blocks = append(blocks, types.ContentBlock{
				Type: "text",
				Text: text,
			})
		}
	}

	// Add reasoning/thinking content if present
	if msg.ReasoningContent != nil && *msg.ReasoningContent != "" {
		blocks = append(blocks, types.ContentBlock{
			Type:     "thinking",
			Thinking: *msg.ReasoningContent,
		})
	}

	// Add tool calls if present
	for _, tc := range msg.ToolCalls {
		blocks = append(blocks, types.ContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: json.RawMessage(tc.Function.Arguments),
		})
	}

	// If no blocks were created, create a text block with empty content
	if len(blocks) == 0 {
		blocks = append(blocks, types.ContentBlock{
			Type: "text",
			Text: "",
		})
	}

	return json.Marshal(blocks)
}

// convertToolResultMessage converts an OpenAI tool result message to Anthropic tool_result content blocks.
func convertToolResultMessage(msg *types.ChatMessage) (json.RawMessage, error) {
	// The content of a tool result message becomes a tool_result content block.
	// Anthropic's tool_result content field can be either a string or an array of content blocks.
	// We need to handle both cases: if the original content is already a JSON array, pass it through;
	// otherwise, wrap the text in a text content block array.

	var contentValue json.RawMessage

	if len(msg.Content) > 0 && msg.Content[0] == '[' {
		// Content is already an array of content blocks — pass through as-is
		contentValue = msg.Content
	} else {
		// Content is a string — create a text content block array
		textBlock := types.ContentBlock{
			Type: "text",
			Text: msg.ContentText(),
		}
		contentJSON, err := json.Marshal([]types.ContentBlock{textBlock})
		if err != nil {
			return nil, fmt.Errorf("marshaling tool result content: %w", err)
		}
		contentValue = contentJSON
	}

	isError := false // tool results are not errors by default

	block := types.ContentBlock{
		Type:      "tool_result",
		ToolUseID: msg.ToolCallID,
		Content:   contentValue,
		IsError:   &isError,
	}

	blocks := []types.ContentBlock{block}
	return json.Marshal(blocks)
}

// convertUserMessage converts a user message to Anthropic content blocks.
func convertUserMessage(msg *types.ChatMessage) (json.RawMessage, error) {
	// If content is already an array of content blocks, pass through
	if len(msg.Content) > 0 && msg.Content[0] == '[' {
		// Try to parse as array of content blocks
		var blocks []types.ContentBlock
		if err := json.Unmarshal(msg.Content, &blocks); err == nil {
			return msg.Content, nil
		}
	}

	// Otherwise, treat as text content
	text := msg.ContentText()
	blocks := []types.ContentBlock{
		{
			Type: "text",
			Text: text,
		},
	}
	return json.Marshal(blocks)
}
