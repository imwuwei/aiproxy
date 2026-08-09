package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"aiproxy/internal/models"
)

// runModels 模型管理命令分发。
func runModels(dbPath string, jsonOut bool, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(os.Stdout, `模型管理:
  models list               列出全部可用模型（含渠道数与 Token 用量）
  models list-custom        列出自定义模型
  models sync-all           全量同步所有启用渠道的模型
  models add <name>         添加自定义模型
    --desc <描述>           模型描述（可选）
    --channels <id1,id2>    绑定的渠道 ID 列表（逗号分隔）
  models remove <name>      删除自定义模型（及其所有渠道绑定）
  models edit <name>        编辑自定义模型描述
    --desc <描述>           新的模型描述
  models bindings <name>    查看模型绑定的渠道
  models bind <name>        设置模型绑定的渠道（全量覆盖）
    --channels <id1,id2>    目标渠道 ID 列表（逗号分隔）
`)
		return nil
	}

	sub := args[0]
	subArgs := args[1:]
	switch sub {
	case "list":
		return modelList(dbPath, jsonOut, subArgs)
	case "list-custom":
		return modelListCustom(dbPath, jsonOut, subArgs)
	case "sync-all":
		return modelSyncAll(dbPath, jsonOut, subArgs)
	case "add":
		return modelAdd(dbPath, jsonOut, subArgs)
	case "remove":
		return modelRemove(dbPath, jsonOut, subArgs)
	case "edit":
		return modelEdit(dbPath, jsonOut, subArgs)
	case "bindings":
		return modelBindings(dbPath, jsonOut, subArgs)
	case "bind":
		return modelBind(dbPath, jsonOut, subArgs)
	default:
		return fmt.Errorf("未知模型命令: %s（可用: list/list-custom/sync-all/add/remove/edit/bindings/bind）", sub)
	}
}

// modelList 列出全部可用模型。
func modelList(dbPath string, jsonOut bool, args []string) error {
	fs := newFlagSet("list", "models list")
	if err := fs.Parse(args); err != nil {
		return err
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	modelsList, err := comps.ModelStore.ListAllModels()
	if err != nil {
		return err
	}

	// 加载自定义模型名称集合
	customNames := map[string]bool{}
	if customModels, err := comps.CustomModelStore.List(); err == nil {
		for _, cm := range customModels {
			customNames[cm.Name] = true
		}
	}

	// 模型 Token 用量（全量历史）
	tokenMap := map[string]*models.TokenUsage{}
	if stats, err := comps.UsageStore.ModelTokenUsage(); err == nil {
		for _, st := range stats {
			tokenMap[st.Model] = st
		}
	}

	type modelRow struct {
		Model            string `json:"model"`
		ChannelCount     int    `json:"channel_count"`
		PromptTokens     int64  `json:"prompt_tokens"`
		CompletionTokens int64  `json:"completion_tokens"`
		TotalTokens      int64  `json:"total_tokens"`
		IsCustom         bool   `json:"is_custom"`
	}

	rows := make([]modelRow, 0, len(modelsList))
	for _, m := range modelsList {
		n, err := comps.ModelStore.CountChannelsForModel(m)
		if err != nil {
			n = 0
		}
		r := modelRow{Model: m, ChannelCount: n, IsCustom: customNames[m]}
		if tu, ok := tokenMap[m]; ok {
			r.PromptTokens = tu.PromptTokens
			r.CompletionTokens = tu.CompletionTokens
			r.TotalTokens = tu.TotalTokens
		}
		rows = append(rows, r)
	}

	if jsonOut {
		return printJSON(rows)
	}

	t := newTablePrinter(os.Stdout)
	t.addRow("模型 ID", "渠道数", "输入 Token", "输出 Token", "总 Token", "来源")
	for _, r := range rows {
		source := "同步"
		if r.IsCustom {
			source = "自定义"
		}
		t.addRow(
			r.Model,
			strconv.Itoa(r.ChannelCount),
			formatInt64(r.PromptTokens),
			formatInt64(r.CompletionTokens),
			formatInt64(r.TotalTokens),
			source,
		)
	}
	t.print()
	return nil
}

// modelListCustom 列出自定义模型。
func modelListCustom(dbPath string, jsonOut bool, args []string) error {
	fs := newFlagSet("list-custom", "models list-custom")
	if err := fs.Parse(args); err != nil {
		return err
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	customModels, err := comps.CustomModelStore.List()
	if err != nil {
		return err
	}

	if jsonOut {
		return printJSON(customModels)
	}

	if len(customModels) == 0 {
		fmt.Println("暂无自定义模型")
		return nil
	}

	t := newTablePrinter(os.Stdout)
	t.addRow("模型名称", "描述", "创建时间")
	for _, cm := range customModels {
		desc := cm.Description
		if desc == "" {
			desc = "-"
		}
		t.addRow(cm.Name, desc, formatUnixTime(cm.CreatedAt))
	}
	t.print()
	return nil
}

// modelSyncAll 全量同步所有启用渠道的模型。
func modelSyncAll(dbPath string, jsonOut bool, args []string) error {
	fs := newFlagSet("sync-all", "models sync-all")
	if err := fs.Parse(args); err != nil {
		return err
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	// 同步前记录启用渠道数，便于反馈
	channels, err := comps.ChannelStore.ListEnabled()
	if err != nil {
		return err
	}
	comps.ModelSync.SyncAll() // SyncAll 内部自行加载渠道并并发同步

	if jsonOut {
		return printJSON(map[string]any{
			"synced":       true,
			"channelCount": len(channels),
		})
	}
	fmt.Printf("全量模型同步完成（共 %d 个启用渠道）\n", len(channels))
	return nil
}

// modelAdd 添加自定义模型。
func modelAdd(dbPath string, jsonOut bool, args []string) error {
	fs := newFlagSet("add", "models add <name> --desc <描述> --channels <id1,id2>")
	var desc, channelsStr string
	fs.StringVar(&desc, "desc", "", "模型描述")
	fs.StringVar(&channelsStr, "channels", "", "绑定的渠道 ID 列表（逗号分隔）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("用法: models add <name> --desc <描述> --channels <id1,id2>")
	}
	name := fs.Arg(0)

	channelIDs, err := parseChannelIDs(channelsStr)
	if err != nil {
		return err
	}
	if len(channelIDs) == 0 {
		return fmt.Errorf("请通过 --channels 指定至少一个渠道 ID")
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	// 检查是否已存在
	if existing, _ := comps.CustomModelStore.GetByName(name); existing != nil {
		return fmt.Errorf("自定义模型「%s」已存在", name)
	}

	// 验证渠道存在
	for _, id := range channelIDs {
		if _, err := comps.ChannelStore.Get(id); err != nil {
			return fmt.Errorf("渠道 %d 不存在: %w", id, err)
		}
	}

	if _, err := comps.CustomModelStore.Create(name, desc); err != nil {
		return err
	}
	if err := comps.ModelStore.SetBindings(name, channelIDs); err != nil {
		// 绑定失败时回滚元数据
		_ = comps.CustomModelStore.Delete(name)
		return fmt.Errorf("绑定渠道失败: %w", err)
	}

	if jsonOut {
		return printJSON(map[string]any{
			"created":     true,
			"name":        name,
			"description": desc,
			"channels":    channelIDs,
		})
	}
	fmt.Printf("自定义模型「%s」已创建，绑定 %d 个渠道\n", name, len(channelIDs))
	return nil
}

// modelRemove 删除自定义模型。
func modelRemove(dbPath string, jsonOut bool, args []string) error {
	fs := newFlagSet("remove", "models remove <name>")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("用法: models remove <name>")
	}
	name := fs.Arg(0)

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	// 检查是否存在
	cm, _ := comps.CustomModelStore.GetByName(name)
	if cm == nil {
		return fmt.Errorf("自定义模型「%s」不存在", name)
	}

	// 先删除所有绑定，再删除元数据
	if err := comps.ModelStore.DeleteAllBindings(name); err != nil {
		return fmt.Errorf("删除渠道绑定失败: %w", err)
	}
	if err := comps.CustomModelStore.Delete(name); err != nil {
		return fmt.Errorf("删除自定义模型失败: %w", err)
	}

	if jsonOut {
		return printJSON(map[string]any{"deleted": true, "name": name})
	}
	fmt.Printf("自定义模型「%s」已删除\n", name)
	return nil
}

// modelEdit 编辑自定义模型描述。
func modelEdit(dbPath string, jsonOut bool, args []string) error {
	fs := newFlagSet("edit", "models edit <name> --desc <描述>")
	var desc string
	fs.StringVar(&desc, "desc", "", "新的模型描述")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("用法: models edit <name> --desc <描述>")
	}
	name := fs.Arg(0)

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	if err := comps.CustomModelStore.Update(name, desc); err != nil {
		return err
	}

	if jsonOut {
		return printJSON(map[string]any{"updated": true, "name": name, "description": desc})
	}
	fmt.Printf("自定义模型「%s」描述已更新\n", name)
	return nil
}

// modelBindings 查看模型绑定的渠道。
func modelBindings(dbPath string, jsonOut bool, args []string) error {
	fs := newFlagSet("bindings", "models bindings <name>")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("用法: models bindings <name>")
	}
	name := fs.Arg(0)

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	bindings, err := comps.ModelStore.GetModelBindings(name)
	if err != nil {
		return err
	}

	if jsonOut {
		return printJSON(bindings)
	}

	if len(bindings) == 0 {
		fmt.Printf("模型「%s」暂无渠道绑定\n", name)
		return nil
	}

	t := newTablePrinter(os.Stdout)
	t.addRow("渠道 ID", "渠道名称", "来源", "渠道状态")
	for _, b := range bindings {
		source := b.Source
		switch b.Source {
		case "sync":
			source = "同步"
		case "custom":
			source = "自定义"
		case "excluded":
			source = "已排除"
		}
		status := "启用"
		if !b.ChannelEnabled {
			status = "停用"
		}
		t.addRow(strconv.FormatInt(b.ChannelID, 10), b.ChannelName, source, status)
	}
	t.print()
	return nil
}

// modelBind 设置模型绑定的渠道。
func modelBind(dbPath string, jsonOut bool, args []string) error {
	fs := newFlagSet("bind", "models bind <name> --channels <id1,id2>")
	var channelsStr string
	fs.StringVar(&channelsStr, "channels", "", "目标渠道 ID 列表（逗号分隔）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("用法: models bind <name> --channels <id1,id2>")
	}
	name := fs.Arg(0)

	channelIDs, err := parseChannelIDs(channelsStr)
	if err != nil {
		return err
	}

	comps, err := openApp(dbPath)
	if err != nil {
		return err
	}
	defer comps.Close()

	// 验证渠道存在
	for _, id := range channelIDs {
		if _, err := comps.ChannelStore.Get(id); err != nil {
			return fmt.Errorf("渠道 %d 不存在: %w", id, err)
		}
	}

	if err := comps.ModelStore.SetBindings(name, channelIDs); err != nil {
		return fmt.Errorf("设置渠道绑定失败: %w", err)
	}

	if jsonOut {
		return printJSON(map[string]any{"bound": true, "name": name, "channels": channelIDs})
	}
	fmt.Printf("模型「%s」渠道绑定已更新（共 %d 个渠道）\n", name, len(channelIDs))
	return nil
}

// formatUnixTime 格式化 Unix 秒时间戳为 "YYYY-MM-DD HH:mm:ss"。
func formatUnixTime(ts int64) string {
	if ts == 0 {
		return "--"
	}
	t := time.Unix(ts, 0)
	return t.Format("2006-01-02 15:04:05")
}

// parseChannelIDs 解析逗号分隔的渠道 ID 列表。
func parseChannelIDs(s string) ([]int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	ids := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("无效的渠道 ID: %s", p)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// formatInt64 千分位格式化整数。
func formatInt64(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}
