package resolver_test

import (
	"codegraph-cli/internal/indexer"
	"codegraph-cli/internal/resolver"
	"os"
	"path/filepath"
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

func TestResolveTypeScriptPathAlias(t *testing.T) {
	root := filepath.Join(t.TempDir(), "proj")
	store := filepath.Join(t.TempDir(), "store")
	writeFile(t, filepath.Join(root, "tsconfig.json"), `{"compilerOptions":{"paths":{"@/*":["src/*"]}}}`)
	writeFile(t, filepath.Join(root, "src", "util.ts"), "export function helper() { return 1; }\n")
	writeFile(t, filepath.Join(root, "src", "main.ts"), "import { helper } from '@/util';\nexport function main() { return helper(); }\n")

	if _, err := indexer.IndexAll(root, store, false); err != nil {
		t.Fatal(err)
	}
	edges, err := resolver.ResolvedEdgesFor(store, "", "", "calls")
	if err != nil {
		t.Fatal(err)
	}
	for _, edge := range edges {
		if edge["source_file"] == "src/main.ts" && edge["target_file"] == "src/util.ts" && edge["target_name"] == "helper" {
			return
		}
	}
	t.Fatalf("expected alias-resolved helper edge, got %#v", edges)
}

func TestResolvePythonSameFileImportAliasGlobalAndAmbiguous(t *testing.T) {
	root := filepath.Join(t.TempDir(), "proj")
	store := filepath.Join(t.TempDir(), "store")
	writeFile(t, filepath.Join(root, "a.py"), "def helper():\n    return 42\n\ndef unique_one():\n    return 1\n\ndef common():\n    return 1\n")
	writeFile(t, filepath.Join(root, "b.py"), "from a import helper as h\n\ndef main():\n    return h()\n")
	writeFile(t, filepath.Join(root, "c.py"), "def local_helper():\n    return 1\n\ndef local_main():\n    return local_helper()\n")
	writeFile(t, filepath.Join(root, "d.py"), "def main2():\n    return unique_one()\n")
	writeFile(t, filepath.Join(root, "e.py"), "def common():\n    return 2\n")
	writeFile(t, filepath.Join(root, "f.py"), "def main3():\n    return common()\n")

	if _, err := indexer.IndexAll(root, store, false); err != nil {
		t.Fatal(err)
	}
	edges, err := resolver.ResolvedEdgesFor(store, "", "", "calls")
	if err != nil {
		t.Fatal(err)
	}
	var aliasImport, sameFile, globalUnique bool
	for _, edge := range edges {
		if edge["source_file"] == "b.py" && edge["target_file"] == "a.py" && edge["target_name"] == "helper" {
			aliasImport = true
		}
		if edge["source_file"] == "c.py" && edge["target_file"] == "c.py" && edge["target_name"] == "local_helper" {
			sameFile = true
		}
		if edge["source_file"] == "d.py" && edge["target_file"] == "a.py" && edge["target_name"] == "unique_one" {
			globalUnique = true
		}
		if edge["source_file"] == "f.py" && edge["target_name"] == "common" {
			t.Fatalf("ambiguous common should not resolve: %#v", edges)
		}
	}
	if !aliasImport || !sameFile || !globalUnique {
		t.Fatalf("alias=%v sameFile=%v global=%v edges=%#v", aliasImport, sameFile, globalUnique, edges)
	}
}

func TestFindNodeModesAndResolveIdempotent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "proj")
	store := filepath.Join(t.TempDir(), "store")
	writeFile(t, filepath.Join(root, "a.py"), "class Foo:\n    def bar(self): pass\n\ndef main():\n    return Foo()\n")
	if _, err := indexer.IndexAll(root, store, false); err != nil {
		t.Fatal(err)
	}
	byName, err := resolver.FindNode(store, "Foo")
	if err != nil || len(byName) == 0 || byName[0]["name"] != "Foo" {
		t.Fatalf("find by name = %#v err=%v", byName, err)
	}
	byQName, err := resolver.FindNode(store, "Foo.bar")
	if err != nil || len(byQName) == 0 || byQName[0]["qualified_name"] != "Foo.bar" {
		t.Fatalf("find by qname = %#v err=%v", byQName, err)
	}
	byID, err := resolver.FindNode(store, byName[0]["id"].(string))
	if err != nil || len(byID) == 0 || byID[0]["id"] != byName[0]["id"] {
		t.Fatalf("find by id = %#v err=%v", byID, err)
	}
	s1, err := resolver.ResolveAll(store)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := resolver.ResolveAll(store)
	if err != nil {
		t.Fatal(err)
	}
	if s2.EdgesResolvedNow != 0 {
		t.Fatalf("second resolve resolved %d edges after first %#v", s2.EdgesResolvedNow, s1)
	}
}

func TestResolveGoSamePackageAndModuleImport(t *testing.T) {
	root := filepath.Join(t.TempDir(), "proj")
	store := filepath.Join(t.TempDir(), "store")
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/team/proj\n")
	writeFile(t, filepath.Join(root, "pkg", "svc", "helper.go"), "package svc\n\nfunc helper() int { return 1 }\n")
	writeFile(t, filepath.Join(root, "pkg", "svc", "main.go"), "package svc\n\nfunc Main() int { return helper() }\n")
	writeFile(t, filepath.Join(root, "pkg", "util", "util.go"), "package util\n\nfunc Do() int { return 1 }\n")
	writeFile(t, filepath.Join(root, "cmd", "app", "main.go"), "package main\n\nimport alias \"example.com/team/proj/pkg/util\"\n\nfunc Run() int { return alias.Do() }\n")

	if _, err := indexer.IndexAll(root, store, false); err != nil {
		t.Fatal(err)
	}
	edges, err := resolver.ResolvedEdgesFor(store, "", "", "calls")
	if err != nil {
		t.Fatal(err)
	}
	var samePkg, moduleImport bool
	for _, edge := range edges {
		if edge["source_file"] == "pkg/svc/main.go" && edge["target_file"] == "pkg/svc/helper.go" && edge["target_name"] == "helper" {
			samePkg = true
		}
		if edge["source_file"] == "cmd/app/main.go" && edge["target_file"] == "pkg/util/util.go" && edge["target_name"] == "Do" {
			moduleImport = true
		}
	}
	if !samePkg || !moduleImport {
		t.Fatalf("samePkg=%v moduleImport=%v edges=%#v", samePkg, moduleImport, edges)
	}
}
