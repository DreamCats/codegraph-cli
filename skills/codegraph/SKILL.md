---
name: codegraph
description: "代码知识图谱查询。当用户需要按符号名搜索函数/类/方法定义、查调用关系、构造任务上下文、查看文件结构、了解索引状态时使用。支持多仓库切换。"
---

# codegraph

Go implementation of the codegraph CLI. It indexes source files into SQLite and answers graph queries without a long-running MCP server.

## Prepare

```bash
codegraph init --path /path/to/project --index
codegraph index --path /path/to/project --quiet
codegraph index --path /path/to/project --force
```

## Multi-Repo

```bash
codegraph list
codegraph info
codegraph --target foo query MyClass
codegraph --target /path/to/project status
codegraph overview --path /path/to/project
```

## Queries

For project understanding, prefer this order:

```bash
codegraph --json overview
codegraph --json context "task description" --summary
codegraph --json callers SymbolName
codegraph --json callees SymbolName
codegraph --json impact SymbolName --depth 3
```

Use `context --summary` first. Use full `context` only when snippets are needed.

```bash
codegraph overview
codegraph architecture
codegraph query handleRequest
codegraph query Service --kind class --limit 10
codegraph files --glob 'src/**/*.go'
codegraph status
codegraph callers UserService.login
codegraph callees handle_request
codegraph impact UserService.login --depth 3
codegraph context "fix login bug" --summary
codegraph context "fix login bug" --max-nodes 20 --max-code 8
codegraph affected --stdin --filter '_test\\.go$'
```

Use `--json` for agent consumption:

```bash
codegraph --json overview
codegraph --json query MyClass
codegraph --json context "fix login bug" --summary
```

Read commands include `stale` metadata in JSON. If `stale.is_stale` is true, run `stale.command` and retry the query.

Full `context --json` may be compacted when it exceeds `--max-json-bytes`. If `output.truncated=true`, prefer `output.summary_command`, `output.no_code_command`, or `output.full_command` depending on the task.

Most read commands accept either global `--target` or subcommand `--path`. Prefer `--path` when the project path is known:

```bash
codegraph --json overview --path /path/to/project
codegraph --json context "task" --path /path/to/project --summary
```

## Limits

- Supported languages: Python, TypeScript, TSX, JavaScript, JSX, Go.
- Resolver does not infer object types for `obj.method()`.
- `unlock` is currently a no-op because there is no lock backend.
