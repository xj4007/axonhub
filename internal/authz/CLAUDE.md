[Root](../../CLAUDE.md) > [internal](../) > **authz**

# Authorization Module (`internal/authz/`)

## Module Responsibilities

Provides the core authorization primitives for AxonHub. Defines principals (authenticated entities), scopes (permission identifiers), bypass contexts, and system-level authorization helpers.

## Key Files

- `principal.go` -- Principal type representing an authenticated entity (user, API key, system)
- `scope.go` -- Scope definitions and validation
- `bypass.go` -- Context bypass for internal operations
- `system.go` -- System-level authorization context
- `test.go` -- Test helpers for creating auth contexts
- `doc.go` -- Package documentation

## Testing

- `principal_test.go`, `scope_test.go`, `bypass_test.go`
- Run: `go test ./internal/authz/...`

## Changelog

- **2026-02-21**: Initial documentation created during architecture scan.
