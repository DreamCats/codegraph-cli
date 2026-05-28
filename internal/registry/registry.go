package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/DreamCats/codegraph-cli/internal/config"
)

type Data struct {
	Version int              `json:"version"`
	Entries map[string]Entry `json:"entries"`
}

type Entry struct {
	Key       string  `json:"key"`
	Root      string  `json:"root"`
	Remote    *string `json:"remote"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

var ErrTargetNotFound = errors.New("target not found")

type AmbiguousTargetError struct {
	Target     string
	Candidates []string
}

func (e AmbiguousTargetError) Error() string {
	return fmt.Sprintf("target %q is ambiguous; candidates: %s", e.Target, strings.Join(e.Candidates, ", "))
}

func Load() Data {
	data := Data{Version: 1, Entries: map[string]Entry{}}
	raw, err := os.ReadFile(config.RegistryPath())
	if err != nil {
		return data
	}
	if json.Unmarshal(raw, &data) != nil || data.Entries == nil {
		return Data{Version: 1, Entries: map[string]Entry{}}
	}
	return data
}

func save(data Data) error {
	if err := os.MkdirAll(filepath.Dir(config.RegistryPath()), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(config.RegistryPath(), raw, 0o644)
}

func Upsert(name string, entry Entry) error {
	data := Load()
	if old, ok := data.Entries[name]; ok && old.CreatedAt != "" {
		entry.CreatedAt = old.CreatedAt
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if entry.CreatedAt == "" {
		entry.CreatedAt = now
	}
	entry.UpdatedAt = now
	data.Entries[name] = entry
	return save(data)
}

func Remove(name string) (bool, error) {
	data := Load()
	if _, ok := data.Entries[name]; !ok {
		return false, nil
	}
	delete(data.Entries, name)
	return true, save(data)
}

func ResolveTarget(target, cwd string) (string, Entry, error) {
	data := Load()
	if target != "" {
		if e, ok := data.Entries[target]; ok {
			return target, e, nil
		}
		real := config.Abs(target)
		for name, e := range data.Entries {
			if config.Abs(e.Root) == real {
				return name, e, nil
			}
		}
		matches := suffixMatches(data, target)
		if len(matches) == 1 {
			return matches[0], data.Entries[matches[0]], nil
		}
		if len(matches) > 1 {
			return "", Entry{}, AmbiguousTargetError{Target: target, Candidates: matches}
		}
		return "", Entry{}, ErrTargetNotFound
	}
	real := config.Abs(cwd)
	for name, e := range data.Entries {
		if config.Abs(e.Root) == real {
			return name, e, nil
		}
	}
	return "", Entry{}, ErrTargetNotFound
}

func DefaultNameForEntry(key, root string) string {
	data := Load()
	parts := strings.Split(strings.Trim(toSlash(key), "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := strings.Join(parts[i:], "/")
		if candidate == "" {
			continue
		}
		old, exists := data.Entries[candidate]
		if !exists || sameEntry(old, key, root) {
			return candidate
		}
	}
	fallback := normalizeTarget(key)
	if fallback == "" {
		return "unnamed"
	}
	return fallback
}

func sameEntry(entry Entry, key, root string) bool {
	if entry.Key == key {
		return true
	}
	return root != "" && config.Abs(entry.Root) == config.Abs(root)
}

func suffixMatches(data Data, target string) []string {
	token := normalizeTarget(target)
	if token == "" {
		return nil
	}
	seen := map[string]bool{}
	for name, entry := range data.Entries {
		if hasPathSuffix(name, token) ||
			hasPathSuffix(entry.Key, token) ||
			hasPathSuffix(entry.Root, token) ||
			(entry.Remote != nil && hasPathSuffix(*entry.Remote, token)) {
			seen[name] = true
		}
	}
	matches := make([]string, 0, len(seen))
	for name := range seen {
		matches = append(matches, name)
	}
	sort.Strings(matches)
	return matches
}

func hasPathSuffix(value, token string) bool {
	value = normalizeTarget(value)
	return value == token || strings.HasSuffix(value, "/"+token)
}

func normalizeTarget(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(strings.TrimSuffix(value, "/"), ".git")
	value = config.NormalizeRemote(value)
	value = strings.Trim(toSlash(value), "/")
	return value
}

func toSlash(value string) string {
	return filepath.ToSlash(value)
}
