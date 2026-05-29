# Requirements

## Functional

- Register projects without writing project-local state.
- Index Python, TypeScript, TSX, JavaScript, JSX, and Go source files.
- Store symbols, files, and call edges in SQLite.
- Support incremental indexing by content hash.
- Resolve call edges across same file, imports, Go packages, Go modules, and simple TS/JS path aliases.
- Provide human and JSON output for all core commands.
- Provide project-level architecture overview and compact task context for agent workflows.
- Warn when read commands see stale indexes.

## Non-Functional

- Native Go binary.
- No daemon requirement.
- Keep CLI, indexer, resolver, graph, store, and registry as separate internal packages.
- Keep project data under `$CODEGRAPH_HOME`, `$XDG_CONFIG_HOME/codegraph`, or `~/.config/codegraph`.

## Acceptance

```bash
go test ./...
go build ./...
codegraph init --path <project> --index
codegraph --json overview --path <project>
codegraph --json query <symbol>
codegraph --json context "task" --summary
codegraph --json affected <file>
```
