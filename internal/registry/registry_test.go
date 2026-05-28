package registry_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/DreamCats/codegraph-cli/internal/registry"
)

func strptr(s string) *string { return &s }

func TestResolveTargetBySuffixAndAmbiguity(t *testing.T) {
	t.Setenv("CODEGRAPH_HOME", filepath.Join(t.TempDir(), "home"))
	ttecRoot := filepath.Join(t.TempDir(), "code.byted.org", "ttec", "live_pack")
	oecRoot := filepath.Join(t.TempDir(), "code.byted.org", "oec", "live_pack")

	if err := registry.Upsert("ttec/live_pack", registry.Entry{
		Key:    "code.byted.org/ttec/live_pack",
		Root:   ttecRoot,
		Remote: strptr("git@code.byted.org:ttec/live_pack.git"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Upsert("oec/live_pack", registry.Entry{
		Key:    "code.byted.org/oec/live_pack",
		Root:   oecRoot,
		Remote: strptr("git@code.byted.org:oec/live_pack.git"),
	}); err != nil {
		t.Fatal(err)
	}

	name, entry, err := registry.ResolveTarget("ttec/live_pack", "")
	if err != nil {
		t.Fatal(err)
	}
	if name != "ttec/live_pack" || entry.Root != ttecRoot {
		t.Fatalf("resolved %q %#v, want ttec root %s", name, entry, ttecRoot)
	}

	_, _, err = registry.ResolveTarget("live_pack", "")
	var amb registry.AmbiguousTargetError
	if !errors.As(err, &amb) {
		t.Fatalf("expected ambiguous target, got %v", err)
	}
	if len(amb.Candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %#v", amb.Candidates)
	}
}

func TestDefaultNameForEntryUsesShortestFreeSuffix(t *testing.T) {
	t.Setenv("CODEGRAPH_HOME", filepath.Join(t.TempDir(), "home"))
	firstRoot := filepath.Join(t.TempDir(), "code.byted.org", "ttec", "live_pack")
	secondRoot := filepath.Join(t.TempDir(), "code.byted.org", "oec", "live_pack")

	first := registry.DefaultNameForEntry("code.byted.org/ttec/live_pack", firstRoot)
	if first != "live_pack" {
		t.Fatalf("first default name = %q, want live_pack", first)
	}
	if err := registry.Upsert(first, registry.Entry{Key: "code.byted.org/ttec/live_pack", Root: firstRoot}); err != nil {
		t.Fatal(err)
	}

	second := registry.DefaultNameForEntry("code.byted.org/oec/live_pack", secondRoot)
	if second != "oec/live_pack" {
		t.Fatalf("second default name = %q, want oec/live_pack", second)
	}
}
