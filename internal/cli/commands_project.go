package cli

import (
	"bufio"
	"codegraph-cli/internal/config"
	"codegraph-cli/internal/indexer"
	"codegraph-cli/internal/model"
	"codegraph-cli/internal/registry"
	storepkg "codegraph-cli/internal/store"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func cmdInit(cfg appConfig, args []string) error {
	fs := newFlagSet("init")
	pathOpt := fs.String("path", "", "project path")
	name := fs.String("name", "", "registry name")
	keyOverride := fs.String("key", "", "project key")
	runIdx := fs.Bool("index", false, "run index")
	fs.BoolVar(runIdx, "i", false, "run index")
	if parseHelp(fs, args) {
		return nil
	}
	if err := parseFlagArgs(fs, args); err != nil {
		return err
	}
	pathArg := ""
	if fs.NArg() > 0 {
		pathArg = fs.Arg(0)
	}
	if pathArg != "" && *pathOpt != "" && config.Abs(pathArg) != config.Abs(*pathOpt) {
		return errors.New("不能同时指定不同的 PATH 参数和 --path")
	}
	root := firstNonEmpty(*pathOpt, pathArg)
	if root == "" {
		root = cfg.Cwd
	}
	root = config.Abs(root)
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return fmt.Errorf("路径不存在: %s", root)
	}
	key := *keyOverride
	if key == "" {
		key = config.DeriveProjectKey(root, *name)
	}
	store := config.StoreDirFor(key)
	db, err := storepkg.Open(store)
	if err != nil {
		return err
	}
	if err := storepkg.SetMeta(db, "project_root", root); err != nil {
		db.Close()
		return err
	}
	db.Close()
	regName := *name
	if regName == "" {
		parts := strings.Split(strings.Trim(key, "/"), "/")
		regName = parts[len(parts)-1]
	}
	remote := config.GitRemote(root)
	if err := registry.Upsert(regName, registry.Entry{Key: key, Root: root, Remote: remote}); err != nil {
		return err
	}
	var idx *model.IndexStats
	if *runIdx {
		s, err := indexer.IndexAll(root, store, false)
		if err != nil {
			return err
		}
		idx = &s
	}
	payload := map[string]any{
		"status": "initialized", "name": regName, "key": key, "root": root,
		"store": store, "remote": remote, "db": storepkg.DBPathFor(store),
		"next":    []string{"codegraph index    # 增量索引 / 重新索引", "codegraph status   # 查看索引状态"},
		"indexed": *runIdx, "index": idx,
	}
	text := fmt.Sprintf("✓ 已注册项目: %s\n  root  : %s\n  store : %s\n  key   : %s\n  remote: %s", regName, root, store, key, remoteText(remote))
	if idx != nil {
		text += fmt.Sprintf("\n\n✓ 索引完成\n  扫描: %d  索引: %d  跳过: %d  删除: %d  失败: %d\n  nodes: %d  edges: %d",
			idx.FilesScanned, idx.FilesIndexed, idx.FilesSkipped, idx.FilesDeleted, idx.FilesFailed, idx.Nodes, idx.Edges)
	} else {
		text += "\n\n下一步: codegraph index"
	}
	return emit(cfg, payload, text)
}

func remoteText(p *string) string {
	if p == nil || *p == "" {
		return "(no git remote)"
	}
	return *p
}

func cmdIndex(cfg appConfig, args []string) error {
	fs := newFlagSet("index")
	pathOpt := fs.String("path", "", "project path")
	force := fs.Bool("force", false, "force")
	quiet := fs.Bool("quiet", false, "quiet")
	if parseHelp(fs, args) {
		return nil
	}
	if err := parseFlagArgs(fs, args); err != nil {
		return err
	}
	_ = quiet
	var name string
	var entry registry.Entry
	var ok bool
	if *pathOpt != "" {
		name, entry, ok = registry.ResolveTarget(config.Abs(*pathOpt), cfg.Cwd)
		if !ok {
			return fmt.Errorf("未在 registry 中找到路径: %s\n提示: 先运行 `codegraph init --path %s`。", config.Abs(*pathOpt), config.Abs(*pathOpt))
		}
	} else {
		var err error
		_, name, entry, err = resolveProject(cfg, false)
		if err != nil {
			return err
		}
	}
	stats, err := indexer.IndexAll(config.Abs(entry.Root), config.StoreDirFor(entry.Key), *force)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"project": name, "root": config.Abs(entry.Root), "store": config.StoreDirFor(entry.Key),
		"files_scanned": stats.FilesScanned, "files_indexed": stats.FilesIndexed,
		"files_skipped": stats.FilesSkipped, "files_failed": stats.FilesFailed,
		"files_deleted": stats.FilesDeleted, "nodes": stats.Nodes, "edges": stats.Edges,
		"edges_resolved": stats.EdgesResolved, "edges_still_unresolved": stats.EdgesStillUnresolved,
		"errors": firstErrors(stats.Errors), "errors_total": len(stats.Errors),
	}
	text := fmt.Sprintf("✓ 索引完成: %s\n  扫描: %d  索引: %d  跳过: %d  删除: %d  失败: %d\n  nodes: %d  edges: %d",
		name, stats.FilesScanned, stats.FilesIndexed, stats.FilesSkipped, stats.FilesDeleted, stats.FilesFailed, stats.Nodes, stats.Edges)
	if len(stats.Errors) > 0 {
		text += fmt.Sprintf("\n  错误(%d): %s...", len(stats.Errors), stats.Errors[0])
	}
	return emit(cfg, payload, text)
}

func firstErrors(in []string) []string {
	if len(in) > 20 {
		return in[:20]
	}
	return in
}

func cmdSync(cfg appConfig, args []string) error {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Print(commandHelp("sync"))
		return nil
	}
	root, name, entry, err := resolveProject(cfg, false)
	if err != nil {
		return err
	}
	store := config.StoreDirFor(entry.Key)
	stats, err := indexer.IndexAll(config.Abs(root), store, false)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"project": name, "root": config.Abs(root), "store": store,
		"files_scanned": stats.FilesScanned, "files_indexed": stats.FilesIndexed,
		"files_skipped": stats.FilesSkipped, "files_failed": stats.FilesFailed,
		"files_deleted": stats.FilesDeleted, "nodes": stats.Nodes, "edges": stats.Edges,
		"edges_resolved": stats.EdgesResolved, "edges_still_unresolved": stats.EdgesStillUnresolved,
		"errors": firstErrors(stats.Errors), "errors_total": len(stats.Errors),
	}
	text := fmt.Sprintf("✓ sync %s\n  扫描: %d  索引: %d  跳过: %d  删除: %d  失败: %d\n  nodes: %d  edges: %d",
		name, stats.FilesScanned, stats.FilesIndexed, stats.FilesSkipped, stats.FilesDeleted, stats.FilesFailed, stats.Nodes, stats.Edges)
	return emit(cfg, payload, text)
}

func cmdUnlock(cfg appConfig, args []string) error {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Print(commandHelp("unlock"))
		return nil
	}
	return emit(cfg, map[string]any{"status": "ok", "message": "no lock backend configured"}, "✓ unlock: no-op")
}

func cmdList(cfg appConfig, args []string) error {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Print(commandHelp("list"))
		return nil
	}
	data := registry.Load()
	rows := make([]map[string]any, 0, len(data.Entries))
	names := make([]string, 0, len(data.Entries))
	for name := range data.Entries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		e := data.Entries[name]
		rows = append(rows, map[string]any{"name": name, "key": e.Key, "root": e.Root, "remote": e.Remote, "size_bytes": dirSize(config.StoreDirFor(e.Key))})
	}
	payload := map[string]any{"home": config.CodegraphHome(), "entries": rows}
	if cfg.JSON {
		return emit(cfg, payload, "")
	}
	if len(rows) == 0 {
		fmt.Println("(空) 还没有注册过项目，运行 `codegraph init` 开始。")
		return nil
	}
	fmt.Printf("home: %s\n\n", config.CodegraphHome())
	fmt.Printf(defaultListFmt, "NAME", "SIZE", "ROOT")
	fmt.Printf(defaultListFmt, strings.Repeat("-", 24), strings.Repeat("-", 8), strings.Repeat("-", 40))
	for _, r := range rows {
		fmt.Printf(defaultListFmt, r["name"], humanSize(r["size_bytes"].(int64)), r["root"])
	}
	return nil
}

func dirSize(root string) int64 {
	var n int64
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if st, err := d.Info(); err == nil {
				n += st.Size()
			}
		}
		return nil
	})
	return n
}

func humanSize(n int64) string {
	units := []string{"B", "KB", "MB", "GB"}
	v := float64(n)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%dB", n)
	}
	return fmt.Sprintf("%.1f%s", v, units[i])
}

func cmdInfo(cfg appConfig, args []string) error {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Print(commandHelp("info"))
		return nil
	}
	root, name, entry, err := resolveProject(cfg, false)
	if err != nil {
		return err
	}
	payload := map[string]any{"name": name, "key": entry.Key, "root": root, "store": config.StoreDirFor(entry.Key), "remote": entry.Remote}
	text := fmt.Sprintf("name : %s\nroot : %s\nstore: %s\nkey  : %s\nremote: %s", name, root, config.StoreDirFor(entry.Key), entry.Key, remoteText(entry.Remote))
	return emit(cfg, payload, text)
}

func cmdUninit(cfg appConfig, args []string) error {
	fs := newFlagSet("uninit")
	yes := fs.Bool("y", false, "yes")
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
	store := config.StoreDirFor(entry.Key)
	if !*yes {
		if err := confirmContinue(fmt.Sprintf("将删除 %s（项目 %s 的索引数据），继续？", store, name)); err != nil {
			return err
		}
	}
	err = os.RemoveAll(store)
	payload := map[string]any{"status": "uninitialized", "name": name, "store": store, "root": root}
	return emit(cfg, payload, fmt.Sprintf("✓ 已删除索引数据: %s", store))
}

func cmdRm(cfg appConfig, args []string) error {
	fs := newFlagSet("rm")
	purge := fs.Bool("purge", false, "purge")
	yes := fs.Bool("y", false, "yes")
	if parseHelp(fs, args) {
		return nil
	}
	if err := parseFlagArgs(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("missing project name")
	}
	name := fs.Arg(0)
	data := registry.Load()
	entry, ok := data.Entries[name]
	if !ok {
		return fmt.Errorf("未找到项目: %s", name)
	}
	if !*yes {
		msg := fmt.Sprintf("将从 registry 移除 %s，继续？", name)
		if *purge {
			msg = fmt.Sprintf("将从 registry 移除 %s（并删除索引数据），继续？", name)
		}
		if err := confirmContinue(msg); err != nil {
			return err
		}
	}
	purgedStore := ""
	if *purge {
		purgedStore = config.StoreDirFor(entry.Key)
		_ = os.RemoveAll(purgedStore)
	}
	if _, err := registry.Remove(name); err != nil {
		return err
	}
	payload := map[string]any{"status": "removed", "name": name, "purged_store": nil}
	if purgedStore != "" {
		payload["purged_store"] = purgedStore
	}
	text := fmt.Sprintf("✓ 已移除 %s", name)
	if purgedStore != "" {
		text += fmt.Sprintf("（并删除 %s）", purgedStore)
	}
	return emit(cfg, payload, text)
}

func confirmContinue(prompt string) error {
	fmt.Fprintf(os.Stderr, "%s [y/N]: ", prompt)
	sc := bufio.NewScanner(os.Stdin)
	if !sc.Scan() {
		return errors.New("operation aborted")
	}
	ans := strings.TrimSpace(strings.ToLower(sc.Text()))
	if ans != "y" && ans != "yes" {
		return errors.New("operation aborted")
	}
	return nil
}
