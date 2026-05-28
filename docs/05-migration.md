# Migration Notes

This Go rewrite targets feature parity with the Python reference implementation in `/Users/bytedance/Work/tools/cli/codegraph-cli`.

## Parity Checklist

| Area | Go Status |
|---|---|
| Central storage and registry | done |
| CLI command names | done |
| Global `-C/--target`, `--json`, `--verbose` | done |
| Incremental indexing and deleted-file cleanup | done |
| `.gitignore`, default ignores, 1MB limit, `errors.log` | done |
| Python / TS / JS / Go extraction | done |
| FTS5 search and LIKE fallback | done |
| Resolver including Go module and TS/JS path alias | done |
| `context` markdown/json payload shape | done |
| `affected --stdin --depth --filter --quiet` | done |
| `unlock` | stub, same as reference status |
| Tests | core Go tests added |

## Remaining Known Limits

The Go rewrite intentionally keeps the same MVP limits as the Python reference:

- no object type inference for `obj.method()`;
- no framework-specific routing or dependency injection model;
- no watcher/git hook automatic sync;
- no MCP server mode;
- no cross-repo graph.

The JS/TS and Python extractors are lightweight Go implementations rather than tree-sitter bindings, so complex syntax can still require targeted extractor hardening.
