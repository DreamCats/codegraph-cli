# Motivation

The reference project moved codegraph from an always-on MCP server shape to a CLI + Skill shape to avoid paying schema/context tokens in sessions that never query the graph.

The Go rewrite keeps that product shape and improves runtime properties:

- single native binary;
- lower cold-start overhead than Python;
- no Python virtualenv or Node runtime requirement;
- state remains in SQLite, so each invocation is independent.

The tradeoff is that long-running watcher/daemon behavior is intentionally out of scope for now.
