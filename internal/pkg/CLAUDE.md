[Root](../../CLAUDE.md) > [internal](../) > **pkg**

# Internal Utilities Module (`internal/pkg/`)

## Module Responsibilities

Shared utility packages used across the AxonHub backend. These are internal-only packages not exposed outside the repository.

## Subpackages

| Package        | Path                    | Description                                          |
|---------------|-------------------------|------------------------------------------------------|
| ringbuffer    | `pkg/ringbuffer/`       | Thread-safe ring buffer data structure               |
| sqlite        | `pkg/sqlite/`           | SQLite-specific utilities and configuration          |
| watcher       | `pkg/watcher/`          | Configuration/event watcher (memory + Redis backends)|
| xcache        | `pkg/xcache/`           | Multi-backend cache (memory, Redis, live/indexed)    |
| xcontext      | `pkg/xcontext/`         | Context utilities                                    |
| xerrors       | `pkg/xerrors/`          | Unified error types and formatting                   |
| xfile         | `pkg/xfile/`            | File path utilities                                  |
| xjson         | `pkg/xjson/`            | JSON utilities (compare, safe ops, schema handling)  |
| xmap          | `pkg/xmap/`             | Map utilities                                        |
| xredis        | `pkg/xredis/`           | Redis client wrapper and configuration               |
| xregexp       | `pkg/xregexp/`          | Regex matching utilities                             |
| xtest         | `pkg/xtest/`            | Test comparison and stream helpers                   |
| xtime         | `pkg/xtime/`            | Time formatting and parsing                          |
| xurl          | `pkg/xurl/`             | URL and data URL utilities                           |

## Testing and Quality

- Each package has corresponding `*_test.go` files.
- Run: `go test ./internal/pkg/...`

## Changelog

- **2026-02-21**: Initial documentation created during architecture scan.
