package config

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func CodegraphHome() string {
	if v := os.Getenv("CODEGRAPH_HOME"); v != "" {
		return Abs(v)
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return Abs(filepath.Join(base, "codegraph"))
}

func StoresDir() string             { return filepath.Join(CodegraphHome(), "stores") }
func RegistryPath() string          { return filepath.Join(CodegraphHome(), "registry.json") }
func StoreDirFor(key string) string { return filepath.Join(StoresDir(), filepath.FromSlash(key)) }

func Abs(p string) string {
	if strings.HasPrefix(p, "~") {
		home, _ := os.UserHomeDir()
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	r, err := filepath.EvalSymlinks(a)
	if err == nil {
		return r
	}
	return a
}

func GitRemote(root string) *string {
	cmd := exec.Command("git", "-C", root, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return nil
	}
	return &s
}

var unsafeKeyRE = regexp.MustCompile(`[^A-Za-z0-9._/-]`)

func normalizeRemote(url string) string {
	url = strings.TrimSpace(url)
	url = strings.TrimSuffix(strings.TrimSuffix(url, "/"), ".git")
	if m := regexp.MustCompile(`^[^@]+@([^:]+):(.+)$`).FindStringSubmatch(url); m != nil {
		return m[1] + "/" + strings.TrimLeft(m[2], "/")
	}
	if m := regexp.MustCompile(`^[a-zA-Z]+://(?:[^@/]+@)?([^/]+)/(.+)$`).FindStringSubmatch(url); m != nil {
		return m[1] + "/" + strings.TrimLeft(m[2], "/")
	}
	return url
}

func safeKey(raw string) string {
	out := strings.Trim(unsafeKeyRE.ReplaceAllString(raw, "_"), "/")
	if out == "" {
		return "unnamed"
	}
	return out
}

func DeriveProjectKey(root, name string) string {
	if name != "" {
		return safeKey(name)
	}
	if remote := GitRemote(root); remote != nil {
		return safeKey(normalizeRemote(*remote))
	}
	h := sha1.Sum([]byte(Abs(root)))
	return "local/" + filepath.Base(root) + "-" + hex.EncodeToString(h[:])[:12]
}
