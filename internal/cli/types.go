package cli

import (
	"github.com/DreamCats/codegraph-cli/internal/config"
	"github.com/DreamCats/codegraph-cli/internal/registry"
	"fmt"
)

const defaultListFmt = "%-24s %8s  %s\n"

type appConfig struct {
	Target  string
	JSON    bool
	Verbose bool
	Cwd     string
}

func resolveProject(cfg appConfig, allowUninit bool) (string, string, registry.Entry, error) {
	name, entry, ok := registry.ResolveTarget(cfg.Target, cfg.Cwd)
	if ok {
		return config.Abs(entry.Root), name, entry, nil
	}
	if allowUninit {
		root := cfg.Cwd
		if cfg.Target != "" {
			root = cfg.Target
		}
		return config.Abs(root), "", registry.Entry{}, nil
	}
	return "", "", registry.Entry{}, fmt.Errorf("未在 registry 中找到目标: %s\n提示: 先运行 `codegraph init`，或用 `codegraph -C <name> ...` 指定已注册项目。", firstNonEmpty(cfg.Target, cfg.Cwd))
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
