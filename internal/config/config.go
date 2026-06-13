// Package config handles application configuration loading and validation.
package config

import "encoding/json"

// Config holds the complete application configuration.
type Config struct {
	APIKey     string                 `json:"api_key"`
	Host       string                 `json:"host"`
	Port       int                    `json:"port"`
	AuthMode   string                 `json:"auth_mode"` // "passthrough" or "config"
	Models     map[string]ModelConfig `json:"models"`
	OpenCodeGo OpenCodeGoConfig       `json:"opencode_go"`
	Logging    LoggingConfig          `json:"logging"`
}

// ModelConfig defines configuration for a specific model.
type ModelConfig struct {
	ModelID         string          `json:"model_id"`
	Endpoint        string          `json:"endpoint"` // "openai" or "anthropic"
	Temperature     float64         `json:"temperature,omitempty"`
	MaxTokens       int             `json:"max_tokens,omitempty"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
	Thinking        json.RawMessage `json:"thinking,omitempty"`

	// MiniMax M3-specific workarounds for tool-calling/schema issues.
	// These flags are ignored for all other models.
	// nil/unset defaults to true for "minimax-m3" and false for every other model.
	M3StrictPrompt      *bool `json:"m3_strict_prompt,omitempty"`
	M3TightenSchemas    *bool `json:"m3_tighten_schemas,omitempty"`
	M3ReasoningSplit    *bool `json:"m3_reasoning_split,omitempty"`
	M3ValidateToolCalls *bool `json:"m3_validate_tool_calls,omitempty"`
}

// IsMiniMaxM3 returns true if this model config refers to the MiniMax M3 model.
func (cfg ModelConfig) IsMiniMaxM3() bool {
	return cfg.ModelID == "minimax-m3"
}

// M3StrictPromptEnabled returns the effective value of the strict prompt flag.
func (cfg ModelConfig) M3StrictPromptEnabled() bool {
	if !cfg.IsMiniMaxM3() {
		return false
	}
	if cfg.M3StrictPrompt == nil {
		return true
	}
	return *cfg.M3StrictPrompt
}

// M3TightenSchemasEnabled returns the effective value of the schema tightening flag.
func (cfg ModelConfig) M3TightenSchemasEnabled() bool {
	if !cfg.IsMiniMaxM3() {
		return false
	}
	if cfg.M3TightenSchemas == nil {
		return true
	}
	return *cfg.M3TightenSchemas
}

// M3ReasoningSplitEnabled returns the effective value of the reasoning split flag.
func (cfg ModelConfig) M3ReasoningSplitEnabled() bool {
	if !cfg.IsMiniMaxM3() {
		return false
	}
	if cfg.M3ReasoningSplit == nil {
		return true
	}
	return *cfg.M3ReasoningSplit
}

// M3ValidateToolCallsEnabled returns the effective value of the tool-call validation flag.
func (cfg ModelConfig) M3ValidateToolCallsEnabled() bool {
	if !cfg.IsMiniMaxM3() {
		return false
	}
	if cfg.M3ValidateToolCalls == nil {
		return true
	}
	return *cfg.M3ValidateToolCalls
}

// OpenCodeGoConfig holds the upstream OpenCode Go API settings.
type OpenCodeGoConfig struct {
	BaseURL          string `json:"base_url"`
	AnthropicBaseURL string `json:"anthropic_base_url"`
	TimeoutMs        int    `json:"timeout_ms"`
}

// LoggingConfig controls application logging behavior.
type LoggingConfig struct {
	Level    string `json:"level"`
	Requests bool   `json:"requests"`
}
