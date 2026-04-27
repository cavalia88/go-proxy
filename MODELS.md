# Supported Models

This document lists all models available through `go-proxy` and their characteristics.

## Model Overview

| Model | Model ID | Endpoint | Context | Best For |
|-------|----------|----------|---------|----------|
| GLM-5.1 | `glm-5.1` | OpenAI | 128K | Complex reasoning, architecture, tool operations |
| GLM-5 | `glm-5` | OpenAI | 128K | Thinking/reasoning tasks |
| Kimi K2.6 | `kimi-k2.6` | OpenAI | 128K | General purpose (best default) |
| Kimi K2.5 | `kimi-k2.5` | OpenAI | 128K | General purpose |
| MiMo-V2.5-Pro | `mimo-v2.5-pro` | OpenAI | 128K | Balanced performance |
| MiMo-V2.5 | `mimo-v2.5` | OpenAI | 128K | General purpose |
| MiMo-V2-Pro | `mimo-v2-pro` | OpenAI | 128K | Fast responses |
| MiMo-V2-Omni | `mimo-v2-omni` | OpenAI | 128K | Multimodal |
| DeepSeek V4 Pro | `deepseek-v4-pro` | OpenAI | 128K | Deep reasoning with thinking mode |
| DeepSeek V4 Flash | `deepseek-v4-flash` | OpenAI | 128K | Fast reasoning |
| Qwen3.6 Plus | `qwen3.6-plus` | OpenAI | 128K | Cost-efficient |
| Qwen3.5 Plus | `qwen3.5-plus` | OpenAI | 128K | Budget tasks |
| MiniMax M2.7 | `minimax-m2.7` | Anthropic | 1M | Long context |
| MiniMax M2.5 | `minimax-m2.5` | Anthropic | 1M | Long context |

## Endpoint Types

⚠️ **Important:** Not all models use the same API endpoint!

- **OpenAI-compatible** models use `/v1/chat/completions` — these work with direct passthrough in `go-proxy`
- **Anthropic-compatible** models (MiniMax) use `/v1/messages` — these are automatically reverse-transformed by `go-proxy`

`go-proxy` handles routing automatically based on the `endpoint` field in your model config.

## Thinking Mode (DeepSeek V4)

DeepSeek V4 Pro and Flash support reasoning/thinking mode. Configure it in your model config:

```json
{
  "deepseek-v4-pro": {
    "model_id": "deepseek-v4-pro",
    "endpoint": "openai",
    "reasoning_effort": "high",
    "thinking": {"type": "enabled", "budget_tokens": 10000}
  }
}
```

When `reasoning_effort` and `thinking` are configured, `go-proxy` automatically:
- **Enables** thinking mode when the conversation contains `reasoning_content` in message history
- **Disables** thinking mode when no reasoning content is detected

This ensures DeepSeek models work correctly with VS Code extensions that support thinking/reasoning.

## Recommended Configuration

```json
{
  "api_key": "${GO_PROXY_API_KEY}",
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

## Anthropic Models

MiniMax models (M2.5, M2.7) use the Anthropic Messages API endpoint. `go-proxy` automatically handles the reverse transformation:

- **Request:** OpenAI ChatCompletionRequest → Anthropic MessageRequest
- **Response:** Anthropic MessageResponse → OpenAI ChatCompletionResponse
- **Streaming:** Anthropic SSE events → OpenAI SSE events

Configure them in your model config with `"endpoint": "anthropic"`.

---

- [OpenCode Go Documentation](https://opencode.ai/docs/go/)
- [Configuration Reference](../configs/config.example.json)
- [README.md](../README.md) for setup instructions
