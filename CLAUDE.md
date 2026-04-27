# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build   # Build binary to bin/go-proxy
make run     # Run without building
make test    # Run tests with race detector
make lint    # go vet + test
make clean   # Remove build artifacts
make install # Build and install to $GOPATH/bin
make dist    # Cross-compile for all platforms
```

Run a single test: `go test ./internal/config/ -v`

## Architecture

**Purpose:** go-proxy is an OpenAI-compatible proxy server for OpenCode Go models. It exposes standard OpenAI Chat Completions and Models API endpoints so VS Code extensions like Kilocode and Cline can use all OpenCode Go models directly.

**Direct model routing, no scenario detection.** The model specified by the client is used directly — no fallback chains, no circuit breakers, no scenario-based routing. Models are defined in `~/.config/go-proxy/config.json` with their endpoint type (`openai` or `anthropic`).

**Two endpoint types:**

- OpenAI endpoint (`/v1/chat/completions`) — used by most models (GLM, Kimi, MiMo, Qwen, DeepSeek V4)
- Anthropic endpoint (`/v1/messages`) — used by MiniMax models (reverse-transformed automatically)

**Auth modes:**

- `config` (default) — Uses the API key from config file or `GO_PROXY_API_KEY` env var
- `passthrough` — Forwards the Bearer token from the client request

**Key API endpoints:**

- `POST /v1/chat/completions` — OpenAI Chat Completions (passthrough for OpenAI models, reverse-transformed for Anthropic models)
- `GET /v1/models` — List configured models in OpenAI format
- `GET /health` — Health check with metrics

## Key Files

- `cmd/go-proxy/main.go` — CLI entry point (cobra commands: serve, init, validate, models).
- `internal/config/` — Config types and JSON loader with `${VAR}` env interpolation.
- `internal/handlers/completions.go` — `/v1/chat/completions` handler with model config overrides.
- `internal/handlers/models.go` — `/v1/models` handler.
- `internal/handlers/health.go` — Health check handler.
- `internal/middleware/auth.go` — Auth middleware (passthrough/config modes).
- `internal/middleware/middleware.go` — Rate limiter, dedup, request ID middleware chain.
- `internal/client/opencode.go` — HTTP client for OpenCode Go API.
- `internal/server/server.go` — HTTP server setup with graceful shutdown.
- `internal/transformer/reverse.go` — OpenAI ↔ Anthropic request/response transformation.
- `internal/transformer/reverse_stream.go` — Streaming SSE reverse transformation.
- `internal/metrics/metrics.go` — In-memory metrics tracking.
- `configs/config.example.json` — Reference config with all options documented.

## Environment Variables

| Variable | Purpose | Default |
|---|---|---|
| `GO_PROXY_API_KEY` | OpenCode Go API key (required in config mode) | — |
| `GO_PROXY_CONFIG` | Custom config file path | `~/.config/go-proxy/config.json` |
| `GO_PROXY_HOST` | Proxy listen host | `127.0.0.1` |
| `GO_PROXY_PORT` | Proxy listen port | `3456` |
| `GO_PROXY_AUTH_MODE` | Auth mode: `config` or `passthrough` | `config` |
| `GO_PROXY_OPENCODE_URL` | OpenCode Go API endpoint | `https://opencode.ai/zen/go/v1/chat/completions` |
| `GO_PROXY_LOG_LEVEL` | Log level: `debug`, `info`, `warn`, `error` | `info` |
