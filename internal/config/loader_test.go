package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadJSON(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfgJSON := `{
		"api_key": "test-key-123",
		"host": "0.0.0.0",
		"port": 8080,
		"auth_mode": "config",
		"opencode_go": {
			"base_url": "https://custom.url/v1",
			"timeout_ms": 60000
		},
		"logging": {
			"level": "debug",
			"requests": true
		}
	}`

	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("GO_PROXY_CONFIG", cfgPath)
	defer func() { _ = os.Unsetenv("GO_PROXY_CONFIG") }()

	// Prevent env var API key from overriding test config
	oldAPIKey := os.Getenv("GO_PROXY_API_KEY")
	_ = os.Unsetenv("GO_PROXY_API_KEY")
	defer func() { _ = os.Setenv("GO_PROXY_API_KEY", oldAPIKey) }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.APIKey != "test-key-123" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "test-key-123")
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("Host = %q, want %q", cfg.Host, "0.0.0.0")
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want %d", cfg.Port, 8080)
	}
	if cfg.AuthMode != "config" {
		t.Errorf("AuthMode = %q, want %q", cfg.AuthMode, "config")
	}
	if cfg.OpenCodeGo.BaseURL != "https://custom.url/v1" {
		t.Errorf("BaseURL = %q, want %q", cfg.OpenCodeGo.BaseURL, "https://custom.url/v1")
	}
	if cfg.OpenCodeGo.TimeoutMs != 60000 {
		t.Errorf("TimeoutMs = %d, want %d", cfg.OpenCodeGo.TimeoutMs, 60000)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.Logging.Level, "debug")
	}
	if !cfg.Logging.Requests {
		t.Error("Logging.Requests = false, want true")
	}
}

func TestLoadMissingAPIKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfgJSON := `{"host": "127.0.0.1", "auth_mode": "config"}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("GO_PROXY_CONFIG", cfgPath)
	defer func() { _ = os.Unsetenv("GO_PROXY_CONFIG") }()

	// Prevent env var API key from making this test pass incorrectly
	oldAPIKey := os.Getenv("GO_PROXY_API_KEY")
	_ = os.Unsetenv("GO_PROXY_API_KEY")
	defer func() { _ = os.Setenv("GO_PROXY_API_KEY", oldAPIKey) }()

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for missing API key in config auth mode, got nil")
	}
}

func TestPassthroughAuthModeNoAPIKeyRequired(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	// In passthrough mode, no API key is needed in config
	cfgJSON := `{"auth_mode": "passthrough"}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("GO_PROXY_CONFIG", cfgPath)
	defer func() { _ = os.Unsetenv("GO_PROXY_CONFIG") }()

	oldAPIKey := os.Getenv("GO_PROXY_API_KEY")
	_ = os.Unsetenv("GO_PROXY_API_KEY")
	defer func() { _ = os.Setenv("GO_PROXY_API_KEY", oldAPIKey) }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error in passthrough mode: %v", err)
	}
	if cfg.AuthMode != "passthrough" {
		t.Errorf("AuthMode = %q, want %q", cfg.AuthMode, "passthrough")
	}
}

func TestEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfgJSON := `{"api_key": "file-key"}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("GO_PROXY_CONFIG", cfgPath)
	_ = os.Setenv("GO_PROXY_API_KEY", "env-key")
	_ = os.Setenv("GO_PROXY_HOST", "env-host")
	_ = os.Setenv("GO_PROXY_PORT", "9999")
	_ = os.Setenv("GO_PROXY_AUTH_MODE", "passthrough")
	_ = os.Setenv("GO_PROXY_OPENCODE_URL", "https://env-url/v1")
	_ = os.Setenv("GO_PROXY_LOG_LEVEL", "warn")
	defer func() {
		_ = os.Unsetenv("GO_PROXY_CONFIG")
		_ = os.Unsetenv("GO_PROXY_API_KEY")
		_ = os.Unsetenv("GO_PROXY_HOST")
		_ = os.Unsetenv("GO_PROXY_PORT")
		_ = os.Unsetenv("GO_PROXY_AUTH_MODE")
		_ = os.Unsetenv("GO_PROXY_OPENCODE_URL")
		_ = os.Unsetenv("GO_PROXY_LOG_LEVEL")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.APIKey != "env-key" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "env-key")
	}
	if cfg.Host != "env-host" {
		t.Errorf("Host = %q, want %q", cfg.Host, "env-host")
	}
	if cfg.Port != 9999 {
		t.Errorf("Port = %d, want %d", cfg.Port, 9999)
	}
	if cfg.AuthMode != "passthrough" {
		t.Errorf("AuthMode = %q, want %q", cfg.AuthMode, "passthrough")
	}
	if cfg.OpenCodeGo.BaseURL != "https://env-url/v1" {
		t.Errorf("BaseURL = %q, want %q", cfg.OpenCodeGo.BaseURL, "https://env-url/v1")
	}
	if cfg.Logging.Level != "warn" {
		t.Errorf("LogLevel = %q, want %q", cfg.Logging.Level, "warn")
	}
}

func TestDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	// Minimal config — only API key, everything else should default.
	cfgJSON := `{"api_key": "test-key"}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("GO_PROXY_CONFIG", cfgPath)
	defer func() { _ = os.Unsetenv("GO_PROXY_CONFIG") }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Host != defaultHost {
		t.Errorf("Host = %q, want %q", cfg.Host, defaultHost)
	}
	if cfg.Port != defaultPort {
		t.Errorf("Port = %d, want %d", cfg.Port, defaultPort)
	}
	if cfg.AuthMode != defaultAuthMode {
		t.Errorf("AuthMode = %q, want %q", cfg.AuthMode, defaultAuthMode)
	}
	if cfg.OpenCodeGo.BaseURL != defaultBaseURL {
		t.Errorf("BaseURL = %q, want %q", cfg.OpenCodeGo.BaseURL, defaultBaseURL)
	}
	if cfg.OpenCodeGo.TimeoutMs != defaultTimeoutMs {
		t.Errorf("TimeoutMs = %d, want %d", cfg.OpenCodeGo.TimeoutMs, defaultTimeoutMs)
	}
	if cfg.Logging.Level != defaultLogLevel {
		t.Errorf("LogLevel = %q, want %q", cfg.Logging.Level, defaultLogLevel)
	}
}

func TestInterpolateEnvVars(t *testing.T) {
	_ = os.Setenv("TEST_SECRET", "my-secret-value")
	defer func() { _ = os.Unsetenv("TEST_SECRET") }()

	input := `{"api_key": "${TEST_SECRET}", "host": "${UNSET_VAR:-fallback}"}`
	result := interpolateEnvVars(input)

	want := `{"api_key": "my-secret-value", "host": "${UNSET_VAR:-fallback}"}`
	if result != want {
		t.Errorf("interpolateEnvVars() = %q, want %q", result, want)
	}
}

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		input string
		want  string
	}{
		{"~/some/path", filepath.Join(home, "some/path")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, tt := range tests {
		got := expandHome(tt.input)
		if got != tt.want {
			t.Errorf("expandHome(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
