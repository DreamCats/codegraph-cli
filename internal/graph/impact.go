package graph

import (
	"codegraph-cli/internal/resolver"
	storepkg "codegraph-cli/internal/store"
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

func ImpactRadius(store, symbol string, depth, limit int) (map[string]any, error) {
	roots, err := resolver.FindNode(store, symbol)
	if err != nil {
		return nil, err
	}
	if len(roots) == 0 {
		return map[string]any{"symbol": symbol, "matched": 0, "roots": []any{}, "nodes": []any{}, "edges": []any{}}, nil
	}
	db, err := storepkg.Open(store)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	type qitem struct {
		id    string
		depth int
	}
	q := []qitem{}
	visited := map[string]bool{}
	nodes := map[string]map[string]any{}
	for _, r := range roots {
		id := fmt.Sprint(r["id"])
		q = append(q, qitem{id, 0})
		visited[id] = true
		nodes[id] = r
	}

	edges := []map[string]any{}
	for len(q) > 0 && len(nodes) < limit {
		item := q[0]
		q = q[1:]
		if item.depth >= depth {
			continue
		}
		rows, err := db.Query(`SELECT e.id,e.source,e.target,e.kind,e.line,e.col,
			sn.id,sn.kind,sn.name,sn.qualified_name,sn.file_path,sn.start_line
			FROM edges e JOIN nodes sn ON sn.id=e.source
			WHERE e.resolved=1 AND e.target=? AND e.kind IN ('calls','references','imports')`, item.id)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var eid int
			var source, target, kind, sid, sk, sn, sq, sf string
			var line, col sql.NullInt64
			var sl int
			if rows.Scan(&eid, &source, &target, &kind, &line, &col, &sid, &sk, &sn, &sq, &sf, &sl) != nil {
				continue
			}
			edge := map[string]any{"id": eid, "source": source, "target": target, "kind": kind, "line": nullInt(line), "col": nullInt(col), "source_id": sid, "source_kind": sk, "source_name": sn, "source_qname": sq, "source_file": sf, "source_line": sl}
			edges = append(edges, edge)
			if !visited[sid] {
				visited[sid] = true
				nodes[sid] = map[string]any{"id": sid, "kind": sk, "name": sn, "qualified_name": sq, "file_path": sf, "start_line": sl}
				q = append(q, qitem{sid, item.depth + 1})
			}
		}
		rows.Close()
	}
	nodeList := []map[string]any{}
	for _, n := range nodes {
		nodeList = append(nodeList, n)
	}
	sort.Slice(nodeList, func(i, j int) bool {
		return fmt.Sprint(nodeList[i]["id"]) < fmt.Sprint(nodeList[j]["id"])
	})
	return map[string]any{"symbol": symbol, "matched": len(roots), "roots": roots, "nodes": nodeList, "edges": edges}, nil
}

func AffectedFiles(store string, files []string, depth int, testFilter string) (map[string]any, error) {
	db, err := storepkg.Open(store)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT id,kind,name,qualified_name,file_path,language,start_line,end_line FROM nodes WHERE kind != 'import'`)
	if err != nil {
		db.Close()
		return nil, err
	}
	all := scanNodeRows(rows)
	db.Close()
	fileSet := map[string]bool{}
	for _, f := range files {
		fileSet[strings.TrimPrefix(f, "./")] = true
	}
	roots := []map[string]any{}
	impacted := map[string]bool{}
	for _, n := range all {
		if fileSet[fmt.Sprint(n["file_path"])] {
			roots = append(roots, n)
			res, err := ImpactRadius(store, fmt.Sprint(n["id"]), depth, 1000)
			if err != nil {
				return nil, err
			}
			if ns, ok := res["nodes"].([]map[string]any); ok {
				for _, x := range ns {
					impacted[fmt.Sprint(x["file_path"])] = true
				}
			}
		}
	}
	var testRe *regexp.Regexp
	if testFilter != "" {
		testRe, err = regexp.Compile(testFilter)
		if err != nil {
			return nil, err
		}
	}
	impFiles := sortedKeys(impacted)
	tests := []string{}
	for _, p := range impFiles {
		if isTestFile(p, testRe) {
			tests = append(tests, p)
		}
	}
	return map[string]any{"changed_files": files, "root_symbols": roots, "impacted_files": impFiles, "affected_tests": tests}, nil
}

func scanNodeRows(rows *sql.Rows) []map[string]any {
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, kind, name, qn, file, lang string
		var start, end int
		if rows.Scan(&id, &kind, &name, &qn, &file, &lang, &start, &end) == nil {
			out = append(out, map[string]any{"id": id, "kind": kind, "name": name, "qualified_name": qn, "file_path": file, "language": lang, "start_line": start, "end_line": end})
		}
	}
	return out
}

func nullInt(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return int(v.Int64)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func isTestFile(p string, re *regexp.Regexp) bool {
	if re != nil {
		return re.MatchString(p)
	}
	return strings.Contains(p, ".test.") || strings.Contains(p, ".spec.") ||
		strings.Contains(p, "/tests/") || strings.Contains(p, "/test/") ||
		strings.Contains(p, "__tests__/") || strings.HasSuffix(p, "_test.go")
}
