package indexer

import (
	"github.com/DreamCats/codegraph-cli/internal/config"
	"github.com/DreamCats/codegraph-cli/internal/extract"
	"github.com/DreamCats/codegraph-cli/internal/model"
	"github.com/DreamCats/codegraph-cli/internal/resolver"
	storepkg "github.com/DreamCats/codegraph-cli/internal/store"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var defaultIgnores = map[string]bool{
	".git": true, ".hg": true, ".svn": true, "node_modules": true, ".venv": true,
	"venv": true, "env": true, "__pycache__": true, ".mypy_cache": true,
	".ruff_cache": true, ".pytest_cache": true, "dist": true, "build": true,
	".tox": true, ".idea": true, ".vscode": true,
}

func IterSourceFiles(root string) ([]string, error) {
	patterns := loadGitignore(root)
	out := []string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == root {
			return nil
		}
		rel := relPosix(path, root)
		if d.IsDir() {
			name := d.Name()
			if defaultIgnores[name] || strings.HasPrefix(name, ".") || matchesGitignore(rel+"/", patterns) {
				return filepath.SkipDir
			}
			return nil
		}
		if matchesGitignore(rel, patterns) {
			return nil
		}
		if extract.DetectLanguage(rel) != "" {
			out = append(out, rel)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

func loadGitignore(root string) []string {
	raw, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return nil
	}
	out := []string{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		out = append(out, line)
	}
	return out
}

func matchesGitignore(rel string, patterns []string) bool {
	for _, p := range patterns {
		n := strings.Trim(p, "/")
		if n == "" {
			continue
		}
		if strings.HasSuffix(p, "/") && (rel == n || strings.HasPrefix(rel, n+"/")) {
			return true
		}
		if !strings.Contains(n, "/") {
			for _, part := range strings.Split(strings.TrimSuffix(rel, "/"), "/") {
				if ok, _ := filepath.Match(n, part); ok {
					return true
				}
			}
		}
		if ok, _ := filepath.Match(n, rel); ok {
			return true
		}
	}
	return false
}

func relPosix(path, root string) string {
	rel, err := filepath.Rel(config.Abs(root), config.Abs(path))
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func contentHash(data []byte) string {
	h := sha1.Sum(data)
	return hex.EncodeToString(h[:])
}

func IndexAll(root, storeDir string, force bool) (model.IndexStats, error) {
	root = config.Abs(root)
	stats := model.IndexStats{Errors: []string{}}
	files, err := IterSourceFiles(root)
	if err != nil {
		return stats, err
	}
	current := map[string]bool{}
	for _, f := range files {
		current[f] = true
	}
	db, err := storepkg.Open(storeDir)
	if err != nil {
		return stats, err
	}
	defer db.Close()
	hasResolved := storepkg.GetMeta(db, "last_resolved_at") != ""
	tx, err := db.Begin()
	if err != nil {
		return stats, err
	}
	cached := map[string]string{}
	if !force {
		rows, _ := tx.Query(`SELECT path, content_hash FROM files`)
		if rows != nil {
			for rows.Next() {
				var p, h string
				_ = rows.Scan(&p, &h)
				cached[p] = h
			}
			rows.Close()
		}
	}
	for p := range cached {
		if !current[p] {
			_ = deleteFileRecords(tx, p)
			stats.FilesDeleted++
		}
	}
	for _, rel := range files {
		stats.FilesScanned++
		absPath := filepath.Join(root, filepath.FromSlash(rel))
		st, err := os.Stat(absPath)
		if err != nil {
			stats.FilesFailed++
			stats.Errors = append(stats.Errors, rel+": read failed: "+err.Error())
			continue
		}
		if st.Size() > model.MaxFileSize {
			stats.FilesFailed++
			stats.Errors = append(stats.Errors, fmt.Sprintf("%s: size exceeded (%d > %d)", rel, st.Size(), model.MaxFileSize))
			continue
		}
		data, err := os.ReadFile(absPath)
		if err != nil {
			stats.FilesFailed++
			stats.Errors = append(stats.Errors, rel+": read failed: "+err.Error())
			continue
		}
		h := contentHash(data)
		if !force && cached[rel] == h {
			stats.FilesSkipped++
			continue
		}
		res := extract.ExtractFile(rel, data)
		if err := deleteFileRecords(tx, rel); err != nil {
			_ = tx.Rollback()
			return stats, err
		}
		n, err := insertNodes(tx, res.Nodes)
		if err != nil {
			_ = tx.Rollback()
			return stats, err
		}
		e, err := insertEdges(tx, res.Edges)
		if err != nil {
			_ = tx.Rollback()
			return stats, err
		}
		errJSON := nullableJSON(res.Errors)
		_, err = tx.Exec(`INSERT OR REPLACE INTO files(path,content_hash,language,size,modified_at,indexed_at,node_count,errors)
			VALUES(?,?,?,?,?,?,?,?)`, rel, h, extract.DetectLanguage(rel), len(data), st.ModTime().UnixMilli(), storepkg.NowMS(), n, errJSON)
		if err != nil {
			_ = tx.Rollback()
			return stats, err
		}
		stats.FilesIndexed++
		stats.Nodes += n
		stats.Edges += e
	}
	if _, err := tx.Exec(`INSERT INTO project_metadata(key,value,updated_at) VALUES('last_indexed',?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, strconv.FormatInt(storepkg.NowMS(), 10), storepkg.NowMS()); err != nil {
		_ = tx.Rollback()
		return stats, err
	}
	if _, err := tx.Exec(`INSERT INTO project_metadata(key,value,updated_at) VALUES('project_root',?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, root, storepkg.NowMS()); err != nil {
		_ = tx.Rollback()
		return stats, err
	}
	if err := tx.Commit(); err != nil {
		return stats, err
	}
	writeErrorLog(storeDir, stats.Errors)
	var r model.ResolveStats
	if force || stats.FilesIndexed > 0 || stats.FilesDeleted > 0 || !hasResolved {
		r, err = resolver.ResolveAll(storeDir)
	} else {
		r, err = resolver.ResolutionSnapshot(storeDir)
	}
	if err != nil {
		return stats, err
	}
	stats.EdgesResolved = r.EdgesResolvedNow
	stats.EdgesStillUnresolved = r.EdgesStillUnresolved
	stats.ErrorsTotal = len(stats.Errors)
	return stats, nil
}

func nullableJSON(v any) any {
	raw, _ := json.Marshal(v)
	if string(raw) == "[]" || string(raw) == "null" {
		return nil
	}
	return string(raw)
}

func deleteFileRecords(tx *sql.Tx, path string) error {
	_, _ = tx.Exec(`DELETE FROM edges WHERE source IN (SELECT id FROM nodes WHERE file_path = ?) OR target IN (SELECT id FROM nodes WHERE file_path = ?)`, path, path)
	if _, err := tx.Exec(`DELETE FROM nodes WHERE file_path = ?`, path); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM files WHERE path = ?`, path)
	return err
}

func insertNodes(tx *sql.Tx, nodes []model.SymbolNode) (int, error) {
	for _, n := range nodes {
		_, err := tx.Exec(`INSERT OR REPLACE INTO nodes(
			id,kind,name,qualified_name,file_path,language,start_line,end_line,start_column,end_column,
			docstring,signature,is_exported,is_async,is_static,import_module,import_symbol,updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			n.ID, n.Kind, n.Name, n.QualifiedName, n.FilePath, n.Language, n.StartLine, n.EndLine,
			n.StartColumn, n.EndColumn, nullableString(n.Docstring), nullableString(n.Signature),
			boolInt(n.IsExported), boolInt(n.IsAsync), boolInt(n.IsStatic),
			nullableString(n.ImportModule), nullableString(n.ImportSymbol), storepkg.NowMS())
		if err != nil {
			return 0, err
		}
	}
	return len(nodes), nil
}

func insertEdges(tx *sql.Tx, edges []model.EdgeRecord) (int, error) {
	for _, e := range edges {
		meta := any(nil)
		if len(e.Metadata) > 0 {
			raw, _ := json.Marshal(e.Metadata)
			meta = string(raw)
		}
		resolved := 1
		if strings.HasPrefix(e.Target, "name:") {
			resolved = 0
		}
		_, err := tx.Exec(`INSERT INTO edges(source,target,kind,resolved,line,col,metadata) VALUES(?,?,?,?,?,?,?)`,
			e.Source, e.Target, e.Kind, resolved, nullableInt(e.Line), nullableInt(e.Col), meta)
		if err != nil {
			return 0, err
		}
	}
	return len(edges), nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func nullableString(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}
func nullableInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func writeErrorLog(store string, errors []string) {
	p := filepath.Join(store, "errors.log")
	if len(errors) == 0 {
		_ = os.Remove(p)
		return
	}
	_ = os.WriteFile(p, []byte(strings.Join(errors, "\n")+"\n"), 0o644)
}

func stableID(file, q string, line int) string { return fmt.Sprintf("%s::%s::%d", file, q, line) }
func strPtr(s string) *string                  { return &s }
func intPtr(i int) *int                        { return &i }
