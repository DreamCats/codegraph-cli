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
codegraph -C foo query MyClass
codegraph -C /path/to/project status
```

## Queries

```bash
codegraph query handleRequest
codegraph query Service --kind class --limit 10
codegraph files --glob 'src/**/*.go'
codegraph status
codegraph callers UserService.login
codegraph callees handle_request
codegraph impact UserService.login --depth 3
codegraph context "fix login bug" --max-nodes 20 --max-code 8
codegraph affected --stdin --filter '_test\\.go$'
```

Use `--json` for agent consumption:

```bash
codegraph --json query MyClass
codegraph --json context "fix login bug"
```

Read commands include `stale` metadata in JSON. If `stale.is_stale` is true, run `stale.command` and retry the query.

## Limits

- Supported languages: Python, TypeScript, TSX, JavaScript, JSX, Go.
- Resolver does not infer object types for `obj.method()`.
- `unlock` is currently a no-op because there is no lock backend.
