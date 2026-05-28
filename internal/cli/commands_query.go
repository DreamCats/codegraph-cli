package cli

import (
	"github.com/DreamCats/codegraph-cli/internal/config"
	graphpkg "github.com/DreamCats/codegraph-cli/internal/graph"
	"fmt"
	"strings"
)

func cmdStatus(cfg appConfig, args []string) error {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Print(commandHelp("status"))
		return nil
	}
	root, name, entry, err := resolveProject(cfg, false)
	if err != nil {
		return err
	}
	store := config.StoreDirFor(entry.Key)
	payload, err := graphpkg.Status(store)
	if err != nil {
		return err
	}
	payload["project"] = name
	payload["root"] = root
	if cfg.JSON {
		return emit(cfg, payload, "")
	}
	if init, _ := payload["initialized"].(bool); !init {
		fmt.Printf("(未索引) %s\n运行 `codegraph index` 进行首次索引。\n", store)
		return nil
	}
	fmt.Printf("project: %s\nroot   : %s\nstore  : %s\nschema : v%v\nfiles  : %v\nnodes  : %v\nedges  : %v\n",
		name, root, store, payload["schema_version"], payload["files"], payload["nodes"], payload["edges"])
	return nil
}

func cmdQuery(cfg appConfig, args []string) error {
	fs := newFlagSet("query")
	limit := fs.Int("limit", 20, "limit")
	kind := fs.String("kind", "", "kind")
	if parseHelp(fs, args) {
		return nil
	}
	if err := parseFlagArgs(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("missing search term")
	}
	term := fs.Arg(0)
	root, name, entry, err := resolveProject(cfg, false)
	if err != nil {
		return err
	}
	store := config.StoreDirFor(entry.Key)
	results, err := graphpkg.Search(store, term, *kind, *limit)
	if err != nil {
		return err
	}
	payload := map[string]any{"project": name, "query": term, "kind": nilIfEmpty(*kind), "count": len(results), "results": results}
	stale := graphpkg.AttachStale(payload, root, store)
	if cfg.JSON {
		return emit(cfg, payload, "")
	}
	if len(results) == 0 {
		fmt.Printf("(无结果) query=%q%s\n", term, staleHintText(stale))
		return nil
	}
	lines := []string{fmt.Sprintf("%d 个结果：", len(results))}
	for _, r := range results {
		sig, _ := r["signature"].(string)
		extra := ""
		if sig != "" {
			if len(sig) > 60 {
				sig = sig[:60]
			}
			extra = "  " + sig
		}
		lines = append(lines, fmt.Sprintf("  %-8v %-40v %v:%v%s", r["kind"], r["qualified_name"], r["file_path"], r["start_line"], extra))
	}
	fmt.Println(strings.Join(lines, "\n") + staleHintText(stale))
	return nil
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func cmdFiles(cfg appConfig, args []string) error {
	fs := newFlagSet("files")
	var globs multiFlag
	fs.Var(&globs, "glob", "glob")
	if parseHelp(fs, args) {
		return nil
	}
	if err := parseFlagArgs(fs, args); err != nil {
		return err
	}
	root, name, entry, err := resolveProject(cfg, false)
	if err != nil {
		return err
	}
	files, err := graphpkg.ListFiles(config.StoreDirFor(entry.Key), globs)
	if err != nil {
		return err
	}
	payload := map[string]any{"project": name, "count": len(files), "files": files}
	stale := graphpkg.AttachStale(payload, root, config.StoreDirFor(entry.Key))
	if cfg.JSON {
		return emit(cfg, payload, "")
	}
	if len(files) == 0 {
		fmt.Println("(无文件) 也许还没运行 `codegraph index`?" + staleHintText(stale))
		return nil
	}
	lines := []string{}
	for _, f := range files {
		lines = append(lines, fmt.Sprintf("%-10v %6v nodes  %v", f["language"], f["node_count"], f["path"]))
	}
	fmt.Println(strings.Join(lines, "\n") + staleHintText(stale))
	return nil
}

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func staleHintText(stale map[string]any) string {
	if v, _ := stale["is_stale"].(bool); !v {
		return ""
	}
	title := "索引可能过期：检测到源码文件晚于上次索引。"
	if stale["reason"] == "never_indexed" {
		title = "索引尚未建立。"
	}
	return fmt.Sprintf("\n\n%s\n建议先运行：%s\n然后重新执行当前查询。", title, stale["command"])
}
