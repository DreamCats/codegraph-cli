# Repository Instructions

Keep commentary concise, practical, and non-performative.
Only expand when explicitly asked.

Use `rtk` as the prefix for shell commands.

This is a Go CLI repository. Prefer the existing layers:

- `cmd/codegraph` for binary entrypoint only.
- `internal/cli` for flags, target resolution, and output.
- `internal/indexer`, `internal/extract`, `internal/resolver`, `internal/graph` for behavior.
- `internal/store`, `internal/config`, `internal/registry` for persistence and configuration.

Before handing off changes, run:

```bash
rtk gofmt -w .
rtk go test ./...
rtk go build ./...
```
