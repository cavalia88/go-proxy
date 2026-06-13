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

	"go-proxy/internal/config"
	"go-proxy/pkg/types"
)

// AnthropicStreamTransformer converts Anthropic SSE events to OpenAI SSE chunks.
// It reads from an Anthropic streaming response body and writes OpenAI-format SSE events.
type AnthropicStreamTransformer struct {
	model         string
	logger        *slog.Logger
	response      *types.MessageResponse // Accumulated response state
	originalTools []types.ToolDef
	modelConfig   config.ModelConfig
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

// SetOriginalTools stores the original OpenAI tool definitions for M3 validation.
func (t *AnthropicStreamTransformer) SetOriginalTools(tools []types.ToolDef) {
	t.originalTools = tools
}

// SetModelConfig stores the model config so M3-specific flags can be read.
func (t *AnthropicStreamTransformer) SetModelConfig(cfg config.ModelConfig) {
	t.modelConfig = cfg
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
		// Note: upstream may send either "data: {...}" or "data:{...}" (no space)
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

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

	// MiniMax M3: sanitize accumulated stream state and emit corrective chunks
	// before closing the stream. No-op for other models.
	t.emitM3StreamCorrections(writer)

	// Send [DONE] signal so downstream OpenAI-compatible clients know the stream is complete.
	// Anthropic streams end with "message_stop", not "[DONE]", but OpenAI clients require it.
	_, _ = fmt.Fprintf(writer, "data: [DONE]\n\n")
	if flusher, ok := writer.(interface{ Flush() }); ok {
		flusher.Flush()
	}
	t.logger.Info("stream complete", "sent", "[DONE]")

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
											Index: startEvent.Index,
											ID:    block.ID,
											Type:  "function",
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
			case "signature_delta":
				// Signature delta — no-op for OpenAI format. The signature is empty/non-actionable.

			case "input_json_delta":
				// Tool call arguments delta
				if event.Index != nil {
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
											Index: *event.Index,
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
		// Stream is done — the main loop sends [DONE] when the connection closes (EOF).

	case "ping":
		// Keep-alive ping from the upstream — silently ignore.

	default:
		// Unknown event type - log and skip
		t.logger.Debug("unknown Anthropic event type", "type", event.Type)
	}

	return chunks
}

// emitM3StreamCorrections sanitizes the accumulated MiniMax M3 stream response
// and emits corrective SSE chunks before the stream closes. It is a no-op for
// non-M3 models or when tool-call validation is disabled.
func (t *AnthropicStreamTransformer) emitM3StreamCorrections(writer io.Writer) {
	if !t.modelConfig.IsMiniMaxM3() || !t.modelConfig.M3ValidateToolCallsEnabled() {
		return
	}

	// Capture original inputs so we can detect repairs.
	originalInputs := make(map[string]string)
	for _, block := range t.response.Content {
		if block.Type == "tool_use" {
			originalInputs[block.ID] = string(block.Input)
		}
	}

	SanitizeM3StreamResponse(t.response, t.originalTools, t.modelConfig)

	var toolIndex int
	var corrections []types.ToolCall
	var warningText string

	for _, block := range t.response.Content {
		switch block.Type {
		case "tool_use":
			if orig, ok := originalInputs[block.ID]; ok && orig != string(block.Input) {
				corrections = append(corrections, types.ToolCall{
					Index: toolIndex,
					ID:    block.ID,
					Type:  "function",
					Function: types.FunctionCall{
						Name:      block.Name,
						Arguments: string(block.Input),
					},
				})
			}
			toolIndex++
		case "text":
			if strings.Contains(block.Text, "invalid and removed") && warningText == "" {
				warningText = strings.TrimSpace(block.Text)
			}
		}
	}

	if len(corrections) == 0 && warningText == "" {
		return
	}

	delta := types.ChatMessage{}
	if len(corrections) > 0 {
		delta.ToolCalls = corrections
	}
	if warningText != "" {
		delta.Content = marshalContent(warningText)
	}

	chunk := &types.ChatCompletionChunk{
		ID:      t.response.ID,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   t.model,
		Choices: []types.Choice{
			{
				Index: 0,
				Delta: delta,
			},
		},
	}

	chunkJSON, err := json.Marshal(chunk)
	if err != nil {
		t.logger.Warn("failed to marshal M3 stream correction chunk", "error", err)
		return
	}
	_, _ = fmt.Fprintf(writer, "data: %s\n\n", chunkJSON)
	if flusher, ok := writer.(interface{ Flush() }); ok {
		flusher.Flush()
	}
	t.logger.Debug("emitted M3 stream correction chunk", "tool_corrections", len(corrections), "warning", warningText != "")
}
