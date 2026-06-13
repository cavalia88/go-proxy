# go-proxy

OpenAI-compatible proxy for [OpenCode Go](https://opencode.ai) models — use any OpenCode Go model with VS Code extensions like [Kilocode](https://github.com/kilocode/kilocode) and [Cline](https://github.com/cline/cline).

`go-proxy` exposes standard OpenAI Chat Completions and Models API endpoints. Your VS Code extension connects to it as an "OpenAI-compatible" provider, and it routes requests to the appropriate OpenCode Go model endpoint.

## Background

This project was forked from and inspired by [`oc-go-cc`](https://github.com/samueltuyizere/oc-go-cc), which was originally designed to let users leverage their OpenCode Go subscription with Claude Code.

We decided to modify and simplify it for use with VS Code extensions like **Kilocode** and **Cline**. The key difference is that `go-proxy` uses a single OpenAI-compatible endpoint for **all supported models** — both OpenAI-format models (GLM, Kimi, MiMo, DeepSeek, Qwen) and Anthropic-format models (MiniMax) — so you don't need separate integrations or switching logic in your editor.

## Supported Models

| Model | Model ID | Endpoint | Notes |
|-------|----------|----------|-------|
| GLM-5.1 | `glm-5.1` | OpenAI | Thinking/reasoning model |
| GLM-5 | `glm-5` | OpenAI | Thinking model |
| Kimi K2.7-Code | `kimi-k2.7-code` | OpenAI | Code-specific tasks |
| Kimi K2.6 | `kimi-k2.6` | OpenAI | Best default choice |
| Kimi K2.5 | `kimi-k2.5` | OpenAI | |
| MiMo-V2.5-Pro | `mimo-v2.5-pro` | OpenAI | |
| MiMo-V2.5 | `mimo-v2.5` | OpenAI | |
| MiMo-V2-Pro | `mimo-v2-pro` | OpenAI | |
| MiMo-V2-Omni | `mimo-v2-omni` | OpenAI | |
| DeepSeek V4 Pro | `deepseek-v4-pro` | OpenAI | Reasoning model |
| DeepSeek V4 Flash | `deepseek-v4-flash` | OpenAI | Fast reasoning |
| Qwen3.6 Plus | `qwen3.6-plus` | OpenAI | |
| Qwen3.5 Plus | `qwen3.5-plus` | OpenAI | Budget option |
| MiniMax M2.7 | `minimax-m2.7` | Anthropic | Long context, reverse-transformed |
| MiniMax M2.5 | `minimax-m2.5` | Anthropic | Long context, reverse-transformed |

## Quick Start

### 1. Install

**Option A — Using `make` (Linux/macOS with make installed):**

```bash
git clone https://github.com/cavalia88/go-proxy
cd go-proxy
make build

# Binary is at bin/go-proxy
# Optionally install to $GOPATH/bin
make install
```

**Option B — Using `go build` (cross-platform, no make required):**

```bash
git clone https://github.com/cavalia88/go-proxy
cd go-proxy

# Windows
go build -o bin/go-proxy.exe ./cmd/go-proxy

# Linux / macOS
go build -o bin/go-proxy ./cmd/go-proxy

# Cross-compile for other platforms
GOOS=darwin GOARCH=arm64 go build -o bin/go-proxy-darwin-arm64 ./cmd/go-proxy
GOOS=linux GOARCH=amd64 go build -o bin/go-proxy-linux-amd64 ./cmd/go-proxy
```

### 2. Initialize Configuration

```bash
go-proxy init
```

Creates a default config at `~/.config/go-proxy/config.json`.

### 3. Set Your API Key

```bash
export GO_PROXY_API_KEY=sk-opencode-your-key-here
```

Or use passthrough mode (see [Auth Modes](#auth-modes)).

### 4. Start the Proxy

```bash
go-proxy serve
```

```
Starting go-proxy v0.1.0
Listening on 127.0.0.1:3456
Auth mode: config
Forwarding to: https://opencode.ai/zen/go/v1/chat/completions

Configure Kilocode/Cline with:
  Base URL: http://127.0.0.1:3456/v1
  API Key: (from config)
```

### 5. Configure Your VS Code Extension

In Kilocode or Cline settings:
- **Provider:** OpenAI Compatible
- **Base URL:** `http://127.0.0.1:3456/v1`
- **API Key:** Your OpenCode Go key (in passthrough mode) or any value (in config mode)
- **Model:** Select from the list above (e.g., `kimi-k2.6`)

## Auth Modes

### Config Mode (default)

The proxy uses the API key from the config file or `GO_PROXY_API_KEY` environment variable. Clients can send any value as the Bearer token.

```json
{
  "auth_mode": "config",
  "api_key": "sk-opencode-your-key"
}
```

### Passthrough Mode

The proxy forwards the Bearer token from each client request to OpenCode Go. Useful when multiple users have different API keys.

```json
{
  "auth_mode": "passthrough"
}
```
### 6. Debugging

Added `-d` / `--debug-dump` CLI flag to the `serve` command. Usage:

```Windows
go-proxy.exe serve --debug-dump
```

When enabled, raw upstream Anthropic request/response bodies are dumped to timestamped files in the `debug-dumps/` directory. No environment variable needed.


## Architecture

```
┌──────────────┐     OpenAI API        ┌────────────┐     OpenAI API       ┌─────────────┐
│  Kilocode /  ├──────────────────────►│  go-proxy  ├────────────────────►│ OpenCode Go │
│  Cline       │  POST /v1/chat/       │  (Proxy)   │  /chat/completions  │  (Upstream)  │
│  (VS Code)   │  completions          │            │                     │             │
└──────────────┘  GET /v1/models       └────────────┘                     └─────────────┘
                                       │
                                       │ For Anthropic models:
                                       │ OpenAI → Anthropic transform
                                       │ Anthropic → OpenAI transform
```

**How it works:**

1. Your VS Code extension sends a request in OpenAI Chat Completions format
2. `go-proxy` looks up the model config, applies overrides (temperature, max_tokens, thinking params)
3. For OpenAI-endpoint models: the request is forwarded directly (passthrough)
4. For Anthropic-endpoint models: the request is transformed to Anthropic format, sent, and the response is transformed back
5. Streaming responses are piped directly to the client

## Configuration

Location: `~/.config/go-proxy/config.json`

Override with `GO_PROXY_CONFIG` environment variable.

```json
{
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
    "kimi-k2.7-code": {
      "model_id": "kimi-k2.7-code",
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
    "qwen3.6-plus": {
      "model_id": "qwen3.6-plus",
      "endpoint": "openai"
    },
    "qwen3.5-plus": {
      "model_id": "qwen3.5-plus",
      "endpoint": "openai"
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
```

### Model Configuration

Each model in the `models` map supports:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `model_id` | string | ✅ | The model ID sent to OpenCode Go |
| `endpoint` | string | ✅ | `openai` or `anthropic` |
| `temperature` | float | ❌ | Override temperature (0.0–2.0) |
| `max_tokens` | int | ❌ | Override max output tokens |
| `reasoning_effort` | string | ❌ | For DeepSeek models: `low`, `medium`, `high` |
| `thinking` | object | ❌ | For DeepSeek models: `{"type": "enabled", "budget_tokens": N}` |

The key in the `models` map is what the client specifies as the model name. The `model_id` is what gets sent to OpenCode Go. This lets you create aliases:

```json
"fast": {
  "model_id": "qwen3.5-plus",
  "endpoint": "openai"
}
```

### Thinking Mode (DeepSeek Models)

DeepSeek V4 Pro and Flash support reasoning/thinking mode. When `reasoning_effort` and `thinking` are configured, `go-proxy` automatically enables them when the conversation contains `reasoning_content` in the message history:

```json
"deepseek-v4-pro": {
  "model_id": "deepseek-v4-pro",
  "endpoint": "openai",
  "reasoning_effort": "high",
  "thinking": {"type": "enabled", "budget_tokens": 10000}
}
```

When no thinking content is detected, thinking is automatically disabled (`{"type": "disabled"}`).

### Environment Variables

| Variable | Purpose | Default |
|---|---|---|
| `GO_PROXY_API_KEY` | OpenCode Go API key (required in config mode) | — |
| `GO_PROXY_CONFIG` | Custom config file path | `~/.config/go-proxy/config.json` |
| `GO_PROXY_HOST` | Proxy listen host | `127.0.0.1` |
| `GO_PROXY_PORT` | Proxy listen port | `3456` |
| `GO_PROXY_AUTH_MODE` | Auth mode: `config` or `passthrough` | `config` |
| `GO_PROXY_OPENCODE_URL` | OpenCode Go API endpoint | `https://opencode.ai/zen/go/v1/chat/completions` |
| `GO_PROXY_LOG_LEVEL` | Log level: `debug`, `info`, `warn`, `error` | `info` |

Environment variable interpolation is supported in config values: `"api_key": "${GO_PROXY_API_KEY}"`

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/chat/completions` | OpenAI Chat Completions (streaming + non-streaming) |
| GET | `/v1/models` | List configured models in OpenAI format |
| GET | `/health` | Health check with metrics |

## CLI Commands

```bash
go-proxy serve              Start the proxy server
go-proxy serve -p 8080      Start on a custom port
go-proxy serve -c /path/to/config.json  Use a custom config
go-proxy init               Create default configuration file
go-proxy validate           Validate configuration file
go-proxy models             List available OpenCode Go models
go-proxy --version          Show version
```

## Troubleshooting

1. Validate your config: `go-proxy validate`
2. Check the server is running: `curl http://127.0.0.1:3456/health`
3. List models: `curl http://127.0.0.1:3456/v1/models`
4. Test a completion: `curl -X POST http://127.0.0.1:3456/v1/chat/completions -H "Content-Type: application/json" -d '{"model":"kimi-k2.6","messages":[{"role":"user","content":"Hello"}]}'`
5. Enable debug logging: `GO_PROXY_LOG_LEVEL=debug go-proxy serve`

## Project Structure

```
cmd/go-proxy/main.go            CLI entry point (cobra commands: serve, init, validate, models)
internal/
  client/
    client.go                   Shared HTTP client utilities
    opencode.go                 HTTP client for OpenCode Go API
    opencode_test.go            Tests for OpenCode Go client
  config/
    config.go                   Config types (ModelConfig, AuthMode)
    loader.go                   JSON loader with env interpolation
    loader_test.go              Tests for config loader
  handlers/
    completions.go              /v1/chat/completions handler
    health.go                   /health handler
    models.go                   /v1/models handler
  metrics/
    metrics.go                  In-memory metrics tracking
  middleware/
    auth.go                     Bearer token extraction (passthrough/config)
    middleware.go               Rate limiter, dedup, request ID
  server/
    server.go                   HTTP server with graceful shutdown
  token/
    counter.go                  Tiktoken-based token counting
  transformer/
    reverse.go                  OpenAI ↔ Anthropic request/response transformation
    reverse_response.go         Non-streaming response reverse transform
    reverse_stream.go           Streaming SSE reverse transform
configs/
  config.example.json           Reference config with all options documented
Makefile                        Build, test, lint, and cross-compilation targets
```

## License

MIT
