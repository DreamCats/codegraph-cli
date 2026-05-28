# Design

## Layout

```text
cmd/codegraph/main.go
internal/cli        # argument parsing and human/json output
internal/config     # CODEGRAPH_HOME, registry paths, project key derivation
internal/registry   # registry.json lifecycle
internal/store      # SQLite connection, schema, metadata helpers
internal/indexer    # walk files, extract, write db, run resolver
internal/extract    # language extractors
internal/resolver   # placeholder call edge resolution
internal/graph      # search, status, files, context, impact, affected
internal/model      # shared structs
```

The CLI layer does not own graph behavior. It resolves the target project, parses flags, calls `internal/*`, and formats output.

## Storage

The CLI uses user-level centralized storage:

```text
~/.config/codegraph/
├── registry.json
└── stores/<projectKey>/codegraph.db
```

No project-local `.codegraph` directory is written.

## Schema

Main tables:

| Table | Purpose |
|---|---|
| `nodes` | symbols: function/class/method/struct/interface/type_alias/constant/variable/import |
| `edges` | relationships: calls/contains/imports/references |
| `files` | indexed source files with content hash |
| `project_metadata` | `project_root`, `last_indexed`, `last_resolved_at` |
| `schema_versions` | migration marker |
| `nodes_fts` | FTS5 search over name, qualified name, docstring, signature |

Edges can be unresolved placeholders with `target = "name:<symbol>"`. Resolver rewrites them to real node IDs and sets `resolved = 1`.

## Resolver

Resolution order:

1. Same file symbol.
2. Go same-package cross-file symbol.
3. Import-driven module candidate lookup.
4. `tsconfig.json` / `jsconfig.json` simple `paths` aliases.
5. Go `go.mod` module path.
6. Globally unique exported symbol fallback.

No type inference, inheritance analysis, or framework-specific resolution is attempted.
