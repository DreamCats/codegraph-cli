package registry

import (
	"github.com/DreamCats/codegraph-cli/internal/config"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
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

func ResolveTarget(target, cwd string) (string, Entry, bool) {
	data := Load()
	if target != "" {
		if e, ok := data.Entries[target]; ok {
			return target, e, true
		}
		real := config.Abs(target)
		for name, e := range data.Entries {
			if config.Abs(e.Root) == real {
				return name, e, true
			}
		}
		return "", Entry{}, false
	}
	real := config.Abs(cwd)
	for name, e := range data.Entries {
		if config.Abs(e.Root) == real {
			return name, e, true
		}
	}
	return "", Entry{}, false
}
