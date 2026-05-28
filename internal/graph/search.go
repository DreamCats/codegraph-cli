package graph

import (
	storepkg "github.com/DreamCats/codegraph-cli/internal/store"
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

func Search(store, query, kind string, limit int) ([]map[string]any, error) {
	db, err := storepkg.Open(store)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if rows, err := searchFTS(db, query, kind, limit); err == nil && len(rows) > 0 {
		return rows, nil
	}
	return searchLike(db, query, kind, limit)
}

func searchFTS(db *sql.DB, query, kind string, limit int) ([]map[string]any, error) {
	fts := ftsQuery(query)
	if fts == "" {
		return nil, nil
	}
	q := `SELECT n.id,n.kind,n.name,n.qualified_name,n.file_path,n.language,
		n.start_line,n.end_line,n.signature,n.docstring,n.is_exported,
		abs(bm25(nodes_fts, 0, 20, 5, 1, 2)) AS score
		FROM nodes_fts JOIN nodes n ON nodes_fts.id = n.id
		WHERE nodes_fts MATCH ?`
	args := []any{fts}
	if kind != "" {
		q += " AND n.kind = ?"
		args = append(args, kind)
	}
	q += ` ORDER BY CASE WHEN n.name = ? THEN 0 WHEN n.name LIKE ? THEN 1 ELSE 2 END,
		score, n.name LIMIT ?`
	args = append(args, query, query+"%", limit)
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		m, err := scanNodeSearch(rows, true)
		if err == nil {
			out = append(out, m)
		}
	}
	rankRows(out, query)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, rows.Err()
}

func ftsQuery(query string) string {
	query = strings.ReplaceAll(query, "::", " ")
	terms := []string{}
	for _, term := range strings.Fields(query) {
		term = strings.NewReplacer(`"`, "", "*", "", ":", "").Replace(term)
		if term != "" {
			terms = append(terms, `"`+term+`"*`)
		}
	}
	return strings.Join(terms, " OR ")
}

func searchLike(db *sql.DB, query, kind string, limit int) ([]map[string]any, error) {
	like := "%" + query + "%"
	lower := "%" + strings.ToLower(query) + "%"
	q := `SELECT id,kind,name,qualified_name,file_path,language,start_line,end_line,
		signature,docstring,is_exported FROM nodes
		WHERE (name LIKE ? OR qualified_name LIKE ? OR lower(name) LIKE ? OR lower(docstring) LIKE ? OR lower(signature) LIKE ?)`
	args := []any{like, like, lower, lower, lower}
	if kind != "" {
		q += " AND kind = ?"
		args = append(args, kind)
	}
	q += " ORDER BY CASE WHEN name = ? THEN 0 WHEN name LIKE ? THEN 1 ELSE 2 END, name LIMIT ?"
	args = append(args, query, query+"%", limit)
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		m, err := scanNodeSearch(rows, false)
		if err == nil {
			out = append(out, m)
		}
	}
	rankRows(out, query)
	return out, rows.Err()
}

func scanNodeSearch(rows *sql.Rows, withScore bool) (map[string]any, error) {
	var id, kind, name, qn, fp, lang string
	var start, end int
	var sig, doc sql.NullString
	var exported int
	var score sql.NullFloat64
	var err error
	if withScore {
		err = rows.Scan(&id, &kind, &name, &qn, &fp, &lang, &start, &end, &sig, &doc, &exported, &score)
	} else {
		err = rows.Scan(&id, &kind, &name, &qn, &fp, &lang, &start, &end, &sig, &doc, &exported)
	}
	m := map[string]any{
		"id": id, "kind": kind, "name": name, "qualified_name": qn,
		"file_path": fp, "language": lang, "start_line": start,
		"end_line": end, "is_exported": exported,
	}
	if sig.Valid {
		m["signature"] = sig.String
	}
	if doc.Valid {
		m["docstring"] = doc.String
	}
	if score.Valid {
		m["score"] = score.Float64
	}
	return m, err
}

func rankRows(rows []map[string]any, query string) {
	lq := strings.ToLower(query)
	for _, r := range rows {
		name := strings.ToLower(fmt.Sprint(r["name"]))
		qn := strings.ToLower(fmt.Sprint(r["qualified_name"]))
		score := 0.0
		if s, ok := r["score"].(float64); ok {
			score += s
		}
		switch {
		case name == lq:
			score += 100
		case strings.HasPrefix(name, lq):
			score += 70
		case strings.Contains(name, lq):
			score += 40
		case strings.Contains(qn, lq):
			score += 20
		}
		switch r["kind"] {
		case "function", "method", "class":
			score += 10
		case "struct", "interface":
			score += 8
		case "import":
			score -= 40
		}
		r["rank"] = score
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ri := rows[i]["rank"].(float64)
		rj := rows[j]["rank"].(float64)
		if ri == rj {
			return fmt.Sprint(rows[i]["name"]) < fmt.Sprint(rows[j]["name"])
		}
		return ri > rj
	})
}

func ListFiles(store string, patterns []string) ([]map[string]any, error) {
	db, err := storepkg.Open(store)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT path, language, size, node_count, indexed_at FROM files ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var p, lang string
		var size, nodes int
		var indexed int64
		if rows.Scan(&p, &lang, &size, &nodes, &indexed) != nil {
			continue
		}
		if len(patterns) > 0 && !matchAny(patterns, p) {
			continue
		}
		out = append(out, map[string]any{"path": p, "language": lang, "size": size, "node_count": nodes, "indexed_at": indexed})
	}
	return out, rows.Err()
}

func matchAny(patterns []string, p string) bool {
	for _, pat := range patterns {
		if ok, _ := filepath.Match(filepath.FromSlash(pat), filepath.FromSlash(p)); ok {
			return true
		}
		if ok, _ := filepath.Match(pat, p); ok {
			return true
		}
	}
	return false
}
