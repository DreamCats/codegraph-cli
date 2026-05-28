package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeCLIFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTopLevelAndSubcommandHelp(t *testing.T) {
	if err := Run([]string{"--help"}); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{"init", "uninit", "rm", "index", "sync", "unlock", "query", "files", "context", "affected", "impact", "status", "list", "info"} {
		if err := Run([]string{cmd, "--help"}); err != nil {
			t.Fatalf("%s --help: %v", cmd, err)
		}
	}
}

func TestInitIndexQueryAndStale(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("CODEGRAPH_HOME", home)
	project := filepath.Join(t.TempDir(), "proj")
	source := filepath.Join(project, "app.py")
	writeCLIFile(t, source, "def helper():\n    return 1\n")

	if err := Run([]string{"init", "--path", project, "--name", "demo", "--index"}); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"list"}); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"index", "--path", project, "--quiet"}); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(5 * time.Second)
	if err := os.Chtimes(source, future, future); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := Run([]string{"--json", "--target", project, "query", "helper"}); err != nil {
			t.Fatal(err)
		}
	})
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid json %q: %v", out, err)
	}
	stale := payload["stale"].(map[string]any)
	if stale["is_stale"] != true {
		t.Fatalf("expected stale payload, got %#v", stale)
	}
}

func TestRmAndUninitRequireConfirmation(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("CODEGRAPH_HOME", home)
	project := filepath.Join(t.TempDir(), "proj")
	writeCLIFile(t, filepath.Join(project, "app.py"), "def helper():\n    return 1\n")
	if err := Run([]string{"init", "--path", project, "--name", "demo", "--index"}); err != nil {
		t.Fatal(err)
	}

	withStdin(t, "n\n", func() {
		if err := Run([]string{"rm", "demo"}); err == nil || !strings.Contains(err.Error(), "aborted") {
			t.Fatalf("expected abort, got %v", err)
		}
	})
	if err := Run([]string{"rm", "demo", "--purge", "-y"}); err != nil {
		t.Fatal(err)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	raw, _ := io.ReadAll(r)
	return string(raw)
}

func withStdin(t *testing.T, input string, fn func()) {
	t.Helper()
	old := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(input); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	os.Stdin = r
	defer func() { os.Stdin = old }()
	fn()
}
