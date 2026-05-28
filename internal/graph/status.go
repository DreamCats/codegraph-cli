package graph

import (
	"github.com/DreamCats/codegraph-cli/internal/indexer"
	storepkg "github.com/DreamCats/codegraph-cli/internal/store"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
)

func Status(store string) (map[string]any, error) {
	if _, err := os.Stat(storepkg.DBPathFor(store)); err != nil {
		return map[string]any{"initialized": false, "store": store}, nil
	}
	db, err := storepkg.Open(store)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	out := map[string]any{
		"initialized": true, "store": store,
		"files":             storepkg.CountSQL(db, "SELECT COUNT(*) FROM files"),
		"nodes":             storepkg.CountSQL(db, "SELECT COUNT(*) FROM nodes"),
		"edges":             storepkg.CountSQL(db, "SELECT COUNT(*) FROM edges"),
		"schema_version":    storepkg.CountSQL(db, "SELECT MAX(version) FROM schema_versions"),
		"last_indexed":      nullableMeta(db, "last_indexed"),
		"nodes_by_kind":     groupedCount(db, "SELECT kind, COUNT(*) FROM nodes GROUP BY kind"),
		"files_by_language": groupedCount(db, "SELECT language, COUNT(*) FROM files GROUP BY language"),
	}
	return out, nil
}

func nullableMeta(db *sql.DB, k string) any {
	v := storepkg.GetMeta(db, k)
	if v == "" {
		return nil
	}
	return v
}

func groupedCount(db *sql.DB, q string) map[string]int {
	out := map[string]int{}
	rows, err := db.Query(q)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var n int
		if rows.Scan(&k, &n) == nil {
			out[k] = n
		}
	}
	return out
}

func AttachStale(payload map[string]any, root, store string) map[string]any {
	stale := CheckStaleness(root, store)
	payload["stale"] = stale
	return stale
}

func CheckStaleness(root, store string) map[string]any {
	cmd := "codegraph index --path " + root + " --quiet"
	base := map[string]any{"is_stale": false, "reason": nil, "last_indexed": nil, "latest_source_mtime": nil, "latest_source_file": nil, "command": cmd, "retry": "rerun current command after indexing"}
	db, err := storepkg.Open(store)
	if err != nil {
		base["is_stale"] = true
		base["reason"] = "staleness_check_failed: " + err.Error()
		return base
	}
	raw := storepkg.GetMeta(db, "last_indexed")
	db.Close()
	if raw == "" {
		base["is_stale"] = true
		base["reason"] = "never_indexed"
		return base
	}
	base["last_indexed"] = raw
	last, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		base["is_stale"] = true
		base["reason"] = "invalid_last_indexed"
		return base
	}
	latest := int64(0)
	latestFile := ""
	files, _ := indexer.IterSourceFiles(root)
	for _, f := range files {
		if st, err := os.Stat(filepath.Join(root, filepath.FromSlash(f))); err == nil {
			ms := st.ModTime().UnixMilli()
			if ms > latest {
				latest = ms
				latestFile = f
			}
		}
	}
	if latest > 0 {
		base["latest_source_mtime"] = latest
		base["latest_source_file"] = latestFile
	}
	if latest > last {
		base["is_stale"] = true
		base["reason"] = "source file modified after last_indexed"
	}
	return base
}
