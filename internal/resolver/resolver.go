package resolver

import (
	"bufio"
	"github.com/DreamCats/codegraph-cli/internal/extract"
	"github.com/DreamCats/codegraph-cli/internal/model"
	storepkg "github.com/DreamCats/codegraph-cli/internal/store"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type importInfo struct {
	Module string
	Symbol *string
	Local  string
}

type pathAlias struct {
	Pattern string
	Target  string
}

func strPtr(s string) *string { return &s }

type resolver struct {
	db          *sql.DB
	projectRoot string
	goModule    string
	aliases     []pathAlias
	byFile      map[string]map[string][]string
	global      map[string][]struct{ file, id string }
	imports     map[string][]importInfo
	nodeFile    map[string]string
	goPkgCache  map[string]string
}

func newResolver(db *sql.DB) (*resolver, error) {
	r := &resolver{db: db, projectRoot: storepkg.GetMeta(db, "project_root"), byFile: map[string]map[string][]string{}, global: map[string][]struct{ file, id string }{}, imports: map[string][]importInfo{}, nodeFile: map[string]string{}, goPkgCache: map[string]string{}}
	r.goModule = loadGoModule(r.projectRoot)
	r.aliases = loadPathAliases(r.projectRoot)
	rows, err := db.Query(`SELECT id,name,file_path,kind,is_exported,import_module,import_symbol FROM nodes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name, file, kind string
		var exp int
		var mod, sym sql.NullString
		if rows.Scan(&id, &name, &file, &kind, &exp, &mod, &sym) != nil {
			continue
		}
		r.nodeFile[id] = file
		if kind == "import" {
			var sp *string
			if sym.Valid {
				sp = strPtr(sym.String)
			}
			m := ""
			if mod.Valid {
				m = mod.String
			}
			r.imports[file] = append(r.imports[file], importInfo{Module: m, Symbol: sp, Local: name})
			continue
		}
		if r.byFile[file] == nil {
			r.byFile[file] = map[string][]string{}
		}
		r.byFile[file][name] = append(r.byFile[file][name], id)
		if exp == 1 {
			r.global[name] = append(r.global[name], struct{ file, id string }{file, id})
		}
	}
	return r, nil
}

func loadPathAliases(root string) []pathAlias {
	if root == "" {
		return nil
	}
	out := []pathAlias{}
	for _, name := range []string{"tsconfig.json", "jsconfig.json"} {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		var data struct {
			CompilerOptions struct {
				Paths map[string][]string `json:"paths"`
			} `json:"compilerOptions"`
		}
		if json.Unmarshal(raw, &data) != nil {
			continue
		}
		for pattern, targets := range data.CompilerOptions.Paths {
			if len(targets) == 0 {
				continue
			}
			out = append(out, pathAlias{Pattern: pattern, Target: targets[0]})
		}
	}
	return out
}

func loadGoModule(root string) string {
	if root == "" {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	sc := bufio.NewScanner(strings.NewReader(string(raw)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.Fields(line)[1]
		}
	}
	return ""
}

func (r *resolver) resolveOne(sourceID, name, qualifier string) string {
	sourceFile := r.nodeFile[sourceID]
	if sourceFile == "" {
		return ""
	}
	if hit := first(r.byFile[sourceFile][name]); hit != "" {
		return hit
	}
	if strings.HasSuffix(sourceFile, ".go") {
		dir := pathDir(sourceFile)
		for file, names := range r.byFile {
			if file != sourceFile && strings.HasSuffix(file, ".go") && pathDir(file) == dir {
				if hit := first(names[name]); hit != "" {
					return hit
				}
			}
		}
	}
	wantedViaImport := ""
	for _, im := range r.imports[sourceFile] {
		if qualifier != "" {
			if im.Local != qualifier && !r.goImportPackageMatches(im.Module, qualifier, sourceFile) {
				continue
			}
		} else if im.Local != name {
			continue
		}
		wanted := name
		if qualifier == "" && im.Symbol != nil && *im.Symbol != "default" && *im.Symbol != "*" {
			wanted = *im.Symbol
		}
		wantedViaImport = wanted
		for _, cand := range r.moduleCandidates(im.Module, sourceFile) {
			if hit := r.candidateHit(cand, wanted); hit != "" {
				return hit
			}
		}
	}
	for _, im := range r.imports[sourceFile] {
		if im.Symbol == nil || *im.Symbol != "*" {
			continue
		}
		for _, cand := range r.moduleCandidates(im.Module, sourceFile) {
			if hit := r.candidateHit(cand, name); hit != "" {
				return hit
			}
		}
	}
	for _, cand := range []string{wantedViaImport, name} {
		if cand == "" {
			continue
		}
		g := r.global[cand]
		if len(g) == 1 {
			return g[0].id
		}
	}
	return ""
}

func first(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return v[0]
}

func pathDir(p string) string {
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return ""
	}
	return p[:i]
}

func (r *resolver) moduleCandidates(module, sourceFile string) []string {
	sourceDir := pathDir(sourceFile)
	bases := []string{}
	switch {
	case strings.HasPrefix(module, "./") || strings.HasPrefix(module, "../"):
		bases = append(bases, cleanPosix(sourceDir+"/"+module))
	case strings.HasPrefix(module, "."):
		name := strings.TrimLeft(module, ".")
		if name != "" {
			bases = append(bases, cleanPosix(sourceDir+"/"+strings.ReplaceAll(name, ".", "/")))
		}
	default:
		bases = append(bases, r.applyAliases(module)...)
		if r.goModule != "" && strings.HasPrefix(module, r.goModule+"/") {
			bases = append(bases, strings.TrimPrefix(module, r.goModule+"/"))
		}
		bases = append(bases, strings.ReplaceAll(module, ".", "/"))
	}
	out := []string{}
	for _, b := range bases {
		if hasSourceExt(b) {
			out = append(out, b)
		} else {
			for ext := range extract.SupportedExtensions() {
				out = append(out, b+ext)
			}
			out = append(out, b+"/*.go", b+"/__init__.py", b+"/index.ts", b+"/index.tsx", b+"/index.js", b+"/index.jsx")
		}
	}
	return unique(out)
}

func (r *resolver) applyAliases(module string) []string {
	out := []string{}
	for _, alias := range r.aliases {
		pattern := alias.Pattern
		target := alias.Target
		if !strings.Contains(pattern, "*") {
			if module == pattern {
				out = append(out, strings.ReplaceAll(target, "*", ""))
			}
			continue
		}
		parts := strings.SplitN(pattern, "*", 2)
		prefix, suffix := parts[0], parts[1]
		if !strings.HasPrefix(module, prefix) {
			continue
		}
		if suffix != "" && !strings.HasSuffix(module, suffix) {
			continue
		}
		middle := strings.TrimPrefix(module, prefix)
		if suffix != "" {
			middle = strings.TrimSuffix(middle, suffix)
		}
		out = append(out, strings.ReplaceAll(target, "*", middle))
	}
	return out
}

func cleanPosix(p string) string {
	parts := []string{}
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			if len(parts) > 0 {
				parts = parts[:len(parts)-1]
			}
			continue
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "/")
}

func hasSourceExt(p string) bool {
	_, ok := extract.SupportedExtensions()[strings.ToLower(filepath.Ext(p))]
	return ok
}

func unique(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, x := range in {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func (r *resolver) candidateHit(candidate, name string) string {
	if strings.HasSuffix(candidate, "/*.go") {
		prefix := strings.TrimSuffix(candidate, "*.go")
		keys := make([]string, 0, len(r.byFile))
		for k := range r.byFile {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, f := range keys {
			if strings.HasPrefix(f, prefix) && strings.HasSuffix(f, ".go") {
				if hit := first(r.byFile[f][name]); hit != "" {
					return hit
				}
			}
		}
		return ""
	}
	return first(r.byFile[candidate][name])
}

func (r *resolver) goImportPackageMatches(module, qualifier, sourceFile string) bool {
	if !strings.HasSuffix(sourceFile, ".go") || r.projectRoot == "" {
		return false
	}
	for _, cand := range r.moduleCandidates(module, sourceFile) {
		dir := strings.TrimSuffix(cand, "/*.go")
		if !strings.HasSuffix(cand, "/*.go") {
			dir = pathDir(cand)
		}
		pkg := r.readGoPackage(filepath.Join(r.projectRoot, filepath.FromSlash(dir)))
		if pkg == qualifier {
			return true
		}
	}
	return false
}

func (r *resolver) readGoPackage(dir string) string {
	if pkg, ok := r.goPkgCache[dir]; ok {
		return pkg
	}
	pkg := readGoPackage(dir)
	r.goPkgCache[dir] = pkg
	return pkg
}

func readGoPackage(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "package ") {
				f := strings.Fields(line)
				if len(f) >= 2 {
					return f[1]
				}
			}
		}
	}
	return ""
}

func ResolveAll(store string) (model.ResolveStats, error) {
	db, err := storepkg.Open(store)
	if err != nil {
		return model.ResolveStats{}, err
	}
	defer db.Close()
	stats := model.ResolveStats{
		EdgesTotal:          storepkg.CountSQL(db, `SELECT COUNT(*) FROM edges`),
		EdgesResolvedBefore: storepkg.CountSQL(db, `SELECT COUNT(*) FROM edges WHERE resolved = 1`),
	}
	r, err := newResolver(db)
	if err != nil {
		return stats, err
	}
	rows, err := db.Query(`SELECT id, source, target, metadata FROM edges WHERE resolved = 0 AND target LIKE 'name:%'`)
	if err != nil {
		return stats, err
	}
	type erow struct {
		id                   int
		source, target, meta string
	}
	list := []erow{}
	for rows.Next() {
		var row erow
		var meta sql.NullString
		if rows.Scan(&row.id, &row.source, &row.target, &meta) == nil {
			if meta.Valid {
				row.meta = meta.String
			}
			list = append(list, row)
		}
	}
	rows.Close()
	tx, err := db.Begin()
	if err != nil {
		return stats, err
	}
	for _, row := range list {
		name := strings.TrimPrefix(row.target, "name:")
		qualifier := ""
		if row.meta != "" {
			var m map[string]any
			if json.Unmarshal([]byte(row.meta), &m) == nil {
				if ct, ok := m["callee_text"].(string); ok && strings.Contains(ct, ".") {
					qualifier = ct[:strings.LastIndex(ct, ".")]
				}
			}
		}
		if hit := r.resolveOne(row.source, name, qualifier); hit != "" {
			if _, err := tx.Exec(`UPDATE edges SET target = ?, resolved = 1 WHERE id = ?`, hit, row.id); err == nil {
				stats.EdgesResolvedNow++
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return stats, err
	}
	stats.EdgesStillUnresolved = storepkg.CountSQL(db, `SELECT COUNT(*) FROM edges WHERE resolved = 0`)
	_ = storepkg.SetMeta(db, "last_resolved_at", strconv.FormatInt(storepkg.NowMS(), 10))
	return stats, nil
}

func ResolutionSnapshot(store string) (model.ResolveStats, error) {
	db, err := storepkg.Open(store)
	if err != nil {
		return model.ResolveStats{}, err
	}
	defer db.Close()
	return model.ResolveStats{
		EdgesTotal:           storepkg.CountSQL(db, `SELECT COUNT(*) FROM edges`),
		EdgesResolvedBefore:  storepkg.CountSQL(db, `SELECT COUNT(*) FROM edges WHERE resolved = 1`),
		EdgesStillUnresolved: storepkg.CountSQL(db, `SELECT COUNT(*) FROM edges WHERE resolved = 0`),
	}, nil
}

func FindNode(store, ref string) ([]map[string]any, error) {
	db, err := storepkg.Open(store)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	for _, q := range []string{
		`SELECT id,kind,name,qualified_name,file_path,language,start_line,end_line FROM nodes WHERE id = ?`,
		`SELECT id,kind,name,qualified_name,file_path,language,start_line,end_line FROM nodes WHERE qualified_name = ?`,
	} {
		rows, err := db.Query(q, ref)
		if err != nil {
			return nil, err
		}
		out := scanNodeRows(rows)
		if len(out) > 0 {
			return out, nil
		}
	}
	rows, err := db.Query(`SELECT id,kind,name,qualified_name,file_path,language,start_line,end_line FROM nodes WHERE name = ? AND kind != 'import' ORDER BY is_exported DESC, file_path, start_line`, ref)
	if err != nil {
		return nil, err
	}
	return scanNodeRows(rows), nil
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

func ResolvedEdgesFor(store, sourceID, targetID, kind string) ([]map[string]any, error) {
	db, err := storepkg.Open(store)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	q := `SELECT e.id,e.source,e.target,e.kind,e.line,e.col,
		sn.name AS source_name,sn.file_path AS source_file,
		tn.name AS target_name,tn.file_path AS target_file,tn.kind AS target_kind,
		tn.qualified_name AS target_qname,tn.start_line AS target_line
		FROM edges e JOIN nodes sn ON sn.id=e.source JOIN nodes tn ON tn.id=e.target WHERE e.resolved=1`
	args := []any{}
	if sourceID != "" {
		q += " AND e.source = ?"
		args = append(args, sourceID)
	}
	if targetID != "" {
		q += " AND e.target = ?"
		args = append(args, targetID)
	}
	if kind != "" {
		q += " AND e.kind = ?"
		args = append(args, kind)
	}
	q += " ORDER BY e.id"
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, line, col sql.NullInt64
		var source, target, k, sn, sf, tn, tf, tk, tq string
		var tl int
		if rows.Scan(&id, &source, &target, &k, &line, &col, &sn, &sf, &tn, &tf, &tk, &tq, &tl) == nil {
			out = append(out, map[string]any{"id": id.Int64, "source": source, "target": target, "kind": k, "line": nullInt(line), "col": nullInt(col), "source_name": sn, "source_file": sf, "target_name": tn, "target_file": tf, "target_kind": tk, "target_qname": tq, "target_line": tl})
		}
	}
	return out, nil
}

func nullInt(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return int(v.Int64)
}
