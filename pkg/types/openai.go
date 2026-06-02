// Package types defines shared data structures and interfaces.
package types

import "encoding/json"

// OpenAI API types for the Chat Completions API.
// Reference: https://platform.openai.com/docs/api-reference/chat

// ChatCompletionRequest represents a request to the OpenAI Chat Completions API.
type ChatCompletionRequest struct {
	Model           string          `json:"model"`
	Messages        []ChatMessage   `json:"messages"`
	Stream          *bool           `json:"stream,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	TopP            *float64        `json:"top_p,omitempty"`
	MaxTokens       *int            `json:"max_tokens,omitempty"`
	ReasoningEffort *string         `json:"reasoning_effort,omitempty"`
	Thinking        json.RawMessage `json:"thinking,omitempty"`
	Tools           []ToolDef       `json:"tools,omitempty"`
	ToolChoice      interface{}     `json:"tool_choice,omitempty"`
	Stop            interface{}     `json:"stop,omitempty"`
}

// ChatMessage represents a single message in the conversation.
// Content can be either a string or an array of content parts:
//   - string: "Hello"
//   - array: [{"type": "text", "text": "Hello"}]
// We use json.RawMessage to accept both formats and forward them as-is.
type ChatMessage struct {
	Role             string          `json:"role,omitempty"`
	Content          json.RawMessage `json:"content"`
	ReasoningContent *string         `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall      `json:"tool_calls,omitempty"`
	Name             string          `json:"name,omitempty"`
	ToolCallID       string          `json:"tool_call_id,omitempty"`
	CacheControl     *CacheControl   `json:"cache_control,omitempty"`
}

// ContentText extracts the text content from a ChatMessage.
// If Content is a plain string, it returns the string.
// If Content is an array of content parts, it concatenates all text parts.
// If Content is null or empty, it returns an empty string.
func (m *ChatMessage) ContentText() string {
	if len(m.Content) == 0 {
		return ""
	}

	// Try string format first: "Hello"
	var str string
	if err := json.Unmarshal(m.Content, &str); err == nil {
		return str
	}

	// Try array format: [{"type": "text", "text": "Hello"}]
	var parts []ContentPart
	if err := json.Unmarshal(m.Content, &parts); err == nil {
		var result string
		for _, part := range parts {
			if part.Type == "text" {
				result += part.Text
			}
		}
		return result
	}

	// Fallback: return raw content as string
	return string(m.Content)
}

// ContentPart represents a single content part in an array-format message.
type ContentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ToolCall represents a function call made by the model.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall represents the function invocation details.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolDef represents a tool definition for function calling.
type ToolDef struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

// FunctionDef represents the function definition schema.
type FunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ChatCompletionResponse represents a response from the OpenAI Chat Completions API.
type ChatCompletionResponse struct {
	ID      string    `json:"id"`
	Object  string    `json:"object"`
	Created int64     `json:"created"`
	Model   string    `json:"model"`
	Choices []Choice  `json:"choices"`
	Usage   UsageInfo `json:"usage"`
}

// Choice represents a single choice in the response.
type Choice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message,omitempty"`
	FinishReason string      `json:"finish_reason,omitempty"`
	Delta        ChatMessage `json:"delta,omitempty"`
}

// UsageInfo represents token usage information.
type UsageInfo struct {
	PromptTokens          int `json:"prompt_tokens"`
	CompletionTokens      int `json:"completion_tokens"`
	TotalTokens           int `json:"total_tokens"`
	PromptCacheHitTokens  int `json:"prompt_cache_hit_tokens,omitempty"`
	PromptCacheMissTokens int `json:"prompt_cache_miss_tokens,omitempty"`
}

// ChatCompletionChunk represents a streaming chunk from the Chat Completions API.
type ChatCompletionChunk struct {
	ID      string     `json:"id"`
	Object  string     `json:"object"`
	Created int64      `json:"created"`
	Model   string     `json:"model"`
	Choices []Choice   `json:"choices"`
	Usage   *UsageInfo `json:"usage,omitempty"`
}

// ErrorResponse represents an error response from the OpenAI API.
type ErrorResponse struct {
	Error ErrorDetails `json:"error"`
}

// ErrorDetails contains the details of an API error.
type ErrorDetails struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}
