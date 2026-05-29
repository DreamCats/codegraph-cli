# Agent Usability Improvements

This note records tool friction found while using `codegraph-cli` as an agent-facing project understanding tool. Items are ordered by expected benefit.

1. Add `codegraph overview` / `codegraph architecture`. Done.
   - Highest benefit. Return package responsibilities, package dependencies, core entrypoints, data flow, key symbols, and storage tables in one project-level response.

2. Add a compressed `context` mode. Done.
   - `context --json` can return too much code and edge data. A compact mode should provide enough orientation without flooding the caller.

3. Make target selection consistent. Partial.
   - Some commands accept `--path` as a subcommand flag while others require global `--target`. Support `--path` consistently or return a targeted hint.
   - Current policy: keep global `--target` for names and paths; support subcommand `--path` as a convenience when the project path is known. Do not deprecate either form yet.

4. Rank core symbols by graph importance. Done.
   - Text search alone does not identify central functions reliably. Ranking should consider degree, exported status, CLI entrypoints, and file/package position.

5. Reduce noisy call edges.
   - Incorrect or low-value edges can mislead architecture summaries. Extractor and resolver noise should be filtered or fixed.

6. Provide package-level dependency JSON.
   - If a full architecture command is too broad, a package graph with imports, file counts, node counts, and exported symbols is still useful.

7. Add large-output protection. Done.
   - Commands that can emit very large JSON should support thresholds, truncation metadata, and suggested follow-up queries.
   - `context --json` compacts oversized payloads by default and includes `output.summary_command`, `output.full_command`, and `output.no_code_command`. Use `--allow-large` or `--max-json-bytes 0` to force full JSON.

8. Improve flag error hints. Done.
   - Errors like `flag provided but not defined: -path` should explain the correct global target syntax or supported replacement.
