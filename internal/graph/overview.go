package graph

import (
	"database/sql"
	"fmt"
	storepkg "github.com/DreamCats/codegraph-cli/internal/store"
	"path/filepath"
	"sort"
	"strings"
)

func Overview(store string, packageLimit, symbolLimit, dependencyLimit int) (map[string]any, error) {
	db, err := storepkg.Open(store)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if packageLimit <= 0 {
		packageLimit = 20
	}
	if symbolLimit <= 0 {
		symbolLimit = 12
	}
	if dependencyLimit <= 0 {
		dependencyLimit = 20
	}
	packages, err := overviewPackages(db, packageLimit)
	if err != nil {
		return nil, err
	}
	symbols, err := overviewSymbols(db, symbolLimit)
	if err != nil {
		return nil, err
	}
	deps, err := overviewDependencies(db, dependencyLimit)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"summary": map[string]any{
			"files":          storepkg.CountSQL(db, "SELECT COUNT(*) FROM files"),
			"nodes":          storepkg.CountSQL(db, "SELECT COUNT(*) FROM nodes"),
			"edges":          storepkg.CountSQL(db, "SELECT COUNT(*) FROM edges"),
			"resolved_edges": storepkg.CountSQL(db, "SELECT COUNT(*) FROM edges WHERE resolved = 1"),
		},
		"packages":             packages,
		"core_symbols":         symbols,
		"package_dependencies": deps,
		"storage":              overviewStorage(db),
	}, nil
}

type packageSummary struct {
	Path      string
	Files     int
	Nodes     int
	Languages map[string]int
	Symbols   []string
}

func overviewPackages(db *sql.DB, limit int) ([]map[string]any, error) {
	pkgs := map[string]*packageSummary{}
	rows, err := db.Query(`SELECT path, language, node_count FROM files`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var path, lang string
		var nodeCount int
		if rows.Scan(&path, &lang, &nodeCount) != nil {
			continue
		}
		pkg := packagePath(path)
		if pkgs[pkg] == nil {
			pkgs[pkg] = &packageSummary{Path: pkg, Languages: map[string]int{}}
		}
		pkgs[pkg].Files++
		pkgs[pkg].Nodes += nodeCount
		pkgs[pkg].Languages[lang]++
	}
	rows.Close()
	rows, err = db.Query(`SELECT qualified_name, file_path FROM nodes
		WHERE kind IN ('function','method','class','struct','interface') AND is_exported = 1
		AND file_path NOT LIKE '%_test.go'
		ORDER BY file_path, start_line`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var qn, file string
		if rows.Scan(&qn, &file) != nil {
			continue
		}
		pkg := packagePath(file)
		if pkgs[pkg] == nil || len(pkgs[pkg].Symbols) >= 8 {
			continue
		}
		pkgs[pkg].Symbols = append(pkgs[pkg].Symbols, qn)
	}
	rows.Close()
	list := make([]*packageSummary, 0, len(pkgs))
	for _, pkg := range pkgs {
		list = append(list, pkg)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Nodes == list[j].Nodes {
			return list[i].Path < list[j].Path
		}
		return list[i].Nodes > list[j].Nodes
	})
	if len(list) > limit {
		list = list[:limit]
	}
	out := make([]map[string]any, 0, len(list))
	for _, pkg := range list {
		out = append(out, map[string]any{
			"path":      pkg.Path,
			"files":     pkg.Files,
			"nodes":     pkg.Nodes,
			"languages": pkg.Languages,
			"symbols":   pkg.Symbols,
		})
	}
	return out, rows.Err()
}

func overviewSymbols(db *sql.DB, limit int) ([]map[string]any, error) {
	rows, err := db.Query(`SELECT n.kind,n.name,n.qualified_name,n.file_path,n.start_line,n.is_exported,
		(SELECT COUNT(*) FROM edges WHERE source = n.id AND resolved = 1) AS out_degree,
		(SELECT COUNT(*) FROM edges WHERE target = n.id AND resolved = 1) AS in_degree
		FROM nodes n
		WHERE n.kind IN ('function','method','class','struct','interface')
		AND n.file_path NOT LIKE '%_test.go'
		ORDER BY (out_degree + in_degree) DESC, n.is_exported DESC, n.file_path, n.start_line
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var kind, name, qn, file string
		var line, exported, outDegree, inDegree int
		if rows.Scan(&kind, &name, &qn, &file, &line, &exported, &outDegree, &inDegree) != nil {
			continue
		}
		out = append(out, map[string]any{
			"kind":           kind,
			"name":           name,
			"qualified_name": qn,
			"file_path":      file,
			"start_line":     line,
			"is_exported":    exported,
			"in_degree":      inDegree,
			"out_degree":     outDegree,
			"degree":         inDegree + outDegree,
		})
	}
	return out, rows.Err()
}

func overviewDependencies(db *sql.DB, limit int) ([]map[string]any, error) {
	rows, err := db.Query(`SELECT sn.file_path, tn.file_path, e.kind
		FROM edges e JOIN nodes sn ON sn.id = e.source JOIN nodes tn ON tn.id = e.target
		WHERE e.resolved = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type dep struct {
		source string
		target string
		kind   string
		count  int
	}
	byKey := map[string]*dep{}
	for rows.Next() {
		var sourceFile, targetFile, kind string
		if rows.Scan(&sourceFile, &targetFile, &kind) != nil {
			continue
		}
		sourcePkg, targetPkg := packagePath(sourceFile), packagePath(targetFile)
		if sourcePkg == targetPkg {
			continue
		}
		key := sourcePkg + "\x00" + targetPkg + "\x00" + kind
		if byKey[key] == nil {
			byKey[key] = &dep{source: sourcePkg, target: targetPkg, kind: kind}
		}
		byKey[key].count++
	}
	list := make([]*dep, 0, len(byKey))
	for _, d := range byKey {
		list = append(list, d)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].count == list[j].count {
			return list[i].source+list[i].target < list[j].source+list[j].target
		}
		return list[i].count > list[j].count
	})
	if len(list) > limit {
		list = list[:limit]
	}
	out := make([]map[string]any, 0, len(list))
	for _, d := range list {
		out = append(out, map[string]any{"source": d.source, "target": d.target, "kind": d.kind, "count": d.count})
	}
	return out, rows.Err()
}

func overviewStorage(db *sql.DB) []map[string]any {
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var name string
		if rows.Scan(&name) != nil {
			continue
		}
		if isFTSShadowTable(name) {
			continue
		}
		out = append(out, map[string]any{"table": name, "rows": storepkg.CountSQL(db, "SELECT COUNT(*) FROM "+name)})
	}
	return out
}

func isFTSShadowTable(name string) bool {
	for _, suffix := range []string{"_config", "_data", "_docsize", "_idx"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func packagePath(file string) string {
	dir := filepath.ToSlash(filepath.Dir(file))
	if dir == "." || dir == "/" {
		return "."
	}
	return strings.TrimSuffix(dir, "/")
}

func FormatOverviewMarkdown(payload map[string]any) string {
	lines := []string{"# Codegraph Overview"}
	if summary, _ := payload["summary"].(map[string]any); len(summary) > 0 {
		lines = append(lines, "", "## Summary")
		lines = append(lines, fmt.Sprintf("- files=%v nodes=%v edges=%v resolved_edges=%v", summary["files"], summary["nodes"], summary["edges"], summary["resolved_edges"]))
	}
	if packages, _ := payload["packages"].([]map[string]any); len(packages) > 0 {
		lines = append(lines, "", "## Packages")
		for _, pkg := range packages {
			lines = append(lines, fmt.Sprintf("- `%v`: files=%v nodes=%v", pkg["path"], pkg["files"], pkg["nodes"]))
		}
	}
	if symbols, _ := payload["core_symbols"].([]map[string]any); len(symbols) > 0 {
		lines = append(lines, "", "## Core Symbols")
		for _, sym := range symbols {
			lines = append(lines, fmt.Sprintf("- `%v` (%v) degree=%v %v:%v", sym["qualified_name"], sym["kind"], sym["degree"], sym["file_path"], sym["start_line"]))
		}
	}
	if deps, _ := payload["package_dependencies"].([]map[string]any); len(deps) > 0 {
		lines = append(lines, "", "## Package Dependencies")
		for _, dep := range deps {
			lines = append(lines, fmt.Sprintf("- `%v` -> `%v` (%v, %v)", dep["source"], dep["target"], dep["kind"], dep["count"]))
		}
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}
