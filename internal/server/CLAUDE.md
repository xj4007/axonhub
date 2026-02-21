[Root](../../CLAUDE.md) > [internal](../) > **server**

# Server Module (`internal/server/`)

## Module Responsibilities

The server module is the HTTP layer of AxonHub. It sets up the Gin HTTP server, configures routes, middleware, API handlers, business logic services, GraphQL resolvers, backup system, and garbage collection. This is the central orchestration point connecting all backend components.

## Entry and Startup

- **`server.go`** -- Creates the Gin engine and HTTP server; `Run()` bootstraps the entire FX dependency graph.
- **`routes.go`** -- Defines all HTTP routes and middleware chains.
- Called from `cmd/axonhub/main.go` via `server.Run(...)`.

## External Interfaces

### REST API Routes (from `routes.go`)

| Group              | Path Pattern                    | Handler                  | Auth         |
|-------------------|---------------------------------|--------------------------|--------------|
| Public            | `GET /health`                   | System.Health            | None         |
| Public            | `GET /favicon`                  | System.GetFavicon        | None         |
| Admin (unauth)    | `GET /admin/system/status`      | System.GetSystemStatus   | None         |
| Admin (unauth)    | `POST /admin/system/initialize` | System.InitializeSystem  | None         |
| Admin (unauth)    | `POST /admin/auth/signin`       | Auth.SignIn              | None         |
| Admin (JWT)       | `POST /admin/graphql`           | GraphQL handler          | JWT          |
| Admin (JWT)       | `POST /admin/playground/chat`   | Playground.ChatCompletion| JWT          |
| Admin (JWT)       | `POST /admin/codex/oauth/*`     | Codex OAuth              | JWT          |
| Admin (JWT)       | `POST /admin/claudecode/oauth/*`| ClaudeCode OAuth         | JWT          |
| Admin (JWT)       | `POST /admin/antigravity/oauth/*`| Antigravity OAuth       | JWT          |
| OpenAPI           | `POST /openapi/v1/graphql`      | OpenAPI GraphQL          | API Key      |
| API (OpenAI)      | `POST /v1/chat/completions`     | OpenAI.ChatCompletion    | API Key      |
| API (OpenAI)      | `POST /v1/responses`            | OpenAI.CreateResponse    | API Key      |
| API (OpenAI)      | `GET /v1/models`                | OpenAI.ListModels        | API Key      |
| API (OpenAI)      | `POST /v1/embeddings`           | OpenAI.CreateEmbedding   | API Key      |
| API (OpenAI)      | `POST /v1/images/generations`   | OpenAI.CreateImage       | API Key      |
| API (Anthropic)   | `POST /v1/messages`             | Anthropic.CreateMessage  | API Key      |
| API (Anthropic)   | `POST /anthropic/v1/messages`   | Anthropic.CreateMessage  | API Key      |
| API (Jina)        | `POST /jina/v1/rerank`          | Jina.Rerank              | API Key      |
| API (Gemini)      | `POST /gemini/:ver/models/*`    | Gemini.GenerateContent   | Gemini Key   |
| API (Gemini)      | `POST /v1beta/models/*`         | Gemini (alias)           | Gemini Key   |

### GraphQL Schemas (in `gql/`)

- `axonhub.graphql` -- Main schema (channels, API keys, users, roles, requests)
- `dashboard.graphql` -- Dashboard statistics and metrics
- `model.graphql` -- Model management
- `prompt.graphql` -- Prompt management
- `cost.graphql` -- Cost tracking
- `price.graphql` -- Channel model pricing
- `backup.graphql` -- Backup/restore operations
- `channel_probe.graphql` -- Channel health probing
- `system.graphql` -- System configuration
- `scopes.graphql` -- Permission scopes
- `me.graphql` -- Current user queries
- `ent.graphql` -- Auto-generated Ent entity types

### Middleware (`middleware/`)

- `auth.go` -- JWT auth, API key auth, Gemini key auth
- `thread.go` -- Thread ID extraction and management
- `trace.go` -- Trace ID extraction and management
- `timeout.go` -- Request timeout handling
- `access_log.go` -- Request access logging
- `metrics.go` -- Metrics collection
- `recover.go` -- Panic recovery
- `ent.go` -- Ent client injection
- `logging.go` -- Logging and tracing context
- `project.go` -- Project ID extraction
- `header.go` -- Header processing
- `error.go` -- Error response formatting

## Key Subpackages

### `biz/` -- Business Logic Services

Core services managing domain logic:

- **ChannelService** -- Channel CRUD, LLM pipeline construction, load balancing, model sync, probe, metrics, pricing
- **AuthService** -- JWT authentication, sign-in, user validation
- **ApiKeyService** -- API key CRUD, quota management
- **RequestService** -- Request logging, persistence
- **ModelService** -- Model management, circuit breaker, association matching
- **TraceService** / **ThreadService** -- Request tracing and threading
- **SystemService** -- System configuration
- **RoleService** -- RBAC role management
- **QuotaService** -- Usage quotas and limits
- **DataStorageService** -- External data storage
- **ProjectService** -- Multi-project support
- **PromptService** -- Prompt template management
- **CostCalcService** -- Cost calculation
- **ProviderQuotaService** -- Provider-level quota checking

### `api/` -- REST API Handlers

HTTP handler structs for each API type (OpenAI, Anthropic, Gemini, AI SDK, Jina, Codex, ClaudeCode, Antigravity, System, Auth, Playground).

### `gql/` -- GraphQL

- Resolvers auto-generated by `gqlgen` from `.graphql` schema files.
- Custom query builders in `qb/` for throughput/dashboard queries.
- OpenAPI sub-graphql at `gql/openapi/`.

### `backup/` -- Backup System

- Auto-backup scheduling
- Backup/restore operations with JSON export
- FX module registration

### `gc/` -- Garbage Collection

- Scheduled database cleanup worker.
- Configurable via cron expression.

### `dependencies/` -- Dependency Wiring

- Pipeline executor creation and FX module registration.

### `static/` -- Frontend Static Assets

- Serves the built frontend from `internal/server/static/dist/`.

## Key Dependencies and Configuration

- **Server Config** (`server.Config`): host, port, name, debug, CORS, timeouts, trace headers.
- Uses `go.uber.org/fx` for dependency injection.
- Uses `github.com/gin-gonic/gin` for HTTP routing.

## Data Models

- All entities defined in `internal/ent/schema/` (see Ent module).
- Business objects in `internal/objects/`.
- GraphQL models auto-generated in `gql/models_gen.go`.

## Testing and Quality

- Unit tests throughout `biz/*_test.go` (extensive coverage).
- API handler tests in `api/*_test.go`.
- Backup tests in `backup/*_test.go`.
- GC tests in `gc/gc_test.go`.
- GraphQL resolver tests (e.g., `gql/dashboard.resolvers_test.go`).
- Middleware tests (`middleware/*_test.go`).
- Run: `go test ./internal/server/...`

## Related File List

- `internal/server/server.go` -- Server creation and lifecycle
- `internal/server/routes.go` -- Route definitions
- `internal/server/config.go` -- Server configuration struct
- `internal/server/biz/fx_module.go` -- Biz service FX module
- `internal/server/api/fx_module.go` -- API handler FX module
- `internal/server/gql/generate.go` -- GraphQL code generation
- `internal/server/gql/gqlgen.yml` -- GraphQL generator config

## Changelog

- **2026-02-21**: Initial module documentation created during architecture scan.
