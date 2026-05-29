package store

import (
	"database/sql"
	"github.com/DreamCats/codegraph-cli/internal/model"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

func DBPathFor(store string) string { return filepath.Join(store, model.DBFileName) }

func Open(store string) (*sql.DB, error) {
	if err := os.MkdirAll(store, 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", DBPathFor(store))
	if err != nil {
		return nil, err
	}
	for _, stmt := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA synchronous = NORMAL",
	} {
		_, _ = db.Exec(stmt)
	}
	if err := ensureSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func ensureSchema(db *sql.DB) error {
	schema := []string{
		`CREATE TABLE IF NOT EXISTS schema_versions (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL, description TEXT)`,
		`CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY, kind TEXT NOT NULL, name TEXT NOT NULL, qualified_name TEXT NOT NULL,
			file_path TEXT NOT NULL, language TEXT NOT NULL, start_line INTEGER NOT NULL, end_line INTEGER NOT NULL,
			start_column INTEGER NOT NULL, end_column INTEGER NOT NULL, docstring TEXT, signature TEXT,
			is_exported INTEGER DEFAULT 0, is_async INTEGER DEFAULT 0, is_static INTEGER DEFAULT 0,
			import_module TEXT, import_symbol TEXT, updated_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS edges (
			id INTEGER PRIMARY KEY AUTOINCREMENT, source TEXT NOT NULL, target TEXT NOT NULL, kind TEXT NOT NULL,
			resolved INTEGER NOT NULL DEFAULT 0, line INTEGER, col INTEGER, metadata TEXT,
			FOREIGN KEY (source) REFERENCES nodes(id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS files (
			path TEXT PRIMARY KEY, content_hash TEXT NOT NULL, language TEXT NOT NULL, size INTEGER NOT NULL,
			modified_at INTEGER NOT NULL, indexed_at INTEGER NOT NULL, node_count INTEGER DEFAULT 0, errors TEXT)`,
		`CREATE TABLE IF NOT EXISTS project_metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at INTEGER NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_kind ON nodes(kind)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_name ON nodes(name)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_qname ON nodes(qualified_name)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_file ON nodes(file_path)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_lower_name ON nodes(lower(name))`,
		`CREATE INDEX IF NOT EXISTS idx_edges_kind ON edges(kind)`,
		`CREATE INDEX IF NOT EXISTS idx_edges_source_kind ON edges(source, kind)`,
		`CREATE INDEX IF NOT EXISTS idx_edges_target_kind ON edges(target, kind)`,
		`CREATE INDEX IF NOT EXISTS idx_files_language ON files(language)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS nodes_fts USING fts5(
			id, name, qualified_name, docstring, signature,
			content='nodes', content_rowid='rowid')`,
		`CREATE TRIGGER IF NOT EXISTS nodes_ai AFTER INSERT ON nodes BEGIN
			INSERT INTO nodes_fts(rowid, id, name, qualified_name, docstring, signature)
			VALUES (NEW.rowid, NEW.id, NEW.name, NEW.qualified_name, NEW.docstring, NEW.signature);
		END`,
		`CREATE TRIGGER IF NOT EXISTS nodes_ad AFTER DELETE ON nodes BEGIN
			INSERT INTO nodes_fts(nodes_fts, rowid, id, name, qualified_name, docstring, signature)
			VALUES ('delete', OLD.rowid, OLD.id, OLD.name, OLD.qualified_name, OLD.docstring, OLD.signature);
		END`,
		`CREATE TRIGGER IF NOT EXISTS nodes_au AFTER UPDATE ON nodes BEGIN
			INSERT INTO nodes_fts(nodes_fts, rowid, id, name, qualified_name, docstring, signature)
			VALUES ('delete', OLD.rowid, OLD.id, OLD.name, OLD.qualified_name, OLD.docstring, OLD.signature);
			INSERT INTO nodes_fts(rowid, id, name, qualified_name, docstring, signature)
			VALUES (NEW.rowid, NEW.id, NEW.name, NEW.qualified_name, NEW.docstring, NEW.signature);
		END`,
	}
	for _, s := range schema {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	_ = ensureColumn(db, "nodes", "import_module", "TEXT")
	_ = ensureColumn(db, "nodes", "import_symbol", "TEXT")
	_ = ensureColumn(db, "edges", "resolved", "INTEGER NOT NULL DEFAULT 0")
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_nodes_import_module ON nodes(import_module) WHERE import_module IS NOT NULL`)
	_, _ = db.Exec(`INSERT INTO nodes_fts(nodes_fts) VALUES ('rebuild')`)
	_, _ = db.Exec(`INSERT OR IGNORE INTO schema_versions(version, applied_at, description) VALUES(1, ?, 'codegraph-cli initial schema')`, NowMS())
	_, _ = db.Exec(`INSERT OR IGNORE INTO schema_versions(version, applied_at, description) VALUES(2, ?, 'add import metadata columns and edge resolution state')`, NowMS())
	_, _ = db.Exec(`INSERT OR IGNORE INTO schema_versions(version, applied_at, description) VALUES(3, ?, 'go rewrite schema compatible')`, NowMS())
	return nil
}

func ensureColumn(db *sql.DB, table, col, typ string) error {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == col {
			return nil
		}
	}
	_, err = db.Exec("ALTER TABLE " + table + " ADD COLUMN " + col + " " + typ)
	return err
}

func NowMS() int64 { return time.Now().UnixMilli() }

func SetMeta(db *sql.DB, key, value string) error {
	_, err := db.Exec(`INSERT INTO project_metadata(key,value,updated_at) VALUES(?,?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, key, value, NowMS())
	return err
}

func GetMeta(db *sql.DB, key string) string {
	var v string
	if db.QueryRow(`SELECT value FROM project_metadata WHERE key = ?`, key).Scan(&v) != nil {
		return ""
	}
	return v
}

func CountSQL(db *sql.DB, q string, args ...any) int {
	var n int
	_ = db.QueryRow(q, args...).Scan(&n)
	return n
}
