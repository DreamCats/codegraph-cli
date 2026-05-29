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
	for _, cmd := range []string{"init", "uninit", "rm", "index", "sync", "unlock", "query", "files", "overview", "architecture", "context", "affected", "impact", "status", "list", "info"} {
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
	if err := Run([]string{"--json", "overview", "--path", project}); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"--json", "context", "--path", project, "--summary", "helper"}); err != nil {
		t.Fatal(err)
	}
	protected := captureStdout(t, func() {
		if err := Run([]string{"--json", "context", "--path", project, "--max-json-bytes", "1", "helper"}); err != nil {
			t.Fatal(err)
		}
	})
	var protectedPayload map[string]any
	if err := json.Unmarshal([]byte(protected), &protectedPayload); err != nil {
		t.Fatalf("invalid protected json %q: %v", protected, err)
	}
	output := protectedPayload["output"].(map[string]any)
	if output["truncated"] != true {
		t.Fatalf("expected large-output protection, got %#v", output)
	}
	if _, ok := protectedPayload["code_blocks"]; ok {
		t.Fatalf("protected context should omit code_blocks: %#v", protectedPayload)
	}
	full := captureStdout(t, func() {
		if err := Run([]string{"--json", "context", "--path", project, "--max-json-bytes", "1", "--allow-large", "helper"}); err != nil {
			t.Fatal(err)
		}
	})
	var fullPayload map[string]any
	if err := json.Unmarshal([]byte(full), &fullPayload); err != nil {
		t.Fatalf("invalid full json %q: %v", full, err)
	}
	if _, ok := fullPayload["output"]; ok {
		t.Fatalf("allow-large should not add output protection metadata: %#v", fullPayload["output"])
	}
	if _, ok := fullPayload["code_blocks"]; !ok {
		t.Fatalf("allow-large should preserve code_blocks: %#v", fullPayload)
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

func TestUnknownFlagHints(t *testing.T) {
	err := Run([]string{"query", "--target", "demo", "Service"})
	if err == nil {
		t.Fatal("expected unknown --target error")
	}
	if !strings.Contains(err.Error(), "--target is global") || !strings.Contains(err.Error(), "--path /path/to/project") {
		t.Fatalf("unexpected --target hint: %v", err)
	}

	err = Run([]string{"rm", "--path", "/tmp/project", "demo"})
	if err == nil {
		t.Fatal("expected unknown --path error")
	}
	if !strings.Contains(err.Error(), "does not accept --path") || !strings.Contains(err.Error(), "--target /path/to/project") {
		t.Fatalf("unexpected --path hint: %v", err)
	}

	err = Run([]string{"query", "--limt", "3", "Service"})
	if err == nil {
		t.Fatal("expected typo flag error")
	}
	if !strings.Contains(err.Error(), "did you mean `--limit`") {
		t.Fatalf("unexpected typo hint: %v", err)
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
