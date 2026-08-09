package cli

import (
	"fmt"
	"os"
	"strconv"

	"aiproxy/internal/models"
)

// runLogs 请求日志命令分发。
func runLogs(dbPath string, jsonOut bool, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(os.Stdout, `请求日志:
  logs list [--limit <N>] [--channel <ID>] [--model <模型>]   查看最近的请求日志（默认 200 条）
  logs clear                                                 清空全部请求日志

参数:
  --limit <N>    返回条数（默认 200）
  --channel <ID> 按渠道 ID 筛选
  --model <模型>  按模型名筛选
`)
		return nil
	}

	sub := args[0]
	subArgs := args[1:]
	switch sub {
	case "list":
		return logList(dbPath, jsonOut, subArgs)
	case "clear":
		return logClear(dbPath, jsonOut, subArgs)
	default:
		return fmt.Errorf("未知日志命令: %s（可用: list/clear）", sub)
	}
}

// logList 列出最近的请求日志。
func logList(dbPath string, jsonOut bool, args []string) error {
	fs := newFlagSet("list", "logs list [--limit <N>] [--channel <ID>] [--model <模型>]")
	limit := fs.Int("limit", 200, "返回条数")
	channelID := fs.Int64("channel", 0, "渠道 ID")
	model := fs.String("model", "", "模型名")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *limit <= 0 {
		return fmt.Errorf("参数 --limit 必须为正整数")
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	records, err := comps.UsageStore.ListRecent(*limit, *channelID, *model)
	if err != nil {
		return err
	}

	if jsonOut {
		return printJSON(records)
	}

	t := newTablePrinter(os.Stdout)
	t.addRow("ID", "时间", "渠道", "模型", "状态码", "耗时(ms)", "输入/输出 Token", "错误")
	for _, r := range records {
		t.addRow(
			strconv.FormatInt(r.ID, 10),
			formatRecordTime(r),
			r.ChannelName,
			r.Model,
			strconv.Itoa(r.StatusCode),
			strconv.FormatInt(r.DurationMs, 10),
			fmt.Sprintf("%d / %d", r.PromptTokens, r.CompletionTokens),
			truncateStr(r.Error, 30),
		)
	}
	t.print()
	return nil
}

// formatRecordTime 日志时间显示（优先数据库原始字符串，回退到格式化）。
func formatRecordTime(r *models.UsageRecord) string {
	if r == nil {
		return ""
	}
	if r.CreatedAtRaw != "" {
		return r.CreatedAtRaw
	}
	if !r.CreatedAt.IsZero() {
		return r.CreatedAt.Format("2006-01-02 15:04:05")
	}
	return ""
}

// logClear 清空全部请求日志。
func logClear(dbPath string, jsonOut bool, args []string) error {
	fs := newFlagSet("clear", "logs clear")
	if err := fs.Parse(args); err != nil {
		return err
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	n, err := comps.UsageStore.Clear()
	if err != nil {
		return err
	}

	if jsonOut {
		return printJSON(map[string]any{"cleared": n})
	}
	fmt.Printf("已清除 %d 条请求日志\n", n)
	return nil
}
