package graph_test

import (
	"codegraph-cli/internal/graph"
	"codegraph-cli/internal/indexer"
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

func TestSearchUsesDocstringAndContextIncludesCode(t *testing.T) {
	root := filepath.Join(t.TempDir(), "proj")
	store := filepath.Join(t.TempDir(), "store")
	writeFile(t, filepath.Join(root, "pkg", "a.py"), `class Service:
    """needle service docs."""
    def serve(self):
        return helper()

def helper():
    return 42
`)
	writeFile(t, filepath.Join(root, "pkg", "b.py"), `from .a import Service

def main():
    s = Service()
    return s.serve()
`)

	stats, err := indexer.IndexAll(root, store, false)
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesIndexed != 2 {
		t.Fatalf("indexed files = %d", stats.FilesIndexed)
	}

	hits, err := graph.Search(store, "needle", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0]["qualified_name"] != "Service" {
		t.Fatalf("expected Service from docstring FTS, got %#v", hits)
	}

	payload, err := graph.BuildContext(root, store, "fix Service serve", 20, 8, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload["entrypoints"].([]map[string]any)) == 0 {
		t.Fatalf("expected entrypoints: %#v", payload)
	}
	if len(payload["code_blocks"].([]map[string]any)) == 0 {
		t.Fatalf("expected code snippets: %#v", payload)
	}
	md := graph.FormatContextMarkdown(payload)
	if !strings.Contains(md, "## Code") || !strings.Contains(md, "Service") {
		t.Fatalf("unexpected markdown:\n%s", md)
	}
}

func TestAffectedDetectsGoTests(t *testing.T) {
	root := filepath.Join(t.TempDir(), "proj")
	store := filepath.Join(t.TempDir(), "store")
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/team/proj\n")
	writeFile(t, filepath.Join(root, "pkg", "svc", "helper.go"), "package svc\n\nfunc Helper() int { return 1 }\n")
	writeFile(t, filepath.Join(root, "pkg", "svc", "helper_test.go"), "package svc\n\nfunc TestHelper() { Helper() }\n")
	if _, err := indexer.IndexAll(root, store, false); err != nil {
		t.Fatal(err)
	}
	result, err := graph.AffectedFiles(store, []string{"pkg/svc/helper.go"}, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	tests := result["affected_tests"].([]string)
	if len(tests) != 1 || tests[0] != "pkg/svc/helper_test.go" {
		t.Fatalf("affected tests = %#v", tests)
	}
}
