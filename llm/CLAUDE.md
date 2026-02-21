[Root](../CLAUDE.md) > **llm**

# LLM Module

## Module Responsibilities

The `llm` module is a standalone Go module (`github.com/looplj/axonhub/llm`) that implements the core AI provider transformation pipeline. It handles bidirectional request/response transformation between AxonHub's unified format and various AI provider APIs, along with HTTP client management, streaming, and retry logic.

## Entry and Startup

- This is a library module (no `main` entrypoint); it is imported by the root module at `internal/server/biz/channel_llm.go`.
- Go module file: `go.mod` (separate from root `go.mod`).

## External Interfaces

### Transformer Interfaces (`llm/transformer/interfaces.go`)

- **`Inbound`** -- Transforms incoming HTTP requests into the unified `llm.Request` format. Also handles stream conversion for responses back to the client.
- **`Outbound`** -- Transforms the unified `llm.Request` into the provider-specific HTTP request. Also handles response/stream transformation from the provider.
- **`Transformer`** -- Extends `Outbound` with additional capabilities like reranking and embedding.

### Pipeline (`llm/pipeline/`)

- **`pipeline.Factory`** -- Creates pipeline instances that chain inbound/outbound transformers with retry and middleware.
- **`Retryable`** -- Interface for cross-channel retry (failover).
- **`ChannelRetryable`** -- Interface for same-channel retry.
- **`ChannelCustomizedExecutor`** -- Interface for channels needing custom HTTP request execution (e.g., AWS Bedrock).
- **`Executor`** -- Interface for HTTP request execution.
- **`Middleware`** -- Pipeline middleware pattern for cross-cutting concerns (max tokens, billing headers, etc.).

### Supported Providers (under `llm/transformer/`)

| Provider       | Directory           | Notes                              |
|---------------|--------------------|------------------------------------|
| OpenAI        | `openai/`          | Chat completions, Responses API    |
| Anthropic     | `anthropic/`       | Messages API, thinking, tools      |
| Gemini        | `gemini/`          | Native + OpenAI-compatible mode    |
| DeepSeek      | `deepseek/`        | OpenAI-compatible with extensions  |
| AI SDK        | `aisdk/`           | Vercel AI SDK data stream format   |
| Antigravity   | `antigravity/`     | Custom provider with health tracking|
| Claude Code   | `anthropic/claudecode/` | Claude Code OAuth integration |
| Codex         | `openai/codex/`    | OpenAI Codex CLI OAuth integration |
| Responses     | `openai/responses/`| OpenAI Responses API format        |
| Jina          | `jina/`            | Reranking and embedding            |
| Bedrock       | `bedrock/`         | AWS Bedrock streaming              |
| Bailian       | `bailian/`         | Alibaba Cloud Bailian              |
| Doubao        | `doubao/`          | ByteDance Doubao                   |
| Moonshot      | `moonshot/`        | Moonshot AI                        |
| OpenRouter    | `openrouter/`      | OpenRouter aggregator              |
| xAI           | `xai/`             | xAI (Grok)                         |
| NanoGPT       | `nanogpt/`         | NanoGPT                            |
| ModelScope    | `modelscope/`      | Alibaba ModelScope                 |
| Longcat       | `longcat/`         | Longcat provider                   |
| Zai           | `zai/`             | Zai provider                       |

### Other Subpackages

- **`llm/httpclient/`** -- HTTP client with proxy support, SSE decoder, error handling.
- **`llm/streams/`** -- Stream utilities (append, filter, map, slice operations).
- **`llm/simulator/`** -- Request simulator for testing.
- **`llm/auth/`** -- API key provider interface.
- **`llm/oauth/`** -- OAuth token provider with auto-refresh.
- **`llm/pipeline/maxtoken/`** -- Max token enforcement middleware.
- **`llm/pipeline/cc/`** -- Claude Code billing header middleware.
- **`llm/pipeline/stream/`** -- Usage extraction from streams.

## Key Dependencies and Configuration

- Separate Go module with its own `go.mod`.
- Key dependencies: `github.com/samber/lo`, `github.com/tidwall/gjson`, `github.com/stretchr/testify`, `github.com/golang-jwt/jwt/v5`.
- No external configuration files; all settings passed programmatically from the server module.

## Data Models

- **`llm.Request`** -- Unified request format (model, messages, tools, parameters).
- **`llm.Response`** -- Unified response format with usage stats.
- **`llm.StreamChunk`** -- Individual streaming chunk.
- **`httpclient.Request`** / **`httpclient.Response`** -- Raw HTTP-level request/response wrappers.

## Testing and Quality

- Extensive unit tests alongside source files (`*_test.go`).
- Integration tests in `llm/pipeline/integration_test.go` and `llm/pipeline/streaming_integration_test.go`.
- Test data fixtures in `llm/transformer/*/testdata/` directories.
- Run: `cd llm && go test ./...`

## Related File List

- `llm/go.mod` -- Module definition
- `llm/model.go` -- Core data models
- `llm/pipeline/pipeline.go` -- Pipeline factory and execution
- `llm/transformer/interfaces.go` -- Transformer interface definitions
- `llm/transformer/errors.go` -- Shared error types
- `llm/httpclient/client.go` -- HTTP client implementation
- `llm/streams/stream.go` -- Stream abstraction

## Changelog

- **2026-02-21**: Initial module documentation created during architecture scan.
