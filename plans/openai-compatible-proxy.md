# go-proxy — OpenAI-Compatible Proxy for Kilocode/Cline

## Overview

**go-proxy** is a simplified fork of `oc-go-cc` that exposes an OpenAI-compatible API so VS Code extensions like Kilocode and Cline can use all OpenCode Go models directly. No scenario-based routing, no fallback chains, no circuit breakers — just direct model passthrough.

**Key simplification:** When Kilocode/Cline specifies a model (e.g., `kimi-k2.6`), go-proxy routes all messages to that model's endpoint. Period.

## Architecture

```mermaid
flowchart LR
    subgraph Client
        KC[Kilocode / Cline]
    end

    subgraph Proxy [go-proxy]
        EP[/v1/chat/completions]
        ML[/v1/models]
        AUTH[Auth Middleware]
        DIR{IsAnthropicModel?}
        PT[Passthrough]
        RT[Reverse Transform]
    end

    subgraph Upstream [OpenCode Go]
        OAI[/v1/chat/completions]
        ANT[/v1/messages]
    end

    KC -- OpenAI format --> AUTH
    AUTH --> EP
    KC -- GET --> ML
    EP --> DIR
    DIR -- No --> PT
    DIR -- Yes --> RT
    PT -- forward as-is --> OAI
    RT -- transform to Anthropic --> ANT
    OAI -- OpenAI response --> KC
    ANT -- Anthropic response --> RT
    RT -- transform to OpenAI --> KC
```

## Complete Model List

Based on MODELS.md and user-provided list:

| Model | Model ID | Endpoint | Format | Path |
|-------|----------|----------|--------|------|
| GLM-5.1 | `glm-5.1` | `/v1/chat/completions` | OpenAI | Passthrough |
| GLM-5 | `glm-5` | `/v1/chat/completions` | OpenAI | Passthrough |
| Kimi K2.7-Code | `kimi-k2.7-code` | `/v1/chat/completions` | OpenAI | Passthrough |
| Kimi K2.6 | `kimi-k2.6` | `/v1/chat/completions` | OpenAI | Passthrough |
| Kimi K2.5 | `kimi-k2.5` | `/v1/chat/completions` | OpenAI | Passthrough |
| DeepSeek V4 Pro | `deepseek-v4-pro` | `/v1/chat/completions` | OpenAI | Passthrough* |
| DeepSeek V4 Flash | `deepseek-v4-flash` | `/v1/chat/completions` | OpenAI | Passthrough* |
| MiMo-V2-Pro | `mimo-v2-pro` | `/v1/chat/completions` | OpenAI | Passthrough |
| MiMo-V2-Omni | `mimo-v2-omni` | `/v1/chat/completions` | OpenAI | Passthrough |
| MiMo-V2.5-Pro | `mimo-v2.5-pro` | `/v1/chat/completions` | OpenAI | Passthrough |
| MiMo-V2.5 | `mimo-v2.5` | `/v1/chat/completions` | OpenAI | Passthrough |
| Qwen3.6 Plus | `qwen3.6-plus` | `/v1/chat/completions` | OpenAI | Passthrough |
| Qwen3.5 Plus | `qwen3.5-plus` | `/v1/chat/completions` | OpenAI | Passthrough |
| MiniMax M2.7 | `minimax-m2.7` | `/v1/messages` | Anthropic | **Reverse Transform** |
| MiniMax M2.5 | `minimax-m2.5` | `/v1/messages` | Anthropic | **Reverse Transform** |

\* DeepSeek V4 models need `reasoning_effort` and `thinking` params injected from config when thinking mode is detected in the conversation history.

## What Gets Removed

These components from `oc-go-cc` are **not needed** in go-proxy:

- `internal/router/scenarios.go` — No scenario detection; model is specified by client
- `internal/router/model_router.go` — No model routing; direct passthrough
- `internal/router/fallback.go` — No fallback chains or circuit breakers
- `internal/handlers/messages.go` — No Anthropic-format endpoint needed
- `internal/transformer/request.go` — No Anthropic→OpenAI transform needed
- `internal/transformer/response.go` — No OpenAI→Anthropic response transform needed
- `internal/transformer/stream.go` — No OpenAI→Anthropic stream transform needed
- `internal/daemon/` — Simplify; just run as a plain server
- `cmd/oc-go-cc/` — Rename to `cmd/go-proxy/`

## What Gets Reused

| Component | Path | Changes |
|-----------|------|---------|
| Config loading + env interpolation | `internal/config/` | Add `models_list`, `auth_mode`, simplify model config |
| HTTP client + connection pooling | `internal/client/opencode.go` | Keep `IsAnthropicModel()`, `ChatCompletion()`, `SendAnthropicRequest()` |
| OpenAI types | `pkg/types/openai.go` | Keep as-is |
| Anthropic types | `pkg/types/anthropic.go` | Keep as-is (needed for reverse transform) |
| Token counter | `internal/token/counter.go` | Keep as-is |
| Server lifecycle | `internal/server/server.go` | Simplify routes |
| Metrics | `internal/metrics/metrics.go` | Keep as-is |
| Health handler | `internal/handlers/health.go` | Keep as-is |

## Implementation

#### 1.1 Config — `internal/config/config.go`

Simplified config:

```go
type Config struct {
    APIKey     string            `json:"api_key"`
    Host       string            `json:"host"`
    Port       int               `json:"port"`
    AuthMode   string            `json:"auth_mode"`    // "passthrough" or "config"
    OpenCodeGo OpenCodeGoConfig  `json:"opencode_go"`
    Models     map[string]ModelConfig `json:"models"` // model_id → config
    Logging    LoggingConfig     `json:"logging"`
}

type ModelConfig struct {
    ModelID          string          `json:"model_id"`
    Endpoint         string          `json:"endpoint"`          // "openai" or "anthropic"
    Temperature      float64         `json:"temperature,omitempty"`
    MaxTokens        int             `json:"max_tokens,omitempty"`
    ReasoningEffort  string          `json:"reasoning_effort,omitempty"`
    Thinking         json.RawMessage `json:"thinking,omitempty"`
}
```

No more scenario-based routing maps. Just a flat map of model IDs to their config.

#### 1.2 Completions Handler — `internal/handlers/completions.go`

The main handler for `POST /v1/chat/completions`:

```
1. Parse ChatCompletionRequest from client
2. Extract model ID from request
3. Look up model config; if not found, return 404
4. Extract API key (from Bearer header or config, based on auth_mode)
5. If model endpoint is "openai":
   a. Apply config overrides (temperature, max_tokens, thinking params)
   b. Forward request to OpenCode Go /v1/chat/completions
   c. If streaming: pipe response body directly to client
   d. If non-streaming: parse and return response
6. If model endpoint is "anthropic":
   a. Transform OpenAI request → Anthropic request
   b. Send to Anthropic endpoint
   c. If streaming: transform Anthropic SSE → OpenAI SSE
   d. If non-streaming: transform Anthropic response → OpenAI response
7. Return response to client
```

#### 1.3 Models Handler — `internal/handlers/models.go`

`GET /v1/models` returns the configured models:

```json
{
  "object": "list",
  "data": [
    {"id": "kimi-k2.6", "object": "model", "owned_by": "opencode-go", "permission": []},
    {"id": "glm-5.1", "object": "model", "owned_by": "opencode-go", "permission": []}
  ]
}
```

#### 1.4 Auth Middleware — `internal/middleware/auth.go`

Two modes:
- **`passthrough`**: Forward the client's `Authorization: Bearer sk-xxx` to OpenCode Go
- **`config`** (default): Use the configured `api_key` for all upstream requests

#### 1.5 Server — `internal/server/server.go`

```go
mux.HandleFunc("/v1/chat/completions", completionsHandler.HandleCompletions)
mux.HandleFunc("/v1/models", modelsHandler.HandleModels)
mux.HandleFunc("/health", healthHandler.HandleHealth)
```

#### 1.6 Config Example — `configs/config.example.json`

```json
{
  "api_key": "${OPENCODE_API_KEY}",
  "host": "127.0.0.1",
  "port": 3456,
  "auth_mode": "config",
  "opencode_go": {
    "base_url": "https://opencode.ai/zen/go/v1/chat/completions",
    "anthropic_base_url": "https://opencode.ai/zen/go/v1/messages",
    "timeout_ms": 300000
  },
  "models": {
    "glm-5.1": { "model_id": "glm-5.1", "endpoint": "openai" },
    "glm-5": { "model_id": "glm-5", "endpoint": "openai" },
    "kimi-k2.7-code": { "model_id": "kimi-k2.7-code", "endpoint": "openai" },
    "kimi-k2.6": { "model_id": "kimi-k2.6", "endpoint": "openai" },
    "kimi-k2.5": { "model_id": "kimi-k2.5", "endpoint": "openai" },
    "deepseek-v4-pro": {
      "model_id": "deepseek-v4-pro",
      "endpoint": "openai",
      "reasoning_effort": "max",
      "thinking": {"type": "enabled"}
    },
    "deepseek-v4-flash": {
      "model_id": "deepseek-v4-flash",
      "endpoint": "openai",
      "reasoning_effort": "max",
      "thinking": {"type": "enabled"}
    },
    "mimo-v2-pro": { "model_id": "mimo-v2-pro", "endpoint": "openai" },
    "mimo-v2-omni": { "model_id": "mimo-v2-omni", "endpoint": "openai" },
    "mimo-v2.5-pro": { "model_id": "mimo-v2.5-pro", "endpoint": "openai" },
    "mimo-v2.5": { "model_id": "mimo-v2.5", "endpoint": "openai" },
    "qwen3.6-plus": { "model_id": "qwen3.6-plus", "endpoint": "openai" },
    "qwen3.5-plus": { "model_id": "qwen3.5-plus", "endpoint": "openai" },
    "minimax-m2.7": { "model_id": "minimax-m2.7", "endpoint": "anthropic" },
    "minimax-m2.5": { "model_id": "minimax-m2.5", "endpoint": "anthropic" }
  },
  "logging": { "level": "info", "requests": true }
}
```

### Anthropic Model Support (MiniMax M2.5, M2.7)

#### 2.1 Reverse Request Transformer — `internal/transformer/reverse.go`

Converts OpenAI `ChatCompletionRequest` → Anthropic `MessageRequest`:

| OpenAI | Anthropic |
|--------|-----------|
| `messages[0].role: "system"` | `system` field |
| `messages[].tool_calls` | `tool_use` content blocks |
| `messages[].tool_call_id` + content | `tool_result` content blocks |
| `messages[].reasoning_content` | `thinking` content blocks |
| `tools[].function` | `tools[].input_schema` |
| `max_tokens` | `max_tokens` |
| `temperature` | `temperature` |
| `stream` | `stream` |

#### 2.2 Reverse Response Transformer — `internal/transformer/reverse_response.go`

Converts Anthropic `MessageResponse` → OpenAI `ChatCompletionResponse`:

| Anthropic | OpenAI |
|-----------|--------|
| `content[].type: "text"` | `choices[0].message.content` |
| `content[].type: "tool_use"` | `choices[0].message.tool_calls` |
| `content[].type: "thinking"` | `choices[0].message.reasoning_content` |
| `stop_reason: "end_turn"` | `finish_reason: "stop"` |
| `stop_reason: "tool_use"` | `finish_reason: "tool_calls"` |
| `usage.input_tokens` | `usage.prompt_tokens` |

#### 2.3 Reverse Stream Transformer — `internal/transformer/reverse_stream.go`

Converts Anthropic SSE events → OpenAI SSE chunks:

| Anthropic SSE | OpenAI SSE |
|---------------|-----------|
| `message_start` | Initial chunk with `role: "assistant"` |
| `content_block_start` type: "text" | `delta: {content: ""}` |
| `content_block_delta` type: "text_delta" | `delta: {content: "..."}` |
| `content_block_start` type: "thinking" | `delta: {reasoning_content: ""}` |
| `content_block_delta` type: "thinking_delta" | `delta: {reasoning_content: "..."}` |
| `content_block_start` type: "tool_use" | `delta: {tool_calls: [{id, function: {name}}]}` |
| `content_block_delta` type: "input_json_delta" | `delta: {tool_calls: [{function: {arguments: "..."}}]}` |
| `message_delta` stop_reason | `finish_reason` mapping |
| `message_stop` | `data: [DONE]` |

#### 2.4 Update Completions Handler

Add the Anthropic path to `HandleCompletions`:

```
6. If model endpoint is "anthropic":
   a. Transform OpenAI request → Anthropic request
   b. Send to Anthropic endpoint
   c. If streaming: transform Anthropic SSE → OpenAI SSE
   d. If non-streaming: transform Anthropic response → OpenAI response
```

## Files to Create/Modify

### New Files
| File | Purpose |
|------|---------|
| `cmd/go-proxy/main.go` | CLI entry point (renamed from oc-go-cc) |
| `internal/handlers/completions.go` | `/v1/chat/completions` handler |
| `internal/handlers/models.go` | `/v1/models` handler |
| `internal/middleware/auth.go` | Bearer token extraction |
| `internal/transformer/reverse.go` | OpenAI→Anthropic request transform |
| `internal/transformer/reverse_response.go` | Anthropic→OpenAI response transform |
| `internal/transformer/reverse_stream.go` | Anthropic→OpenAI stream transform |

### Modified Files
| File | Changes |
|------|---------|
| `internal/config/config.go` | Simplify: remove scenarios/fallbacks, add `auth_mode`, simplify `ModelConfig` |
| `internal/config/loader.go` | Update for new config structure |
| `internal/server/server.go` | New routes, remove `/v1/messages` |
| `internal/client/opencode.go` | Simplify: remove `IsAnthropicModel()` hardcode, use config `endpoint` field |
| `internal/handlers/completions.go` | Add Anthropic model path |
| `go.mod` | Update module name to `go-proxy` |
| `configs/config.example.json` | New simplified config format |
| `Makefile` | Update binary name to `go-proxy` |

### Deleted Files
| File | Reason |
|------|--------|
| `internal/router/scenarios.go` | No scenario detection |
| `internal/router/scenarios_test.go` | No scenario detection |
| `internal/router/model_router.go` | No model routing |
| `internal/router/fallback.go` | No fallback chains |
| `internal/router/router.go` | No router needed |
| `internal/handlers/messages.go` | No Anthropic endpoint |
| `internal/transformer/request.go` | No Anthropic→OpenAI transform |
| `internal/transformer/request_test.go` | No Anthropic→OpenAI transform |
| `internal/transformer/response.go` | No OpenAI→Anthropic response transform |
| `internal/transformer/response_test.go` | No OpenAI→Anthropic response transform |
| `internal/transformer/stream.go` | No OpenAI→Anthropic stream transform |
| `internal/transformer/stream_test.go` | No OpenAI→Anthropic stream transform |
| `internal/transformer/transformer.go` | No transformer needed |
| `internal/daemon/` | Simplify; no daemon mode |
| `cmd/oc-go-cc/` | Replaced by `cmd/go-proxy/` |

## Kilocode/Cline Configuration

After running `go-proxy`, users configure their VS Code extension:

**Kilocode/Cline settings:**
- Provider: OpenAI Compatible
- Base URL: `http://localhost:3456/v1`
- API Key: Your OpenCode Go API key (or any key if `auth_mode: "config"`)
- Model: `kimi-k2.6` (or any model ID from the config)

The `/v1/models` endpoint will auto-populate the model dropdown.