// Package handlers contains HTTP request handlers for API endpoints.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"go-proxy/internal/client"
	"go-proxy/internal/config"
	"go-proxy/internal/debug"
	"go-proxy/internal/middleware"
	"go-proxy/internal/token"
	"go-proxy/internal/transformer"
	"go-proxy/pkg/types"
)

// extendedContentPart extends ContentPart to handle the "summary" field
// that Kilocode includes in reasoning content blocks.
type extendedContentPart struct {
	Type    string        `json:"type"`
	Text    string        `json:"text"`
	Summary []interface{} `json:"summary"`
}

// CompletionsHandler handles /v1/chat/completions requests.
type CompletionsHandler struct {
	config       *config.Config
	client       *client.OpenCodeClient
	tokenCounter *token.Counter
	logger       *slog.Logger
}

// NewCompletionsHandler creates a new completions handler.
func NewCompletionsHandler(
	cfg *config.Config,
	openCodeClient *client.OpenCodeClient,
	tokenCounter *token.Counter,
) *CompletionsHandler {
	return &CompletionsHandler{
		config:       cfg,
		client:       openCodeClient,
		tokenCounter: tokenCounter,
		logger:       slog.Default(),
	}
}

// HandleCompletions handles POST /v1/chat/completions.
func (h *CompletionsHandler) HandleCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get API key from context (set by auth middleware)
	apiKey := ""
	if key, ok := r.Context().Value(middleware.APIKeyContextKey).(string); ok {
		apiKey = key
	}

	// Create client with the extracted API key for this request
	openCodeClient := client.NewOpenCodeClient(h.config.OpenCodeGo, apiKey)

	// Parse the request
	var openaiReq types.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&openaiReq); err != nil {
		h.sendError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	// Validate model
	modelID := openaiReq.Model
	if modelID == "" {
		h.sendError(w, http.StatusBadRequest, "model is required", nil)
		return
	}

	// Get model config
	modelConfig, ok := h.config.Models[modelID]
	if !ok {
		h.sendError(w, http.StatusNotFound, fmt.Sprintf("model '%s' not configured", modelID), nil)
		return
	}

	// Count tokens for logging/metrics
	tokenCount := 0
	if h.tokenCounter != nil {
		var messages []token.MessageContent
		for _, msg := range openaiReq.Messages {
			messages = append(messages, token.MessageContent{
				Role:    msg.Role,
				Content: msg.ContentText(),
			})
		}
		if count, err := h.tokenCounter.CountMessages("", messages); err == nil {
			tokenCount = count
		}
	}

	h.logger.Info("processing request",
		"model", modelID,
		"endpoint", modelConfig.Endpoint,
		"streaming", openaiReq.Stream != nil && *openaiReq.Stream,
		"messages", len(openaiReq.Messages),
		"tokens", tokenCount,
	)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	// Route based on model endpoint type
	switch modelConfig.Endpoint {
	case "anthropic":
		h.handleAnthropicRequest(w, r, ctx, openCodeClient, &openaiReq, modelConfig)
	case "openai":
		fallthrough
	default:
		h.handleOpenAIRequest(w, r, ctx, openCodeClient, &openaiReq, modelConfig)
	}
}

// handleOpenAIRequest handles requests for OpenAI-compatible models (passthrough).
func (h *CompletionsHandler) handleOpenAIRequest(
	w http.ResponseWriter,
	r *http.Request,
	ctx context.Context,
	openCodeClient *client.OpenCodeClient,
	openaiReq *types.ChatCompletionRequest,
	modelConfig config.ModelConfig,
) {
	// Apply model config overrides
	h.applyModelConfig(openaiReq, modelConfig)

	// Determine if thinking mode is enabled for this request.
	// When thinking mode is active, DeepSeek requires reasoning_content on ALL
	// assistant messages — not just those with tool_calls.
	thinkingEnabled := h.isThinkingEnabled(modelConfig.Thinking)

	// Fix: Some providers (e.g. Kimi/Moonshot, DeepSeek) require reasoning_content to be
	// passed back on assistant messages when thinking mode is active. Kilocode sends
	// reasoning as content blocks ({"type":"reasoning","text":"..."}) inside the content
	// array, but providers expect it as a separate reasoning_content field.
	//
	// For assistant messages that have reasoning content blocks, we extract the reasoning
	// text and set reasoning_content (DeepSeek requirement).
	//
	// For assistant messages that have tool_calls but no reasoning_content, we add a
	// placeholder (Kimi/Moonshot requirement).
	//
	// When thinking mode is enabled, ALL assistant messages must have reasoning_content
	// (DeepSeek requirement). If none was found or extracted, we add a placeholder.
	for i := range openaiReq.Messages {
		msg := &openaiReq.Messages[i]
		if msg.Role != "assistant" {
			continue
		}

		// Check if the content array has reasoning blocks and extract text
		var reasoningText string
		if len(msg.Content) > 0 && msg.Content[0] == '[' {
			var parts []extendedContentPart
			if err := json.Unmarshal(msg.Content, &parts); err == nil {
				for _, part := range parts {
					if part.Type == "reasoning" && part.Text != "" {
						reasoningText = part.Text
						break
					}
				}
			} else {
				h.logger.Debug("failed to unmarshal assistant content as array",
					"index", i, "error", err, "content_len", len(msg.Content))
			}
		}

		if reasoningText != "" && (msg.ReasoningContent == nil || *msg.ReasoningContent == "") {
			// Extract reasoning from content blocks → set as reasoning_content
			msg.ReasoningContent = &reasoningText
			h.logger.Info("extracted reasoning_content from content blocks",
				"index", i, "reasoning_len", len(reasoningText))
		} else if msg.ReasoningContent == nil || *msg.ReasoningContent == "" {
			// No reasoning_content found — add placeholder if:
			// 1. Message has tool_calls (Kimi/Moonshot requirement), OR
			// 2. Thinking mode is enabled (DeepSeek requirement: ALL assistant messages
			//    must have reasoning_content when thinking is on)
			if len(msg.ToolCalls) > 0 || thinkingEnabled {
				placeholder := " "
				msg.ReasoningContent = &placeholder
				h.logger.Debug("added placeholder reasoning_content to assistant message",
					"index", i, "tool_calls", len(msg.ToolCalls), "thinking_enabled", thinkingEnabled)
			} else {
				h.logger.Debug("assistant message has no reasoning_content, skipping placeholder",
					"index", i, "tool_calls", len(msg.ToolCalls), "thinking_enabled", thinkingEnabled)
			}
		}
	}

	// For streaming requests, we need to set appropriate headers early
	isStreaming := openaiReq.Stream != nil && *openaiReq.Stream
	if isStreaming {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
	}

	// Send request to OpenCode Go
	resp, err := openCodeClient.ChatCompletion(ctx, modelConfig.ModelID, openaiReq)
	if err != nil {
		h.sendError(w, http.StatusBadGateway, "upstream request failed", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// Handle streaming response
	if isStreaming {
		// Stream response directly to client
		_, err := io.Copy(w, resp.Body)
		if err != nil {
			h.logger.Warn("failed to stream response", "error", err)
		}
		return
	}

	// Handle non-streaming response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		h.logger.Warn("failed to copy response", "error", err)
	}
}

// handleAnthropicRequest handles requests for Anthropic-format models (MiniMax M2.5, M2.7).
// It transforms OpenAI requests to Anthropic format, sends them, and transforms responses back.
func (h *CompletionsHandler) handleAnthropicRequest(
	w http.ResponseWriter,
	r *http.Request,
	ctx context.Context,
	openCodeClient *client.OpenCodeClient,
	openaiReq *types.ChatCompletionRequest,
	modelConfig config.ModelConfig,
) {
	// Step 1: Transform OpenAI request → Anthropic request
	anthropicReq, err := transformer.TransformOpenAIToAnthropic(openaiReq)
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, "failed to transform request", err)
		return
	}

	// Apply model config overrides to Anthropic request
	h.applyAnthropicModelConfig(anthropicReq, modelConfig)

	// Apply MiniMax M3-specific request hardening (no-op for other models).
	if err := transformer.ApplyM3RequestHardening(anthropicReq, openaiReq.Tools, modelConfig); err != nil {
		h.sendError(w, http.StatusInternalServerError, "failed to apply M3 request hardening", err)
		return
	}

	// Marshal the Anthropic request, adding M3-specific top-level fields if needed.
	body, err := transformer.BuildM3RequestBody(anthropicReq, modelConfig)
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, "failed to marshal Anthropic request", err)
		return
	}

	isStreaming := openaiReq.Stream != nil && *openaiReq.Stream

	// Dump the transformed request body for debugging
	if dumpPath := debug.DumpRequest(modelConfig.ModelID, "anthropic", body); dumpPath != "" {
		h.logger.Info("dumped Anthropic request", "path", dumpPath)
	}

	// Step 2: Send to Anthropic endpoint
	resp, err := openCodeClient.SendAnthropicRequest(ctx, body, isStreaming)
	if err != nil {
		h.sendError(w, http.StatusBadGateway, "upstream Anthropic request failed", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// Step 3: Transform response back to OpenAI format
	if isStreaming {
		h.handleAnthropicStreamingResponse(w, resp, modelConfig, openaiReq.Tools)
	} else {
		h.handleAnthropicNonStreamingResponse(w, resp, modelConfig, openaiReq.Tools)
	}
}

// handleAnthropicNonStreamingResponse transforms an Anthropic non-streaming response to OpenAI format.
func (h *CompletionsHandler) handleAnthropicNonStreamingResponse(w http.ResponseWriter, resp *http.Response, modelConfig config.ModelConfig, originalTools []types.ToolDef) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		h.sendError(w, http.StatusBadGateway, "failed to read upstream response", err)
		return
	}

	// Dump raw upstream response for debugging
	if dumpPath := debug.DumpResponse(modelConfig.ModelID, "anthropic", body); dumpPath != "" {
		h.logger.Info("dumped Anthropic response", "path", dumpPath)
	}

	// Parse Anthropic response
	var anthropicResp types.MessageResponse
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		h.sendError(w, http.StatusBadGateway, "failed to parse Anthropic response", err)
		return
	}

	// Transform to OpenAI format
	openaiResp, err := transformer.TransformAnthropicToOpenAI(&anthropicResp, modelConfig.ModelID)
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, "failed to transform response", err)
		return
	}

	// Apply MiniMax M3-specific response sanitization (no-op for other models).
	transformer.SanitizeM3Response(openaiResp, originalTools, modelConfig)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(openaiResp)
}

// handleAnthropicStreamingResponse transforms an Anthropic streaming response to OpenAI SSE format.
func (h *CompletionsHandler) handleAnthropicStreamingResponse(w http.ResponseWriter, resp *http.Response, modelConfig config.ModelConfig, originalTools []types.ToolDef) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	modelID := modelConfig.ModelID

	// Wrap response body with debug dump reader (upstream raw stream)
	upstreamFilePath := debug.CreateStreamFile(modelID)
	reader := debug.DumpReader(resp.Body, upstreamFilePath)
	if upstreamFilePath != "" {
		h.logger.Info("dumping upstream stream", "path", upstreamFilePath)
	}

	// Wrap writer with debug dump writer (downstream SSE output)
	downstreamFilePath := debug.CreateDownstreamFile(modelID)
	writer := debug.DumpWriter(w, downstreamFilePath)
	if downstreamFilePath != "" {
		h.logger.Info("dumping downstream stream", "path", downstreamFilePath)
	}

	streamTransformer := transformer.NewAnthropicStreamTransformer(modelID, h.logger)
	streamTransformer.SetOriginalTools(originalTools)
	streamTransformer.SetModelConfig(modelConfig)
	_, err := streamTransformer.TransformStream(reader, writer)
	if err != nil {
		h.logger.Warn("error transforming Anthropic stream", "error", err)
	}
}

// applyModelConfig applies model-specific configuration overrides for OpenAI-format requests.
func (h *CompletionsHandler) applyModelConfig(req *types.ChatCompletionRequest, modelConfig config.ModelConfig) {
	// Override model ID
	req.Model = modelConfig.ModelID

	// Apply temperature override
	if modelConfig.Temperature > 0 {
		req.Temperature = &modelConfig.Temperature
	}

	// Apply max_tokens override
	if modelConfig.MaxTokens > 0 {
		req.MaxTokens = &modelConfig.MaxTokens
	}

	// Apply reasoning parameters for models that support thinking/reasoning.
	// Some providers (e.g. DeepSeek) require thinking.type=enabled when reasoning_effort
	// is set, and reject requests where thinking is disabled alongside reasoning_effort.
	// Therefore, if the model config explicitly enables thinking, we always keep it enabled.
	if modelConfig.ReasoningEffort != "" || len(modelConfig.Thinking) > 0 {
		// Check if the config explicitly enables thinking
		thinkingEnabled := h.isThinkingEnabled(modelConfig.Thinking)

		if thinkingEnabled {
			// Config explicitly enables thinking — always apply it, regardless of
			// conversation content. DeepSeek requires thinking=enabled when
			// reasoning_effort is set.
			if modelConfig.ReasoningEffort != "" {
				req.ReasoningEffort = &modelConfig.ReasoningEffort
			}
			req.Thinking = modelConfig.Thinking
		} else {
			// No explicit thinking enable in config — use heuristic based on
			// conversation content (e.g. for models like kimi-k2.6 where thinking
			// is optional and triggered by conversation context).
			hasThinking := h.hasThinkingContent(req.Messages)

			if hasThinking {
				if modelConfig.ReasoningEffort != "" {
					req.ReasoningEffort = &modelConfig.ReasoningEffort
				}
				if len(modelConfig.Thinking) > 0 {
					req.Thinking = modelConfig.Thinking
				}
			} else if len(modelConfig.Thinking) > 0 {
				// Disable thinking mode if no thinking content but config has thinking params
				req.Thinking = json.RawMessage(`{"type":"disabled"}`)
			}
		}
	}
}

// applyAnthropicModelConfig applies model-specific configuration overrides for Anthropic-format requests.
func (h *CompletionsHandler) applyAnthropicModelConfig(req *types.MessageRequest, modelConfig config.ModelConfig) {
	// Override model ID
	req.Model = modelConfig.ModelID

	// Apply temperature override
	if modelConfig.Temperature > 0 {
		req.Temperature = &modelConfig.Temperature
	}

	// Apply max_tokens override
	if modelConfig.MaxTokens > 0 {
		req.MaxTokens = modelConfig.MaxTokens
	}
}

// hasThinkingContent checks if any message contains reasoning content.
// It checks both the reasoning_content field and reasoning-type content blocks
// inside the content array (Kilocode sends reasoning as content parts).
func (h *CompletionsHandler) hasThinkingContent(messages []types.ChatMessage) bool {
	for _, msg := range messages {
		// Check the dedicated reasoning_content field
		if msg.ReasoningContent != nil && *msg.ReasoningContent != "" {
			return true
		}
		// Check for reasoning-type content blocks in the content array
		// Kilocode sends: {"type": "reasoning", "text": "..."} as content parts
		if len(msg.Content) > 0 {
			var parts []types.ContentPart
			if err := json.Unmarshal(msg.Content, &parts); err == nil {
				for _, part := range parts {
					if part.Type == "reasoning" {
						return true
					}
				}
			}
		}
	}
	return false
}

// isThinkingEnabled checks if the thinking configuration explicitly enables thinking.
// It parses the thinking JSON and returns true if type is "enabled".
func (h *CompletionsHandler) isThinkingEnabled(thinking json.RawMessage) bool {
	if len(thinking) == 0 {
		return false
	}
	var thinkingObj struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(thinking, &thinkingObj); err != nil {
		return false
	}
	return thinkingObj.Type == "enabled"
}

// sendError sends an error response.
func (h *CompletionsHandler) sendError(w http.ResponseWriter, statusCode int, message string, err error) {
	if err != nil {
		h.logger.Error(message, "error", err)
		message = fmt.Sprintf("%s: %v", message, err)
	} else {
		h.logger.Error(message)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResp := map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    "invalid_request_error",
		},
	}

	_ = json.NewEncoder(w).Encode(errorResp)
}
