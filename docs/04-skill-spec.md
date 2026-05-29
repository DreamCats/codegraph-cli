# Skill Spec

The bundled skill lives at `skills/codegraph/SKILL.md`.

## Trigger

Use the skill when a user needs to:

- search symbol definitions;
- inspect callers/callees;
- inspect project architecture;
- build task context;
- find affected tests;
- inspect index status or indexed files.

## Command Pattern

Prefer explicit project paths during setup:

```bash
codegraph init --path /path/to/project --index
codegraph index --path /path/to/project --quiet
```

Prefer JSON for agent workflows:

```bash
codegraph --json overview --path /path/to/project
codegraph --json context "fix login bug" --path /path/to/project --summary
codegraph --json query Service --path /path/to/project
```

For project understanding, start with `overview`, then use compact `context --summary`, then follow with `callers`, `callees`, or `impact` for specific symbols. Use full `context` only when source snippets are needed.

Full `context --json` payloads may be compacted when they exceed the `--max-json-bytes` threshold. If `output.truncated=true`, use `output.summary_command`, `output.no_code_command`, or `output.full_command`.

If JSON contains `stale.is_stale=true`, run `stale.command`, then retry the original query.
