[Root](../../CLAUDE.md) > [internal](../) > **ent**

# Ent ORM Module (`internal/ent/`)

## Module Responsibilities

This module defines the database schema using Ent ORM with code generation. It provides all entity types, CRUD operations, GraphQL integration, privacy policies, and migration logic for AxonHub's data layer.

## Entry and Startup

- Schemas are defined in `internal/ent/schema/`.
- Code generation triggered by `make generate` (runs `go generate` in `internal/server/gql/`).
- Database client created in `internal/server/db/ent.go` and injected via FX.
- Auto-migration runs on server startup.

## External Interfaces

### Entity Schemas (`schema/`)

| Schema                     | Description                                    |
|---------------------------|------------------------------------------------|
| `user.go`                 | User accounts with soft delete                 |
| `role.go`                 | Permission roles with scope-based access       |
| `user_role.go`            | User-role association                          |
| `project.go`              | Multi-project support                          |
| `user_project.go`         | User-project membership                        |
| `channel.go`              | AI provider channel configurations             |
| `api_key.go`              | Authentication API tokens                      |
| `request.go`              | Request logging                                |
| `request_execution.go`    | Per-channel execution records                  |
| `system.go`               | System-wide key-value configuration            |
| `thread.go`               | Conversation threading                         |
| `trace.go`                | Request tracing                                |
| `model.go`                | AI model definitions                           |
| `prompt.go`               | Prompt templates                               |
| `data_storage.go`         | External data storage configurations           |
| `usage_log.go`            | Detailed usage/cost logs                       |
| `channel_model_price.go`  | Per-model pricing for channels                 |
| `channel_model_price_versions.go` | Price version history                  |
| `channel_override_template.go`    | Request override templates             |
| `channel_probe.go`        | Channel health probe results                   |
| `provider_quota_status.go`| Provider quota tracking                        |
| `mixin.go`                | Shared mixins (timestamps, soft delete)        |
| `directives.go`           | GraphQL directives                             |

### Generated Code

- `client.go` -- Ent database client
- `*_create.go`, `*_update.go`, `*_delete.go`, `*_query.go` -- CRUD operations
- `gql_*.go` -- GraphQL integration (collections, edges, nodes, pagination, where inputs)
- `entql.go` -- Ent query language support
- `hook/hook.go` -- Lifecycle hooks
- `intercept/intercept.go` -- Query interceptors
- `enttest/` -- Test utilities

### Data Migration (`migrate/datamigrate/`)

- `migrator.go` -- Data migration framework
- Version-specific migrations (e.g., `v0.3.0`, `v0.4.0`)

## Key Dependencies and Configuration

- `entgo.io/ent` -- ORM framework
- `entgo.io/contrib` -- Ent GraphQL contrib
- Configured via `db.Config` (dialect, DSN, debug).
- Supports: SQLite, PostgreSQL, MySQL, TiDB.

## Data Models

All entities follow consistent patterns:
- UUID primary keys (via `internal/objects/GUID`)
- Created/updated timestamps (via mixins)
- Soft delete support (via `schematype/soft_delete.go`)
- Privacy policies (via `internal/scopes/`)

## Testing and Quality

- Schema-level tests in `internal/ent/channel/type_test.go`, `internal/ent/extra_test.go`.
- Data migration tests in `migrate/datamigrate/*_test.go`.
- Business logic uses `enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")`.
- Run: `go test ./internal/ent/...`

## Related File List

- `internal/ent/schema/*.go` -- Schema definitions
- `internal/ent/client.go` -- Generated client
- `internal/ent/entc.go` -- Ent code generation entry
- `internal/ent/migrate/datamigrate/migrator.go` -- Data migration
- `internal/server/db/ent.go` -- Client creation and connection

## Changelog

- **2026-02-21**: Initial module documentation created during architecture scan.
