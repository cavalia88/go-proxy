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
