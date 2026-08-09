package cli

import (
	"fmt"
	"os"
	"time"

	"aiproxy/internal/models"
)

// runStats 用量统计命令分发。
func runStats(dbPath string, jsonOut bool, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(os.Stdout, `用量统计:
  stats summary [--range <today|7d|30d|week|month|custom>] [--start <YYYY-MM-DD>] [--end <YYYY-MM-DD>] [--channel <ID>] [--model <模型>]   时间范围内整体汇总
  stats daily    [--range <7d|30d|week|month|custom>] [--start <YYYY-MM-DD>] [--end <YYYY-MM-DD>] [--channel <ID>] [--model <模型>]        按日统计
  stats models   [--range <7d|30d|week|month|custom>] [--start <YYYY-MM-DD>] [--end <YYYY-MM-DD>] [--channel <ID>]                         按模型统计
  stats channels [--range <7d|30d|week|month|custom>] [--start <YYYY-MM-DD>] [--end <YYYY-MM-DD>] [--model <模型>]                         按渠道统计

参数:
  --range <today|7d|30d|week|month|custom>   时间范围（默认 7d；summary 默认 today；custom 需配合 --start/--end）
  --start <YYYY-MM-DD>                       自定义开始日期（--range custom 时生效，含当天）
  --end <YYYY-MM-DD>                         自定义结束日期（--range custom 时生效，含当天）
  --channel <ID>           渠道 ID 筛选
  --model <模型>           模型名筛选
`)
		return nil
	}

	sub := args[0]
	subArgs := args[1:]
	switch sub {
	case "summary":
		return statsSummary(dbPath, jsonOut, subArgs)
	case "daily":
		return statsDaily(dbPath, jsonOut, subArgs)
	case "models":
		return statsModels(dbPath, jsonOut, subArgs)
	case "channels":
		return statsChannels(dbPath, jsonOut, subArgs)
	default:
		return fmt.Errorf("未知统计命令: %s（可用: summary/daily/models/channels）", sub)
	}
}

// parseRange 解析时间范围参数，返回起始时间。
// end 恒为"今天结束 + 1（即明天零点）"，与 GUI 的 [start, end) 语义一致。
// --range custom 时使用 startDate/endDate（YYYY-MM-DD，endDate 含当天）。
func parseRange(rangeVal, startDate, endDate string) (start, end time.Time, err error) {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end = todayStart.AddDate(0, 0, 1)

	switch rangeVal {
	case "today":
		start = todayStart
	case "7d":
		start = todayStart.AddDate(0, 0, -6)
	case "30d":
		start = todayStart.AddDate(0, 0, -29)
	case "week":
		// 本周一 00:00 起
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start = todayStart.AddDate(0, 0, -(weekday - 1))
	case "month":
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	case "custom":
		start = todayStart
		if startDate != "" {
			if t, perr := time.ParseInLocation("2006-01-02", startDate, now.Location()); perr == nil {
				start = t
			}
		}
		if endDate != "" {
			if t, perr := time.ParseInLocation("2006-01-02", endDate, now.Location()); perr == nil {
				end = t.AddDate(0, 0, 1)
			}
		}
		if end.Before(start.AddDate(0, 0, 1)) {
			end = start.AddDate(0, 0, 1)
		}
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("参数 --range 取值无效: %s（可选 today/7d/30d/week/month/custom）", rangeVal)
	}
	return start, end, nil
}

// statBaseFlags2 解析统计命令通用 flags。
func statBaseFlags2(subArgs []string, defaultRange string) (rangeVal string, channelID int64, model, startDate, endDate string, err error) {
	fs := newFlagSet("stats2", "stats2")
	rv := fs.String("range", defaultRange, "时间范围")
	ch := fs.Int64("channel", 0, "渠道 ID")
	m := fs.String("model", "", "模型名")
	sd := fs.String("start", "", "自定义开始日期 YYYY-MM-DD")
	ed := fs.String("end", "", "自定义结束日期 YYYY-MM-DD")
	if err := fs.Parse(subArgs); err != nil {
		return "", 0, "", "", "", err
	}
	return *rv, *ch, *m, *sd, *ed, nil
}

// statsSummary 时间范围内整体汇总（今日调用次数/Token/成功失败）。
func statsSummary(dbPath string, jsonOut bool, args []string) error {
	rangeVal, channelID, model, startDate, endDate, err := statBaseFlags2(args, "today")
	if err != nil {
		return err
	}
	start, end, err := parseRange(rangeVal, startDate, endDate)
	if err != nil {
		return err
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	st, err := comps.UsageStore.Summary(start, end, channelID, model)
	if err != nil {
		return err
	}

	if jsonOut {
		return printJSON(struct {
			Start              string `json:"start"`
			End                string `json:"end"`
			models.SummaryStat `json:",inline"`
		}{
			Start:       start.Format("2006-01-02"),
			End:         end.AddDate(0, 0, -1).Format("2006-01-02"),
			SummaryStat: *st,
		})
	}

	fmt.Printf("统计范围: %s ~ %s\n", start.Format("2006-01-02"), end.AddDate(0, 0, -1).Format("2006-01-02"))
	fmt.Printf("调用次数:     %s\n", formatInt64(st.Count))
	fmt.Printf("输入 Token:   %s\n", formatInt64(st.PromptTokens))
	fmt.Printf("输出 Token:   %s\n", formatInt64(st.CompletionTokens))
	fmt.Printf("总 Token:     %s\n", formatInt64(st.TotalTokens))
	fmt.Printf("成功/失败:     %s / %s\n", formatInt64(st.SuccessCount), formatInt64(st.FailCount))
	return nil
}

// statsDaily 按日统计。
func statsDaily(dbPath string, jsonOut bool, args []string) error {
	rangeVal, channelID, model, startDate, endDate, err := statBaseFlags2(args, "7d")
	if err != nil {
		return err
	}
	start, end, err := parseRange(rangeVal, startDate, endDate)
	if err != nil {
		return err
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	dailyStats, err := comps.UsageStore.DailyStats(start, end, channelID, model)
	if err != nil {
		return err
	}

	if jsonOut {
		return printJSON(dailyStats)
	}

	t := newTablePrinter(os.Stdout)
	t.addRow("日期", "调用次数", "输入 Token", "输出 Token", "总 Token", "成功/失败")
	for _, d := range dailyStats {
		t.addRow(
			d.Date,
			formatInt64(d.Count),
			formatInt64(d.PromptTokens),
			formatInt64(d.CompletionTokens),
			formatInt64(d.TotalTokens),
			fmt.Sprintf("%s / %s", formatInt64(d.SuccessCount), formatInt64(d.FailCount)),
		)
	}
	t.print()
	return nil
}

// statsModels 按模型统计。
func statsModels(dbPath string, jsonOut bool, args []string) error {
	rangeVal, channelID, _, startDate, endDate, err := statBaseFlags2(args, "7d")
	if err != nil {
		return err
	}
	start, end, err := parseRange(rangeVal, startDate, endDate)
	if err != nil {
		return err
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	stats, err := comps.UsageStore.ModelStats(start, end, channelID)
	if err != nil {
		return err
	}

	if jsonOut {
		return printJSON(stats)
	}

	t := newTablePrinter(os.Stdout)
	t.addRow("模型", "调用次数", "输入 Token", "输出 Token", "总 Token", "成功/失败")
	for _, m := range stats {
		t.addRow(
			m.Model,
			formatInt64(m.Count),
			formatInt64(m.PromptTokens),
			formatInt64(m.CompletionTokens),
			formatInt64(m.TotalTokens),
			fmt.Sprintf("%s / %s", formatInt64(m.SuccessCount), formatInt64(m.FailCount)),
		)
	}
	t.print()
	return nil
}

// statsChannels 按渠道统计。
func statsChannels(dbPath string, jsonOut bool, args []string) error {
	rangeVal, _, model, startDate, endDate, err := statBaseFlags2(args, "7d")
	if err != nil {
		return err
	}
	start, end, err := parseRange(rangeVal, startDate, endDate)
	if err != nil {
		return err
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	stats, err := comps.UsageStore.ChannelStats(start, end, model)
	if err != nil {
		return err
	}

	if jsonOut {
		return printJSON(stats)
	}

	t := newTablePrinter(os.Stdout)
	t.addRow("渠道", "调用次数", "输入 Token", "输出 Token", "总 Token", "成功/失败")
	for _, c := range stats {
		name := c.ChannelName
		if name == "" {
			name = fmt.Sprintf("渠道#%d", c.ChannelID)
		}
		t.addRow(
			name,
			formatInt64(c.Count),
			formatInt64(c.PromptTokens),
			formatInt64(c.CompletionTokens),
			formatInt64(c.TotalTokens),
			fmt.Sprintf("%s / %s", formatInt64(c.SuccessCount), formatInt64(c.FailCount)),
		)
	}
	t.print()
	return nil
}
