[Root](../../CLAUDE.md) > [internal](../) > **scopes**

# Scopes and Privacy Module (`internal/scopes/`)

## Module Responsibilities

Implements Ent privacy policies and access control rules for database-level permission enforcement. Provides a rule-based system for evaluating whether a principal has access to a given entity.

## Key Files

- `scopes.go` -- Scope constants and helpers
- `policy.go` -- Privacy policy builder
- `rule.go` -- Base rule interface and helpers
- `rule_owner.go` -- Owner-level access rule
- `rule_user_owned.go` -- User-owned entity access rule
- `rule_user_scope.go` -- User scope-based rule
- `rule_user_project_scope.go` -- Project-scoped user rule
- `rule_apikey_scope.go` -- API key scope-based rule

## Testing

- Comprehensive test coverage for each rule type.
- Run: `go test ./internal/scopes/...`

## Changelog

- **2026-02-21**: Initial documentation created during architecture scan.
