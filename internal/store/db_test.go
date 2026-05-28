package store_test

import (
	storepkg "github.com/DreamCats/codegraph-cli/internal/store"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMetadataRoundtripAndOldSchemaMigration(t *testing.T) {
	dir := t.TempDir()
	raw, err := sql.Open("sqlite", storepkg.DBPathFor(dir))
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.Exec(`
	CREATE TABLE schema_versions (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL, description TEXT);
	INSERT INTO schema_versions(version, applied_at, description) VALUES (1, 1, 'old schema');
	CREATE TABLE nodes (
		id TEXT PRIMARY KEY, kind TEXT NOT NULL, name TEXT NOT NULL, qualified_name TEXT NOT NULL,
		file_path TEXT NOT NULL, language TEXT NOT NULL, start_line INTEGER NOT NULL, end_line INTEGER NOT NULL,
		start_column INTEGER NOT NULL, end_column INTEGER NOT NULL, docstring TEXT, signature TEXT,
		is_exported INTEGER DEFAULT 0, is_async INTEGER DEFAULT 0, is_static INTEGER DEFAULT 0,
		updated_at INTEGER NOT NULL
	);
	CREATE TABLE edges (
		id INTEGER PRIMARY KEY AUTOINCREMENT, source TEXT NOT NULL, target TEXT NOT NULL, kind TEXT NOT NULL,
		line INTEGER, col INTEGER, metadata TEXT
	);
	CREATE TABLE files (
		path TEXT PRIMARY KEY, content_hash TEXT NOT NULL, language TEXT NOT NULL, size INTEGER NOT NULL,
		modified_at INTEGER NOT NULL, indexed_at INTEGER NOT NULL, node_count INTEGER DEFAULT 0, errors TEXT
	);
	CREATE TABLE project_metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at INTEGER NOT NULL);
	`)
	if err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()

	db, err := storepkg.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := storepkg.SetMeta(db, "k", "v"); err != nil {
		t.Fatal(err)
	}
	if got := storepkg.GetMeta(db, "k"); got != "v" {
		t.Fatalf("metadata = %q", got)
	}
	nodeCols := tableCols(t, db, "nodes")
	edgeCols := tableCols(t, db, "edges")
	if !nodeCols["import_module"] || !nodeCols["import_symbol"] || !edgeCols["resolved"] {
		t.Fatalf("missing migrated columns: nodes=%#v edges=%#v", nodeCols, edgeCols)
	}
	if got := storepkg.CountSQL(db, "SELECT MAX(version) FROM schema_versions"); got != 3 {
		t.Fatalf("schema version = %d", got)
	}
	if _, err := db.Exec(`INSERT INTO nodes(id,kind,name,qualified_name,file_path,language,start_line,end_line,start_column,end_column,updated_at) VALUES('id','function','name','name','a.py','python',1,1,0,0,1)`); err != nil {
		t.Fatal(err)
	}
	if got := storepkg.CountSQL(db, "SELECT COUNT(*) FROM nodes_fts"); got != 1 {
		t.Fatalf("fts rows = %d", got)
	}
}

func tableCols(t *testing.T, db *sql.DB, table string) map[string]bool {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		out[name] = true
	}
	return out
}
