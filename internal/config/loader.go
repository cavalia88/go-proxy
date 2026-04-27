package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	defaultConfigPath       = "~/.config/go-proxy/config.json"
	defaultHost             = "127.0.0.1"
	defaultPort             = 3456
	defaultBaseURL          = "https://opencode.ai/zen/go/v1/chat/completions"
	defaultAnthropicBaseURL = "https://opencode.ai/zen/go/v1/messages"
	defaultTimeoutMs        = 300000
	defaultLogLevel         = "info"
	defaultAuthMode         = "config" // "passthrough" or "config"
)

// envVarPattern matches ${ENV_VAR} placeholders in config values.
var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)

// Load reads configuration from a JSON file and applies environment variable overrides.
// Config path resolution:
//  1. GO_PROXY_CONFIG env var (explicit override)
//  2. ~/.config/go-proxy/config.json (default)
func Load() (*Config, error) {
	configPath := resolveConfigPath()

	cfg, err := loadJSON(configPath)
	if err != nil {
		return nil, fmt.Errorf("loading config from %s: %w", configPath, err)
	}

	applyEnvOverrides(cfg)
	applyDefaults(cfg)

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

// resolveConfigPath determines which config file to load.
func resolveConfigPath() string {
	if path := os.Getenv("GO_PROXY_CONFIG"); path != "" {
		return path
	}
	return expandHome(defaultConfigPath)
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// loadJSON reads and parses the configuration file.
func loadJSON(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Interpolate environment variables before parsing.
	data = []byte(interpolateEnvVars(string(data)))

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	return &cfg, nil
}

// interpolateEnvVars replaces ${ENV_VAR} patterns with their actual values.
func interpolateEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		// Extract variable name from ${VAR}
		varName := match[2 : len(match)-1]
		if val := os.Getenv(varName); val != "" {
			return val
		}
		// Leave unchanged if env var is not set
		return match
	})
}

// applyEnvOverrides applies environment variable overrides to the config.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("GO_PROXY_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("GO_PROXY_HOST"); v != "" {
		cfg.Host = v
	}
	if v := os.Getenv("GO_PROXY_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Port = port
		}
	}
	if v := os.Getenv("GO_PROXY_AUTH_MODE"); v != "" {
		cfg.AuthMode = v
	}
	if v := os.Getenv("GO_PROXY_OPENCODE_URL"); v != "" {
		cfg.OpenCodeGo.BaseURL = v
	}
	if v := os.Getenv("GO_PROXY_LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
}

// applyDefaults fills in missing configuration values with sensible defaults.
func applyDefaults(cfg *Config) {
	if cfg.Host == "" {
		cfg.Host = defaultHost
	}
	if cfg.Port == 0 {
		cfg.Port = defaultPort
	}
	if cfg.AuthMode == "" {
		cfg.AuthMode = defaultAuthMode
	}
	if cfg.OpenCodeGo.BaseURL == "" {
		cfg.OpenCodeGo.BaseURL = defaultBaseURL
	}
	if cfg.OpenCodeGo.AnthropicBaseURL == "" {
		cfg.OpenCodeGo.AnthropicBaseURL = defaultAnthropicBaseURL
	}
	if cfg.OpenCodeGo.TimeoutMs == 0 {
		cfg.OpenCodeGo.TimeoutMs = defaultTimeoutMs
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = defaultLogLevel
	}
}

// validate checks that all required configuration fields are present.
func validate(cfg *Config) error {
	// API key is required only in "config" auth mode.
	// In "passthrough" mode, the key comes from the client request.
	if cfg.AuthMode == "config" && cfg.APIKey == "" {
		return fmt.Errorf("api_key is required when auth_mode is 'config' (set via config file or GO_PROXY_API_KEY env var)")
	}
	return nil
}
