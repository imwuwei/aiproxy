// Package cli 实现 AIProxy 纯命令行版本的全部管理命令与服务模式。
// 零第三方依赖，复用 internal/app、internal/store、internal/proxy、internal/modelsync 等核心模块。
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// Run 执行 CLI 入口，返回进程退出码。
func Run(args []string) int {
	dbPath, jsonOut, rest := extractGlobalFlags(args)

	if len(rest) == 0 || rest[0] == "help" || rest[0] == "-h" || rest[0] == "--help" {
		printHelp(os.Stdout)
		return 0
	}

	cmd, cmdArgs := rest[0], rest[1:]
	switch cmd {
	case "serve", "run":
		return runWithFail(runServe(dbPath, cmdArgs))
	case "channels":
		return runWithFail(runChannels(dbPath, jsonOut, cmdArgs))
	case "aliases":
		return runWithFail(runAliases(dbPath, jsonOut, cmdArgs))
	case "models":
		return runWithFail(runModels(dbPath, jsonOut, cmdArgs))
	case "stats":
		return runWithFail(runStats(dbPath, jsonOut, cmdArgs))
	case "logs":
		return runWithFail(runLogs(dbPath, jsonOut, cmdArgs))
	case "settings":
		return runWithFail(runSettings(dbPath, jsonOut, cmdArgs))
	case "version", "--version", "-v":
		printVersion()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "未知命令: %s\n\n", cmd)
		printHelp(os.Stderr)
		return 2
	}
}

// runWithFail 执行命令并统一处理错误退出码。
func runWithFail(err error) int {
	if err == nil {
		return 0
	}
	if err == flag.ErrHelp {
		return 0
	}
	fmt.Fprintf(os.Stderr, "错误: %v\n", err)
	return 1
}

// extractGlobalFlags 从参数中提取全局参数（--db、--json），返回剩余参数。
// 全局参数可出现在任意位置。
func extractGlobalFlags(args []string) (dbPath string, jsonOut bool, rest []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			jsonOut = true
		case a == "--db":
			if i+1 < len(args) {
				dbPath = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "--db="):
			dbPath = strings.TrimPrefix(a, "--db=")
		default:
			rest = append(rest, a)
		}
	}
	return dbPath, jsonOut, rest
}

// newFlagSet 创建子命令 flag 集合：
// 错误信息统一由上层打印（丢弃 flag 包自带的错误输出），
// Usage 简化为一行用法提示。
func newFlagSet(name, usageLine string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: aiproxy %s\n", usageLine)
	}
	return fs
}

func printHelp(w io.Writer) {
	fmt.Fprintf(w, `AIProxy - OpenAI API 代理（命令行版）

用法:
  aiproxy [全局参数] <命令> [命令参数]

全局参数:
  --db <path>   数据库路径（默认: AIPROXY_DB 环境变量，否则可执行文件同目录 aiproxy.db）
  --json        以 JSON 格式输出列表/统计/明细

命令:
  serve           前台启动代理服务（Ctrl+C 停止，含定时模型同步与日志清理）
  channels        渠道管理（list/create/update/delete/enable/disable/test/sync）
  aliases         模型别名管理（list/create/update/delete/enable/disable）
  models          模型管理（list/sync-all）
  stats           用量统计（summary/daily/models/channels）
  logs            请求日志（list/clear）
  settings        配置管理（show/set/gen-token）
  version         显示版本信息
  help            显示本帮助

运行 "aiproxy <命令>" 或 "aiproxy <命令> --help" 查看子命令详情。
`)
}

func printVersion() {
	fmt.Println("aiproxy（命令行版）")
}
