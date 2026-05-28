package indexer_test

import (
	"github.com/DreamCats/codegraph-cli/internal/graph"
	"github.com/DreamCats/codegraph-cli/internal/indexer"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeProject(t *testing.T, root string) {
	writeFile(t, filepath.Join(root, "pkg", "__init__.py"), "")
	writeFile(t, filepath.Join(root, "pkg", "a.py"), `"""module a."""
class Service:
    """needle service docs."""
    def serve(self):
        return helper()

def helper():
    return 42
`)
	writeFile(t, filepath.Join(root, "pkg", "b.py"), "from .a import Service\n\ndef main():\n    s = Service()\n    return s.serve()\n")
	writeFile(t, filepath.Join(root, "__pycache__", "garbage.py"), "xxx")
}

func TestIndexThenQueryAndIncremental(t *testing.T) {
	root := filepath.Join(t.TempDir(), "proj")
	store := filepath.Join(t.TempDir(), "store")
	makeProject(t, root)
	stats, err := indexer.IndexAll(root, store, false)
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesScanned != 3 || stats.FilesIndexed != 3 || stats.Nodes < 4 || len(stats.Errors) != 0 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
	status, err := graph.Status(store)
	if err != nil {
		t.Fatal(err)
	}
	if status["files"] != 3 || status["nodes_by_kind"].(map[string]int)["class"] != 1 {
		t.Fatalf("unexpected status: %#v", status)
	}
	hits, err := graph.Search(store, "needle", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0]["qualified_name"] != "Service" {
		t.Fatalf("docstring search hits = %#v", hits)
	}
	files, err := graph.ListFiles(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("files = %#v", files)
	}
	stats2, err := indexer.IndexAll(root, store, false)
	if err != nil {
		t.Fatal(err)
	}
	if stats2.FilesIndexed != 0 || stats2.FilesSkipped != stats.FilesScanned {
		t.Fatalf("incremental stats = %#v", stats2)
	}
	if err := os.Remove(filepath.Join(root, "pkg", "b.py")); err != nil {
		t.Fatal(err)
	}
	stats3, err := indexer.IndexAll(root, store, false)
	if err != nil {
		t.Fatal(err)
	}
	if stats3.FilesDeleted != 1 {
		t.Fatalf("delete stats = %#v", stats3)
	}
}

func TestGitignoreLargeFileForceAndErrorLog(t *testing.T) {
	root := filepath.Join(t.TempDir(), "proj")
	store := filepath.Join(t.TempDir(), "store")
	writeFile(t, filepath.Join(root, ".gitignore"), "ignored.py\n")
	writeFile(t, filepath.Join(root, "ok.py"), "def ok():\n    return 1\n")
	writeFile(t, filepath.Join(root, "ignored.py"), "def hidden():\n    return 1\n")
	writeFile(t, filepath.Join(root, "large.py"), strings.Repeat("x", 1024*1024+1))

	stats, err := indexer.IndexAll(root, store, false)
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesScanned != 2 || stats.FilesFailed != 1 {
		t.Fatalf("stats = %#v", stats)
	}
	raw, err := os.ReadFile(filepath.Join(store, "errors.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "size exceeded") {
		t.Fatalf("errors.log = %s", raw)
	}
	stats2, err := indexer.IndexAll(root, store, true)
	if err != nil {
		t.Fatal(err)
	}
	if stats2.FilesIndexed != 1 {
		t.Fatalf("force stats = %#v", stats2)
	}
}
