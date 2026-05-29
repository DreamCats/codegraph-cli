package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/DreamCats/codegraph-cli/internal/config"
	graphpkg "github.com/DreamCats/codegraph-cli/internal/graph"
	"github.com/DreamCats/codegraph-cli/internal/resolver"
	"os"
	"strconv"
	"strings"
)

func cmdResolve(cfg appConfig, args []string) error {
	fs := newFlagSet("resolve")
	pathOpt := fs.String("path", "", "project path")
	if parseHelp(fs, args) {
		return nil
	}
	if err := parseFlagArgs(fs, args); err != nil {
		return err
	}
	cfg = withPathTarget(cfg, *pathOpt)
	_, name, entry, err := resolveProject(cfg, false)
	if err != nil {
		return err
	}
	stats, err := resolver.ResolveAll(config.StoreDirFor(entry.Key))
	if err != nil {
		return err
	}
	payload := map[string]any{"project": name, "edges_total": stats.EdgesTotal, "edges_resolved_before": stats.EdgesResolvedBefore, "edges_resolved_now": stats.EdgesResolvedNow, "edges_still_unresolved": stats.EdgesStillUnresolved}
	return emit(cfg, payload, fmt.Sprintf("✓ resolve 完成: resolved_now=%d unresolved=%d", stats.EdgesResolvedNow, stats.EdgesStillUnresolved))
}

func cmdCallers(cfg appConfig, args []string) error {
	fs := newFlagSet("callers")
	pathOpt := fs.String("path", "", "project path")
	limit := fs.Int("limit", 20, "limit")
	if parseHelp(fs, args) {
		return nil
	}
	if err := parseFlagArgs(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("missing symbol")
	}
	cfg = withPathTarget(cfg, *pathOpt)
	return callersOrCallees(cfg, fs.Arg(0), *limit, true)
}

func cmdCallees(cfg appConfig, args []string) error {
	fs := newFlagSet("callees")
	pathOpt := fs.String("path", "", "project path")
	limit := fs.Int("limit", 20, "limit")
	if parseHelp(fs, args) {
		return nil
	}
	if err := parseFlagArgs(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("missing symbol")
	}
	cfg = withPathTarget(cfg, *pathOpt)
	return callersOrCallees(cfg, fs.Arg(0), *limit, false)
}

func callersOrCallees(cfg appConfig, symbol string, limit int, callers bool) error {
	root, name, entry, err := resolveProject(cfg, false)
	if err != nil {
		return err
	}
	store := config.StoreDirFor(entry.Key)
	nodes, err := resolver.FindNode(store, symbol)
	if err != nil {
		return err
	}
	key := "callees"
	if callers {
		key = "callers"
	}
	out := []map[string]any{}
	for _, n := range nodes {
		id := fmt.Sprint(n["id"])
		source, target := "", ""
		if callers {
			target = id
		} else {
			source = id
		}
		edges, _ := resolver.ResolvedEdgesFor(store, source, target, "calls")
		for _, e := range edges {
			if callers {
				out = append(out, map[string]any{"caller_id": e["source"], "caller_name": e["source_name"], "caller_file": e["source_file"], "line": e["line"], "target_id": id, "target_qname": n["qualified_name"]})
			} else {
				out = append(out, map[string]any{"source_id": id, "source_qname": n["qualified_name"], "callee_id": e["target"], "callee_name": e["target_name"], "callee_qname": e["target_qname"], "callee_file": e["target_file"], "callee_line": e["target_line"], "line": e["line"]})
			}
			if len(out) > limit {
				break
			}
		}
		if len(out) > limit {
			break
		}
	}
	truncated := len(out) > limit
	if truncated {
		out = out[:limit]
	}
	payload := map[string]any{"project": name, "symbol": symbol, "matched": len(nodes), key: out, "truncated": truncated}
	stale := graphpkg.AttachStale(payload, root, store)
	if cfg.JSON {
		return emit(cfg, payload, "")
	}
	if len(out) == 0 {
		fmt.Printf("(无%s) %s%s\n", key, symbol, staleHintText(stale))
		return nil
	}
	lines := []string{fmt.Sprintf("%d 个%s：", len(out), key)}
	for _, item := range out {
		lines = append(lines, fmt.Sprint(item))
	}
	fmt.Println(strings.Join(lines, "\n") + staleHintText(stale))
	return nil
}

func cmdImpact(cfg appConfig, args []string) error {
	fs := newFlagSet("impact")
	pathOpt := fs.String("path", "", "project path")
	depth := fs.Int("depth", 2, "depth")
	limit := fs.Int("limit", 100, "limit")
	if parseHelp(fs, args) {
		return nil
	}
	if err := parseFlagArgs(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("missing symbol")
	}
	cfg = withPathTarget(cfg, *pathOpt)
	root, name, entry, err := resolveProject(cfg, false)
	if err != nil {
		return err
	}
	store := config.StoreDirFor(entry.Key)
	payload, err := graphpkg.ImpactRadius(store, fs.Arg(0), *depth, *limit)
	if err != nil {
		return err
	}
	payload["project"] = name
	stale := graphpkg.AttachStale(payload, root, store)
	if cfg.JSON {
		return emit(cfg, payload, "")
	}
	nodes, _ := payload["nodes"].([]map[string]any)
	if len(nodes) == 0 {
		fmt.Printf("(无匹配) symbol=%q%s\n", fs.Arg(0), staleHintText(stale))
		return nil
	}
	fmt.Printf("%d 个受影响节点%s\n", len(nodes), staleHintText(stale))
	return nil
}

func cmdAffected(cfg appConfig, args []string) error {
	fs := newFlagSet("affected")
	pathOpt := fs.String("path", "", "project path")
	depth := fs.Int("depth", 5, "depth")
	testFilter := fs.String("filter", "", "test regex")
	fs.StringVar(testFilter, "test-filter", "", "test regex")
	stdin := fs.Bool("stdin", false, "read files from stdin")
	quiet := fs.Bool("quiet", false, "only print affected test paths")
	if parseHelp(fs, args) {
		return nil
	}
	if err := parseFlagArgs(fs, args); err != nil {
		return err
	}
	files := fs.Args()
	if *stdin {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			if s := strings.TrimSpace(sc.Text()); s != "" {
				files = append(files, s)
			}
		}
	}
	cfg = withPathTarget(cfg, *pathOpt)
	root, name, entry, err := resolveProject(cfg, false)
	if err != nil {
		return err
	}
	store := config.StoreDirFor(entry.Key)
	payload, err := graphpkg.AffectedFiles(store, files, *depth, *testFilter)
	if err != nil {
		return err
	}
	payload["project"] = name
	stale := graphpkg.AttachStale(payload, root, store)
	if *quiet && !cfg.JSON {
		if tests, ok := payload["affected_tests"].([]string); ok {
			fmt.Println(strings.Join(tests, "\n"))
		}
		return nil
	}
	if cfg.JSON {
		return emit(cfg, payload, "")
	}
	tests, _ := payload["affected_tests"].([]string)
	if len(tests) == 0 {
		fmt.Println("(无受影响测试文件)" + staleHintText(stale))
		return nil
	}
	lines := []string{fmt.Sprintf("%d 个受影响测试文件：", len(tests))}
	for _, path := range tests {
		lines = append(lines, "  "+path)
	}
	fmt.Println(strings.Join(lines, "\n") + staleHintText(stale))
	return nil
}

func cmdContext(cfg appConfig, args []string) error {
	fs := newFlagSet("context")
	pathOpt := fs.String("path", "", "project path")
	maxNodes := fs.Int("max-nodes", 20, "max nodes")
	maxCode := fs.Int("max-code", 8, "max code blocks")
	maxJSONBytes := fs.Int("max-json-bytes", 60000, "compact JSON output above this size; 0 disables")
	noCode := fs.Bool("no-code", false, "omit source snippets")
	summary := fs.Bool("summary", false, "compact project context")
	allowLarge := fs.Bool("allow-large", false, "allow large JSON output")
	legacyLimit := fs.Int("limit", 0, "legacy max nodes")
	if parseHelp(fs, args) {
		return nil
	}
	if err := parseFlagArgs(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("missing task")
	}
	if *legacyLimit > 0 {
		*maxNodes = *legacyLimit
	}
	cfg = withPathTarget(cfg, *pathOpt)
	task := strings.Join(fs.Args(), " ")
	root, name, entry, err := resolveProject(cfg, false)
	if err != nil {
		return err
	}
	store := config.StoreDirFor(entry.Key)
	payload, err := graphpkg.BuildContext(root, store, task, *maxNodes, *maxCode, !*noCode && !*summary)
	if err != nil {
		return err
	}
	if *summary {
		payload = graphpkg.CompactContext(payload)
	}
	payload["project"] = name
	stale := graphpkg.AttachStale(payload, root, store)
	if cfg.JSON && !*summary {
		payload = protectLargeContextPayload(payload, task, root, *maxJSONBytes, *allowLarge)
	}
	if cfg.JSON {
		return emit(cfg, payload, "")
	}
	if *summary {
		fmt.Print(graphpkg.FormatContextSummaryMarkdown(payload) + staleHintText(stale))
		return nil
	}
	fmt.Print(graphpkg.FormatContextMarkdown(payload) + staleHintText(stale))
	return nil
}

func protectLargeContextPayload(payload map[string]any, task, root string, maxBytes int, allowLarge bool) map[string]any {
	if allowLarge || maxBytes <= 0 {
		return payload
	}
	size, ok := jsonSize(payload)
	if !ok || size <= maxBytes {
		return payload
	}
	compact := graphpkg.CompactContext(payload)
	for _, key := range []string{"project", "stale"} {
		if v, ok := payload[key]; ok {
			compact[key] = v
		}
	}
	compact["truncated"] = true
	compact["output"] = map[string]any{
		"truncated":        true,
		"reason":           "json payload exceeded max_json_bytes",
		"estimated_bytes":  size,
		"max_json_bytes":   maxBytes,
		"summary_command":  "codegraph --json context " + strconv.Quote(task) + " --path " + strconv.Quote(root) + " --summary",
		"full_command":     "codegraph --json context " + strconv.Quote(task) + " --path " + strconv.Quote(root) + " --allow-large",
		"no_code_command":  "codegraph --json context " + strconv.Quote(task) + " --path " + strconv.Quote(root) + " --no-code",
		"compact_payload":  true,
		"preserved_fields": []string{"task", "entrypoints", "related", "relationships", "project", "stale"},
	}
	return compact
}

func jsonSize(payload any) (int, bool) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, false
	}
	return len(raw), true
}
