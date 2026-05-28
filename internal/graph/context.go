package graph

import (
	"github.com/DreamCats/codegraph-cli/internal/resolver"
	storepkg "github.com/DreamCats/codegraph-cli/internal/store"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var stopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "bug": true,
	"code": true, "fix": true, "for": true, "how": true, "in": true,
	"of": true, "on": true, "or": true, "the": true, "this": true,
	"to": true, "what": true, "where": true, "why": true,
}

var entrypointKinds = map[string]bool{
	"function": true, "method": true, "class": true, "struct": true, "interface": true,
}

func BuildContext(projectRoot, store, task string, maxNodes, maxCode int, includeCode bool) (map[string]any, error) {
	entrypoints := []map[string]any{}
	seen := map[string]bool{}
	for _, term := range ContextTerms(task) {
		limit := maxNodes
		if limit < 10 {
			limit = 10
		}
		candidates, err := Search(store, term, "", limit)
		if err != nil {
			return nil, err
		}
		ordered := append(preferredNodes(candidates, true), preferredNodes(candidates, false)...)
		for _, row := range ordered {
			id := fmt.Sprint(row["id"])
			if seen[id] {
				continue
			}
			seen[id] = true
			entrypoints = append(entrypoints, row)
			if len(entrypoints) >= maxNodes {
				break
			}
		}
		if len(entrypoints) >= maxNodes {
			break
		}
	}

	relatedIDs := []string{}
	relationships := []map[string]any{}
	for _, node := range entrypoints {
		id := fmt.Sprint(node["id"])
		for _, edge := range mustEdges(store, id, "", "calls") {
			relatedIDs = append(relatedIDs, fmt.Sprint(edge["target"]))
			relationships = append(relationships, edge)
		}
		for _, edge := range mustEdges(store, "", id, "calls") {
			relatedIDs = append(relatedIDs, fmt.Sprint(edge["source"]))
			relationships = append(relationships, edge)
		}
	}
	related, err := nodesByID(store, uniqueStrings(relatedIDs))
	if err != nil {
		return nil, err
	}
	if len(related) > maxNodes {
		related = related[:maxNodes]
	}

	codeBlocks := []map[string]any{}
	if includeCode {
		codeNodes := append([]map[string]any{}, entrypoints...)
		codeNodes = append(codeNodes, related...)
		for _, node := range codeNodes {
			if len(codeBlocks) >= maxCode {
				break
			}
			block := map[string]any{
				"id": node["id"], "kind": node["kind"], "name": node["name"],
				"qualified_name": node["qualified_name"], "file_path": node["file_path"],
				"start_line": node["start_line"], "end_line": node["end_line"],
				"language": node["language"], "code": snippet(projectRoot, node, 1),
			}
			codeBlocks = append(codeBlocks, block)
		}
	}
	if len(relationships) > maxNodes*2 {
		relationships = relationships[:maxNodes*2]
	}
	return map[string]any{
		"task": task, "entrypoints": entrypoints, "related": related,
		"relationships": relationships, "code_blocks": codeBlocks,
	}, nil
}

func preferredNodes(nodes []map[string]any, wantEntrypoint bool) []map[string]any {
	out := []map[string]any{}
	for _, n := range nodes {
		isEntry := entrypointKinds[fmt.Sprint(n["kind"])]
		if isEntry == wantEntrypoint {
			out = append(out, n)
		}
	}
	return out
}

func mustEdges(store, sourceID, targetID, kind string) []map[string]any {
	edges, err := resolver.ResolvedEdgesFor(store, sourceID, targetID, kind)
	if err != nil {
		return nil
	}
	return edges
}

func nodesByID(store string, ids []string) ([]map[string]any, error) {
	if len(ids) == 0 {
		return []map[string]any{}, nil
	}
	db, err := storepkg.Open(store)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	out := []map[string]any{}
	for _, id := range ids {
		row := db.QueryRow(`SELECT id,kind,name,qualified_name,file_path,language,start_line,end_line FROM nodes WHERE id = ?`, id)
		m, err := scanOneNode(row)
		if err == nil {
			out = append(out, m)
		}
	}
	return out, nil
}

type nodeScanner interface {
	Scan(dest ...any) error
}

func scanOneNode(row nodeScanner) (map[string]any, error) {
	var id, kind, name, qn, file, lang string
	var start, end int
	err := row.Scan(&id, &kind, &name, &qn, &file, &lang, &start, &end)
	return map[string]any{"id": id, "kind": kind, "name": name, "qualified_name": qn, "file_path": file, "language": lang, "start_line": start, "end_line": end}, err
}

func snippet(root string, node map[string]any, contextLines int) string {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(fmt.Sprint(node["file_path"]))))
	if err != nil {
		return ""
	}
	lines := strings.Split(string(raw), "\n")
	start := asInt(node["start_line"]) - 1 - contextLines
	if start < 0 {
		start = 0
	}
	end := asInt(node["end_line"]) + contextLines
	if end > len(lines) {
		end = len(lines)
	}
	out := []string{}
	for i := start; i < end; i++ {
		out = append(out, fmt.Sprintf("%4d: %s", i+1, lines[i]))
	}
	return strings.Join(out, "\n")
}

func asInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}

func ContextTerms(task string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, term := range regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_$.:-]*`).FindAllString(task, -1) {
		key := strings.ToLower(term)
		if len(key) < 3 || stopWords[key] || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, term)
	}
	if len(out) == 0 && strings.TrimSpace(task) != "" {
		out = append(out, strings.TrimSpace(task))
	}
	return out
}

func FormatContextMarkdown(payload map[string]any) string {
	lines := []string{fmt.Sprintf("# Code Context: %v", payload["task"]), "", "## Entrypoints"}
	entrypoints, _ := payload["entrypoints"].([]map[string]any)
	if len(entrypoints) == 0 {
		lines = append(lines, "- No matching symbols found.")
	} else {
		for _, n := range entrypoints {
			lines = append(lines, fmt.Sprintf("- `%v` (%v) %v:%v", n["qualified_name"], n["kind"], n["file_path"], n["start_line"]))
		}
	}
	if related, _ := payload["related"].([]map[string]any); len(related) > 0 {
		lines = append(lines, "", "## Related Symbols")
		for _, n := range related {
			lines = append(lines, fmt.Sprintf("- `%v` (%v) %v:%v", n["qualified_name"], n["kind"], n["file_path"], n["start_line"]))
		}
	}
	if relationships, _ := payload["relationships"].([]map[string]any); len(relationships) > 0 {
		lines = append(lines, "", "## Relationships")
		for _, e := range relationships {
			lines = append(lines, fmt.Sprintf("- `%v` -> `%v` (%v)", e["source_name"], e["target_qname"], e["kind"]))
		}
	}
	if blocks, _ := payload["code_blocks"].([]map[string]any); len(blocks) > 0 {
		lines = append(lines, "", "## Code")
		for _, b := range blocks {
			lines = append(lines,
				fmt.Sprintf("### `%v`", b["qualified_name"]),
				fmt.Sprintf("%v:%v", b["file_path"], b["start_line"]),
				fmt.Sprintf("```%v", b["language"]),
				fmt.Sprint(b["code"]),
				"```",
				"",
			)
		}
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, x := range in {
		if x == "" || seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	return out
}

var _ nodeScanner = (*sql.Row)(nil)
