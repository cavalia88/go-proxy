// Package transformer handles conversion between OpenAI and Anthropic API formats.
package transformer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"go-proxy/pkg/types"
)

// AnthropicStreamTransformer converts Anthropic SSE events to OpenAI SSE chunks.
// It reads from an Anthropic streaming response body and writes OpenAI-format SSE events.
type AnthropicStreamTransformer struct {
	model    string
	logger   *slog.Logger
	response *types.MessageResponse // Accumulated response state
}

// NewAnthropicStreamTransformer creates a new stream transformer for the given model.
func NewAnthropicStreamTransformer(model string, logger *slog.Logger) *AnthropicStreamTransformer {
	return &AnthropicStreamTransformer{
		model:  model,
		logger: logger,
		response: &types.MessageResponse{
			Role: "assistant",
		},
	}
}

// TransformStream reads Anthropic SSE events from the reader and writes OpenAI SSE chunks to the writer.
// Returns the final accumulated response for usage tracking.
func (t *AnthropicStreamTransformer) TransformStream(reader io.Reader, writer io.Writer) (*types.MessageResponse, error) {
	scanner := bufio.NewScanner(reader)
	// Increase buffer size for large events
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Skip non-data lines (comments, etc.)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// Check for stream end
		if data == "[DONE]" {
			// Write OpenAI stream end
			_, _ = fmt.Fprintf(writer, "data: [DONE]\n\n")
			return t.response, nil
		}

		// Parse the Anthropic event
		var event struct {
			Type    string                 `json:"type"`
			Index   *int                   `json:"index,omitempty"`
			Message *types.MessageResponse `json:"message,omitempty"`
			Delta   *types.Delta           `json:"delta,omitempty"`
			Usage   *types.Usage           `json:"usage,omitempty"`
			Error   *types.APIError        `json:"error,omitempty"`
		}

		if err := json.Unmarshal([]byte(data), &event); err != nil {
			t.logger.Warn("failed to parse Anthropic SSE event", "error", err, "data", data)
			continue
		}

		// Handle error events
		if event.Error != nil {
			t.logger.Error("Anthropic stream error", "type", event.Error.Type, "message", event.Error.Message)
			// Send error as an OpenAI-format error
			errResp := map[string]interface{}{
				"error": map[string]interface{}{
					"message": event.Error.Message,
					"type":    event.Error.Type,
				},
			}
			errJSON, _ := json.Marshal(errResp)
			_, _ = fmt.Fprintf(writer, "data: %s\n\n", errJSON)
			continue
		}

		// Process event by type
		chunks := t.processEvent(data, &event)
		for _, chunk := range chunks {
			chunkJSON, err := json.Marshal(chunk)
			if err != nil {
				t.logger.Warn("failed to marshal OpenAI chunk", "error", err)
				continue
			}
			_, _ = fmt.Fprintf(writer, "data: %s\n\n", chunkJSON)
		}

		// Flush if writer supports it
		if flusher, ok := writer.(interface{ Flush() }); ok {
			flusher.Flush()
		}
	}

	if err := scanner.Err(); err != nil {
		return t.response, fmt.Errorf("error reading stream: %w", err)
	}

	return t.response, nil
}

// processEvent converts an Anthropic SSE event to one or more OpenAI ChatCompletionChunks.
// It also receives the raw data for re-parsing content_block_start events.
func (t *AnthropicStreamTransformer) processEvent(data string, event *struct {
	Type    string                 `json:"type"`
	Index   *int                   `json:"index,omitempty"`
	Message *types.MessageResponse `json:"message,omitempty"`
	Delta   *types.Delta           `json:"delta,omitempty"`
	Usage   *types.Usage           `json:"usage,omitempty"`
	Error   *types.APIError        `json:"error,omitempty"`
}) []*types.ChatCompletionChunk {
	var chunks []*types.ChatCompletionChunk

	switch event.Type {
	case "message_start":
		// Initialize response state from the message
		if event.Message != nil {
			t.response.ID = event.Message.ID
			t.response.Model = event.Message.Model
			t.response.Role = event.Message.Role
			if event.Message.Usage.InputTokens > 0 {
				t.response.Usage.InputTokens = event.Message.Usage.InputTokens
			}
		}

		// Send initial chunk with role
		chunks = append(chunks, &types.ChatCompletionChunk{
			ID:      t.response.ID,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   t.model,
			Choices: []types.Choice{
				{
					Index: 0,
					Delta: types.ChatMessage{
						Role:    "assistant",
						Content: json.RawMessage(`""`),
					},
				},
			},
		})

	case "content_block_start":
		// content_block_start has a content_block field with the block info
		// We need to parse it to get tool_use ID and name
		var startEvent struct {
			Type         string          `json:"type"`
			Index        int             `json:"index"`
			ContentBlock json.RawMessage `json:"content_block"`
		}
		if err := json.Unmarshal([]byte(data), &startEvent); err == nil && len(startEvent.ContentBlock) > 0 {
			var block struct {
				Type string `json:"type"`
				ID   string `json:"id,omitempty"`
				Name string `json:"name,omitempty"`
				Text string `json:"text,omitempty"`
			}
			if err := json.Unmarshal(startEvent.ContentBlock, &block); err == nil {
				switch block.Type {
				case "tool_use":
					// Tool use block starting - send tool_calls delta with ID and name
					chunks = append(chunks, &types.ChatCompletionChunk{
						ID:      t.response.ID,
						Object:  "chat.completion.chunk",
						Created: time.Now().Unix(),
						Model:   t.model,
						Choices: []types.Choice{
							{
								Index: 0,
								Delta: types.ChatMessage{
									ToolCalls: []types.ToolCall{
										{
											ID:   block.ID,
											Type: "function",
											Function: types.FunctionCall{
												Name:      block.Name,
												Arguments: "",
											},
										},
									},
								},
							},
						},
					})
				case "text":
					// Text block starting - no action needed, will receive text_delta events
				case "thinking":
					// Thinking block starting - no action needed, will receive thinking_delta events
				}
			}
		}

	case "content_block_delta":
		if event.Delta != nil {
			switch event.Delta.Type {
			case "text_delta":
				// Text content delta
				contentJSON, _ := json.Marshal(event.Delta.Text)
				chunks = append(chunks, &types.ChatCompletionChunk{
					ID:      t.response.ID,
					Object:  "chat.completion.chunk",
					Created: time.Now().Unix(),
					Model:   t.model,
					Choices: []types.Choice{
						{
							Index: 0,
							Delta: types.ChatMessage{
								Content: json.RawMessage(contentJSON),
							},
						},
					},
				})
			case "thinking_delta":
				// Thinking content delta
				thinking := event.Delta.Thinking
				chunks = append(chunks, &types.ChatCompletionChunk{
					ID:      t.response.ID,
					Object:  "chat.completion.chunk",
					Created: time.Now().Unix(),
					Model:   t.model,
					Choices: []types.Choice{
						{
							Index: 0,
							Delta: types.ChatMessage{
								ReasoningContent: &thinking,
							},
						},
					},
				})
			case "input_json_delta":
				// Tool call arguments delta
				if event.Index != nil {
					_ = *event.Index // tool call index
					chunks = append(chunks, &types.ChatCompletionChunk{
						ID:      t.response.ID,
						Object:  "chat.completion.chunk",
						Created: time.Now().Unix(),
						Model:   t.model,
						Choices: []types.Choice{
							{
								Index: 0,
								Delta: types.ChatMessage{
									ToolCalls: []types.ToolCall{
										{
											Function: types.FunctionCall{
												Arguments: event.Delta.PartialJSON,
											},
										},
									},
								},
							},
						},
					})
				}
			}
		}

	case "content_block_stop":
		// Content block ended - no action needed for OpenAI format

	case "message_delta":
		// Message-level delta (stop_reason, usage)
		if event.Delta != nil && event.Delta.StopReason != "" {
			finishReason := mapStopReason(event.Delta.StopReason)
			chunks = append(chunks, &types.ChatCompletionChunk{
				ID:      t.response.ID,
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   t.model,
				Choices: []types.Choice{
					{
						Index:        0,
						FinishReason: finishReason,
					},
				},
			})
		}

		// Update usage if provided
		if event.Usage != nil {
			t.response.Usage.OutputTokens = event.Usage.OutputTokens
		}

	case "message_stop":
		// Stream is done - we already send [DONE] in the main loop

	default:
		// Unknown event type - log and skip
		t.logger.Debug("unknown Anthropic event type", "type", event.Type)
	}

	return chunks
}
