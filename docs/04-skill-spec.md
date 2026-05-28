# Skill Spec

The bundled skill lives at `skills/codegraph/SKILL.md`.

## Trigger

Use the skill when a user needs to:

- search symbol definitions;
- inspect callers/callees;
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
codegraph --json --target /path/to/project query Service
codegraph --json --target /path/to/project context "fix login bug"
```

If JSON contains `stale.is_stale=true`, run `stale.command`, then retry the original query.
