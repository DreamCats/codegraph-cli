# codegraph-cli

Go rewrite of the local codegraph CLI. It keeps the Python reference repo's CLI shape and storage model while producing a single native binary.

## Current Status

| Command | Status | Notes |
|---|---|---|
| `init / list / info / rm / uninit` | done | multi-repo registry |
| `index [--path P] [--force] [--quiet]` | done | hash-based incremental index |
| `sync` | done | alias of incremental index |
| `query <term> [--kind] [--limit]` | done | FTS5 first, LIKE fallback |
| `files [--glob ...]` | done | indexed files |
| `status` | done | counts by kind/language |
| `resolve` | done | rerun call edge resolver |
| `callers / callees / impact` | done | resolved call graph queries |
| `context` | done | entrypoints, related symbols, relationships, snippets |
| `affected` | done | reverse call graph to affected tests |
| `unlock` | stub | no lock backend yet |

Supported languages: Python, TypeScript, TSX, JavaScript, JSX, Go.

## Quick Start

```bash
go install ./cmd/codegraph

cd /path/to/project
codegraph init --index
codegraph status
codegraph query Service --kind class
codegraph context "fix login bug"
codegraph affected src/foo.py
```

Agent-friendly JSON:

```bash
codegraph --json query Service
codegraph --json status
codegraph --json context "fix login bug"
```

## Storage

```text
~/.config/codegraph/
├── registry.json
└── stores/
    └── <projectKey>/
        ├── codegraph.db
        ├── codegraph.db-wal
        └── errors.log
```

`projectKey` priority: explicit `--name` or `--key`, normalized git remote, then `local/<dir>-<sha1[:12]>`.

Override storage with `$CODEGRAPH_HOME` or `$XDG_CONFIG_HOME/codegraph`.

## Development

```bash
make build
make test
make install
```

The Go implementation is layered under `cmd/codegraph` and `internal/*`; no public `pkg` API is exported yet.
