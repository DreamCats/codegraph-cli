package cli

import (
	"github.com/DreamCats/codegraph-cli/internal/model"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

func Run(args []string) error {
	_, err := execute(args)
	return err
}

func execute(args []string) (appConfig, error) {
	cfg, rest, err := parseGlobal(args)
	if err != nil {
		return cfg, err
	}
	if len(rest) == 0 {
		printTopHelp()
		return cfg, nil
	}
	cmd := rest[0]
	if cmd == "-h" || cmd == "--help" || cmd == "help" {
		printTopHelp()
		return cfg, nil
	}
	if cmd == "--version" || cmd == "version" {
		fmt.Printf("codegraph %s\n", model.Version)
		return cfg, nil
	}
	return cfg, runCommand(cfg, cmd, rest[1:])
}

func Main(args []string) {
	if cfg, err := execute(args); err != nil {
		exitErr(cfg, err)
	}
}

func parseGlobal(args []string) (appConfig, []string, error) {
	cwd, _ := os.Getwd()
	cfg := appConfig{Cwd: cwd}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			cfg.JSON = true
		case a == "--verbose":
			cfg.Verbose = true
		case a == "-C" || a == "--target":
			if i+1 >= len(args) {
				return cfg, nil, fmt.Errorf("%s requires value", a)
			}
			i++
			cfg.Target = args[i]
		case strings.HasPrefix(a, "-C="):
			cfg.Target = strings.TrimPrefix(a, "-C=")
		case strings.HasPrefix(a, "--target="):
			cfg.Target = strings.TrimPrefix(a, "--target=")
		default:
			out = append(out, args[i:]...)
			return cfg, out, nil
		}
	}
	return cfg, out, nil
}

func runCommand(cfg appConfig, cmd string, args []string) error {
	switch cmd {
	case "init":
		return cmdInit(cfg, args)
	case "uninit":
		return cmdUninit(cfg, args)
	case "rm":
		return cmdRm(cfg, args)
	case "index":
		return cmdIndex(cfg, args)
	case "sync":
		return cmdSync(cfg, args)
	case "unlock":
		return cmdUnlock(cfg, args)
	case "query":
		return cmdQuery(cfg, args)
	case "files":
		return cmdFiles(cfg, args)
	case "status":
		return cmdStatus(cfg, args)
	case "list":
		return cmdList(cfg, args)
	case "info":
		return cmdInfo(cfg, args)
	case "resolve":
		return cmdResolve(cfg, args)
	case "callers":
		return cmdCallers(cfg, args)
	case "callees":
		return cmdCallees(cfg, args)
	case "impact":
		return cmdImpact(cfg, args)
	case "affected":
		return cmdAffected(cfg, args)
	case "context":
		return cmdContext(cfg, args)
	default:
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

func exitErr(cfg appConfig, err error) {
	if cfg.Verbose {
		panic(err)
	}
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

func printTopHelp() {
	fmt.Println(`Usage: codegraph [OPTIONS] COMMAND [ARGS]...

Options:
  -C, --target NAME|PATH  选择已注册项目（registry 中的 name 或路径）
      --json              JSON 格式输出
      --verbose           显示完整错误栈
  -h, --help              Show help
      --version           Show version

Commands:
  init      注册项目
  uninit    删除当前项目索引数据但保留注册
  rm        注销项目
  index     全量/增量索引
  sync      增量同步
  unlock    清理锁（当前为 no-op）
  query     搜索符号
  files     列出索引文件
  context   为任务构造上下文
  affected  查找受影响测试
  impact    符号影响半径
  status    索引统计
  callers   查调用方
  callees   查被调用方
  resolve   重跑调用关系解析
  list      列出已注册项目
  info      当前项目元数据`)
}

func commandHelp(name string) string {
	return "Usage: codegraph " + name + " [OPTIONS]\n"
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	fs.Usage = func() {
		fmt.Print(commandHelp(name))
		if fs.NFlag() >= 0 {
			fs.PrintDefaults()
		}
	}
	return fs
}

func parseHelp(fs *flag.FlagSet, args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			fs.Usage()
			return true
		}
	}
	return false
}

func parseFlagArgs(fs *flag.FlagSet, args []string) error {
	flags := []string{}
	positionals := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}
		name := strings.TrimLeft(arg, "-")
		if eq := strings.Index(name, "="); eq >= 0 {
			name = name[:eq]
		}
		f := fs.Lookup(name)
		if f == nil {
			flags = append(flags, arg)
			continue
		}
		flags = append(flags, arg)
		if strings.Contains(arg, "=") || isBoolFlag(f) {
			continue
		}
		if i+1 >= len(args) {
			return fmt.Errorf("%s requires value", arg)
		}
		i++
		flags = append(flags, args[i])
	}
	return fs.Parse(append(flags, positionals...))
}

func isBoolFlag(f *flag.Flag) bool {
	type boolFlag interface {
		IsBoolFlag() bool
	}
	bf, ok := f.Value.(boolFlag)
	return ok && bf.IsBoolFlag()
}

func emit(cfg appConfig, payload any, text string) error {
	if cfg.JSON {
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	if text != "" {
		fmt.Println(text)
		return nil
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
