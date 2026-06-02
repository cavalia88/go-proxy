// Package main is the CLI entry point for the go-proxy server.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"go-proxy/internal/config"
	"go-proxy/internal/debug"
	"go-proxy/internal/server"

	"github.com/spf13/cobra"
)

const (
	appName     = "go-proxy"
	pidFileName = "go-proxy.pid"
)

// Version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:   appName,
		Short: "OpenAI-compatible proxy for OpenCode Go models",
		Long: `go-proxy is a CLI proxy tool that exposes an OpenAI-compatible API for 
OpenCode Go models. It allows VS Code extensions like Kilocode and Cline to 
use all OpenCode Go models directly by routing requests to the appropriate 
model endpoint.

Configuration is stored at ~/.config/go-proxy/config.json`,
		Version: version,
	}

	// Add subcommands.
	rootCmd.AddCommand(serveCmd())
	rootCmd.AddCommand(initCmd())
	rootCmd.AddCommand(validateCmd())
	rootCmd.AddCommand(modelsCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// serveCmd returns the command to start the proxy server.
func serveCmd() *cobra.Command {
	var configPath string
	var port int
	var debugDump bool

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the proxy server",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Override config path if provided.
			if configPath != "" {
				_ = os.Setenv("GO_PROXY_CONFIG", configPath)
			}

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Override port if provided via flag.
			if port != 0 {
				cfg.Port = port
			}

			// Enable debug dump if requested via flag.
			if debugDump {
				debug.SetEnabled(true)
			}

			// Write PID file for process management.
			pidPath := getPIDPath()
			if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to write PID file: %v\n", err)
			}
			defer func() { _ = os.Remove(pidPath) }()

			// Create and start server.
			srv, err := server.NewServer(cfg)
			if err != nil {
				return fmt.Errorf("failed to create server: %w", err)
			}

			fmt.Printf("Starting %s v%s\n", appName, version)
			fmt.Printf("Listening on %s:%d\n", cfg.Host, cfg.Port)
			fmt.Printf("Auth mode: %s\n", cfg.AuthMode)
			fmt.Printf("Forwarding to: %s\n", cfg.OpenCodeGo.BaseURL)
			fmt.Println()
			fmt.Println("Configure Kilocode/Cline with:")
			fmt.Printf("  Base URL: http://%s:%d/v1\n", cfg.Host, cfg.Port)
			fmt.Printf("  API Key: %s\n", map[bool]string{true: "your-opencode-key", false: "(from config)"}[cfg.AuthMode == "passthrough"])
			fmt.Println()

			return srv.Start()
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to config file")
	cmd.Flags().IntVarP(&port, "port", "p", 0, "Override listen port")
	cmd.Flags().BoolVarP(&debugDump, "debug-dump", "d", false, "Dump raw upstream request/response bodies to debug-dumps/ directory")

	return cmd
}

// initCmd returns the command to create a default configuration file.
func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create default configuration file",
		RunE: func(cmd *cobra.Command, args []string) error {
			configDir := getConfigDir()
			configPath := filepath.Join(configDir, "config.json")

			// Check if config already exists
			if _, err := os.Stat(configPath); err == nil {
				return fmt.Errorf("config file already exists at %s (remove it first or edit manually)", configPath)
			}

			if err := os.MkdirAll(configDir, 0755); err != nil {
				return fmt.Errorf("failed to create config directory: %w", err)
			}

			if err := os.WriteFile(configPath, []byte(getDefaultConfig()), 0644); err != nil {
				return fmt.Errorf("failed to write config file: %w", err)
			}

			fmt.Printf("Created default config at %s\n", configPath)
			fmt.Println("Edit the file and add your OpenCode Go API key.")
			return nil
		},
	}
}

// validateCmd returns the command to validate the configuration file.
func validateCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if configPath != "" {
				_ = os.Setenv("GO_PROXY_CONFIG", configPath)
			}

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("invalid config: %w", err)
			}

			fmt.Println("Configuration is valid!")
			fmt.Printf("  Host: %s\n", cfg.Host)
			fmt.Printf("  Port: %d\n", cfg.Port)
			fmt.Printf("  Auth mode: %s\n", cfg.AuthMode)
			if cfg.APIKey != "" {
				fmt.Printf("  API Key: %s...\n", maskString(cfg.APIKey, 8))
			} else {
				fmt.Println("  API Key: (not set, passthrough mode)")
			}
			fmt.Printf("  Base URL: %s\n", cfg.OpenCodeGo.BaseURL)
			fmt.Printf("  Anthropic URL: %s\n", cfg.OpenCodeGo.AnthropicBaseURL)
			fmt.Printf("  Models configured: %d\n", len(cfg.Models))
			for id, mc := range cfg.Models {
				fmt.Printf("    %s → %s (%s)\n", id, mc.ModelID, mc.Endpoint)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to config file")
	return cmd
}

// modelsCmd returns the command to list available models.
func modelsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "models",
		Short: "List available OpenCode Go models",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Available OpenCode Go models:")
			fmt.Println()
			fmt.Println("  Model ID            Endpoint Type")
			fmt.Println("  ────────────────────────────────────────────")
			fmt.Println("  glm-5.1             OpenAI-compatible")
			fmt.Println("  glm-5               OpenAI-compatible")
			fmt.Println("  kimi-k2.6           OpenAI-compatible")
			fmt.Println("  kimi-k2.5           OpenAI-compatible")
			fmt.Println("  mimo-v2.5-pro       OpenAI-compatible")
			fmt.Println("  mimo-v2.5           OpenAI-compatible")
			fmt.Println("  mimo-v2-pro          OpenAI-compatible")
			fmt.Println("  mimo-v2-omni         OpenAI-compatible")
			fmt.Println("  deepseek-v4-pro      OpenAI-compatible")
			fmt.Println("  deepseek-v4-flash    OpenAI-compatible")
			fmt.Println("  qwen3.7-max          Anthropic-compatible")
			fmt.Println("  qwen3.6-plus         OpenAI-compatible")
			fmt.Println("  qwen3.5-plus         OpenAI-compatible")
			fmt.Println("  minimax-m3           Anthropic-compatible")
			fmt.Println("  minimax-m2.7         Anthropic-compatible")
			fmt.Println("  minimax-m2.5         Anthropic-compatible")
			fmt.Println()
			fmt.Println("Use these model IDs in your config.json file.")
		},
	}
}

// getConfigDir returns the default configuration directory path.
func getConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "go-proxy")
}

// getPIDPath returns the path to the PID file.
func getPIDPath() string {
	return filepath.Join(os.TempDir(), pidFileName)
}

// maskString masks all but the first `visible` characters of a string.
func maskString(s string, visible int) string {
	if len(s) <= visible {
		return s
	}
	return s[:visible] + "..."
}

// getDefaultConfig returns a default configuration JSON template.
func getDefaultConfig() string {
	return `{
  "api_key": "${GO_PROXY_API_KEY}",
  "host": "127.0.0.1",
  "port": 3456,
  "auth_mode": "config",
  "models": {
    "glm-5.1": {
      "model_id": "glm-5.1",
      "endpoint": "openai"
    },
    "glm-5": {
      "model_id": "glm-5",
      "endpoint": "openai"
    },
    "kimi-k2.6": {
      "model_id": "kimi-k2.6",
      "endpoint": "openai"
    },
    "kimi-k2.5": {
      "model_id": "kimi-k2.5",
      "endpoint": "openai"
    },
    "mimo-v2.5-pro": {
      "model_id": "mimo-v2.5-pro",
      "endpoint": "openai"
    },
    "mimo-v2.5": {
      "model_id": "mimo-v2.5",
      "endpoint": "openai"
    },
	"mimo-v2-pro": {
      "model_id": "mimo-v2-pro",
      "endpoint": "openai"
    },
    "mimo-v2-omni": {
      "model_id": "mimo-v2-omni",
      "endpoint": "openai"
    },
    "deepseek-v4-pro": {
      "model_id": "deepseek-v4-pro",
      "endpoint": "openai",
      "reasoning_effort": "high",
      "thinking": {"type": "enabled", "budget_tokens": 10000}
    },
    "deepseek-v4-flash": {
      "model_id": "deepseek-v4-flash",
      "endpoint": "openai",
      "reasoning_effort": "medium",
      "thinking": {"type": "enabled", "budget_tokens": 5000}
    },
	"qwen3.5-plus": {
      "model_id": "qwen3.5-plus",
      "endpoint": "openai"
    },
    "qwen3.6-plus": {
      "model_id": "qwen3.6-plus",
      "endpoint": "openai"
    },
    "qwen3.7-max": {
      "model_id": "qwen3.7-max",
      "endpoint": "anthropic"
    },
	"minimax-m3": {
      "model_id": "minimax-m3",
      "endpoint": "anthropic"
    },
    "minimax-m2.7": {
      "model_id": "minimax-m2.7",
      "endpoint": "anthropic"
    },
    "minimax-m2.5": {
      "model_id": "minimax-m2.5",
      "endpoint": "anthropic"
    }
  },
  "opencode_go": {
    "base_url": "https://opencode.ai/zen/go/v1/chat/completions",
    "anthropic_base_url": "https://opencode.ai/zen/go/v1/messages",
    "timeout_ms": 300000
  },
  "logging": {
    "level": "info",
    "requests": true
  }
}
`
}
