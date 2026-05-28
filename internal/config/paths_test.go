package config_test

import (
	"codegraph-cli/internal/config"
	"strings"
	"testing"
)

func TestCodegraphHomeEnvPriority(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODEGRAPH_HOME", dir)
	home := config.CodegraphHome()
	if home != config.Abs(dir) {
		t.Fatalf("home = %q, want %q", home, config.Abs(dir))
	}
}

func TestDeriveProjectKeyWithNameAndLocalFallback(t *testing.T) {
	root := t.TempDir()
	if got := config.DeriveProjectKey(root, "my-proj"); got != "my-proj" {
		t.Fatalf("key with name = %q", got)
	}
	got := config.DeriveProjectKey(root, "")
	if !strings.HasPrefix(got, "local/") {
		t.Fatalf("local key = %q", got)
	}
}

func TestDeriveProjectKeySanitizesName(t *testing.T) {
	if got := config.DeriveProjectKey(t.TempDir(), "foo bar/baz!"); got != "foo_bar/baz_" {
		t.Fatalf("sanitized key = %q", got)
	}
}
